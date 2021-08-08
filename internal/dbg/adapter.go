package dbg

import (
	"context"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"

	"github.com/traefik/yaegi/internal/dap"
	"github.com/traefik/yaegi/internal/iox"
	"github.com/traefik/yaegi/interp"
)

// Options for Adapter.
type Options struct {
	// SrcPath is used for setting breakpoints on '_.go'. If it is non-empty,
	// SrcPath will be replaced with _.go in breakpoint requests and vice versa
	// in stack traces.
	SrcPath string

	// NewInterpreter is called to create the interpreter.
	NewInterpreter func(interp.Options) (*interp.Interpreter, error)

	// If StopAtEntry is set, the debugger will halt on entry.
	StopAtEntry bool

	// Non-fatal errors will be sent to Errors if it is non-nil
	Errors chan<- error
}

type compileFunc func(*interp.Interpreter, string) (*interp.Program, error)

// Adapter is a DAP handler that debugs Go code compiled with an interpreter.
type Adapter struct {
	opts    Options
	compile compileFunc
	arg     string

	session *dap.Session
	ccaps   *dap.InitializeRequestArguments

	interp   *interp.Interpreter
	debugger *interp.Debugger

	events *events
	frames *frames
	vars   *variables
}

// NewEvalAdapter returns an Adapter that debugs a Go code represented as a
// string.
func NewEvalAdapter(src string, opts *Options) *Adapter {
	return newAdapter((*interp.Interpreter).Compile, src, opts)
}

// NewEvalPathAdapter returns an Adapter that debugs Go code located at the
// given path.
func NewEvalPathAdapter(path string, opts *Options) *Adapter {
	return newAdapter((*interp.Interpreter).CompilePath, path, opts)
}

func newAdapter(eval compileFunc, arg string, opts *Options) *Adapter {
	if opts == nil {
		opts = new(Options)
	}
	if opts.NewInterpreter == nil {
		opts.NewInterpreter = func(opts interp.Options) (*interp.Interpreter, error) {
			return interp.New(opts), nil
		}
	}

	a := new(Adapter)
	a.opts = *opts
	a.compile = eval
	a.arg = arg
	a.events = newEvents()
	a.frames = newFrames()
	a.vars = newVariables()
	return a
}

func (a *Adapter) step(id int, reason interp.DebugEventReason) (bool, string) {
	err := a.debugger.Step(id, reason)
	if err == nil {
		return true, ""
	}
	return false, fmt.Sprintf("routine %d: failed to step: %v", id, err)
}

func (a *Adapter) cont(id int) (bool, string) {
	err := a.debugger.Continue(id)
	if err == nil {
		return true, ""
	}
	return false, fmt.Sprintf("routine %d: failed to continue: %v", id, err)
}

// read from stdin.
func (a *Adapter) stdin(b []byte) (int, error) {
	return 0, io.EOF
}

// send a stdout event.
func (a *Adapter) stdout(b []byte) (int, error) {
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: dap.Str("stdout"),
		Output:   string(b),
	})
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// send a stderr event.
func (a *Adapter) stderr(b []byte) (int, error) {
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: dap.Str("stderr"),
		Output:   string(b),
	})
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// Initialize implements dap.Handler and should not be called directly.
func (a *Adapter) Initialize(s *dap.Session, ccaps *dap.InitializeRequestArguments) (*dap.Capabilities, error) {
	a.session, a.ccaps = s, ccaps
	return &dap.Capabilities{
		SupportsConfigurationDoneRequest: dap.Bool(true),
	}, nil
}

// Process implements dap.Handler and should not be called directly.
func (a *Adapter) Process(m dap.IProtocolMessage) error {
	var stop bool
	switch m := m.(type) { //nolint:gocritic
	case *dap.Request:
		success := false
		var message string
		var body dap.ResponseBody
		switch m.Command {
		case "launch", "attach":
			i, err := a.opts.NewInterpreter(interp.Options{
				Stdin:  iox.ReaderFunc(a.stdin),
				Stdout: iox.WriterFunc(a.stdout),
				Stderr: iox.WriterFunc(a.stderr),
			})
			if err != nil {
				return err
			}
			a.interp = i

			prog, err := a.compile(a.interp, a.arg)
			if err == nil {
				success = true
				err = a.session.Event("initialized", nil)
			} else {
				stop = true
				err = a.session.Event("output", &dap.OutputEventBody{
					Category: dap.Str("stderr"),
					Output:   err.Error(),
					Data:     err,
				})
				message = fmt.Sprintf("Failed to compile: %v", err)
				break
			}
			if err != nil {
				return err
			}

			a.debugger = a.interp.Debug(context.Background(), prog, func(e *interp.DebugEvent) {
				if e.Reason() == interp.DebugEnterGoRoutine {
					err := a.session.Event("thread", &dap.ThreadEventBody{
						Reason:   "started",
						ThreadId: e.GoRoutine(),
					})
					if a.opts.Errors != nil {
						a.opts.Errors <- err
					}
					return
				}

				if e.Reason() == interp.DebugExitGoRoutine {
					err := a.session.Event("thread", &dap.ThreadEventBody{
						Reason:   "exited",
						ThreadId: e.GoRoutine(),
					})
					if a.opts.Errors != nil {
						a.opts.Errors <- err
					}
					return
				}

				a.frames.Purge()
				a.vars.Purge()

				if e.Reason() == interp.DebugTerminate {
					err := a.session.Event("terminated", nil)
					if a.opts.Errors != nil {
						a.opts.Errors <- err
					}
					stop = true
					return
				}

				a.events.Retain(e)

				body := new(dap.StoppedEventBody)
				body.ThreadId = dap.Int(e.GoRoutine())
				switch e.Reason() {
				case interp.DebugBreak:
					body.Reason = "breakpoint"
				case interp.DebugStepInto, interp.DebugStepOver, interp.DebugStepOut:
					body.Reason = "step"
				case interp.DebugEntry:
					body.Reason = "entry"
				default:
					body.Reason = "pause"
				}
				err := a.session.Event("stopped", body)
				if a.opts.Errors != nil {
					a.opts.Errors <- err
				}
			}, &interp.DebugOptions{
				GoRoutineStartAt1: true,
			})

		case "setBreakpoints":
			args := m.Arguments.(*dap.SetBreakpointsArguments)
			if args.Source.Path == nil {
				message = "Missing source"
				break
			}

			path := args.Source.Path.Get()
			if a.opts.SrcPath != "" && path == a.opts.SrcPath {
				path = "_.go"
			}

			var bp []*interp.Breakpoint
			if args.Breakpoints != nil {
				bp = make([]*interp.Breakpoint, len(args.Breakpoints))
				for i := range bp {
					b := args.Breakpoints[i]
					if a.ccaps.LinesStartAt1.False() {
						b.Line++
					}
					if b.Column != nil && a.ccaps.ColumnsStartAt1.False() {
						*b.Column++
					}

					bp[i] = &interp.Breakpoint{
						Line:   b.Line,
						Column: b.Column.GetOr(0),
					}
				}
			} else {
				bp = make([]*interp.Breakpoint, len(args.Lines))
				for i := range bp {
					l := args.Lines[i]
					if a.ccaps.LinesStartAt1.False() {
						l++
					}

					bp[i].Line = l
				}
			}

			bp = a.debugger.SetBreakpoints(path, bp)

			b := new(dap.SetBreakpointsResponseBody)
			body = b
			b.Breakpoints = make([]*dap.Breakpoint, len(bp))

			for i, bp := range bp {
				if bp == nil {
					b.Breakpoints[i] = &dap.Breakpoint{Verified: false}
					continue
				}

				if a.ccaps.LinesStartAt1.False() {
					bp.Line--
				}
				if a.ccaps.ColumnsStartAt1.False() {
					bp.Column--
				}

				b.Breakpoints[i] = &dap.Breakpoint{
					Verified: true,
					Line:     dap.Int(bp.Line),
					Column:   dap.Int(bp.Column),
				}
			}

			success = true

		case "configurationDone":
			if a.opts.StopAtEntry {
				success, message = a.step(1, interp.DebugEntry)
			} else {
				success, message = a.cont(1)
			}

		case "continue":
			args := m.Arguments.(*dap.ContinueArguments)
			a.events.Release(args.ThreadId)
			success, message = a.cont(args.ThreadId)
			body = &dap.ContinueResponseBody{AllThreadsContinued: dap.Bool(false)}

		case "stepIn":
			args := m.Arguments.(*dap.StepInArguments)
			a.events.Release(args.ThreadId)
			success, message = a.step(args.ThreadId, interp.DebugStepInto)

		case "next":
			args := m.Arguments.(*dap.NextArguments)
			a.events.Release(args.ThreadId)
			success, message = a.step(args.ThreadId, interp.DebugStepOver)

		case "stepOut":
			args := m.Arguments.(*dap.StepOutArguments)
			a.events.Release(args.ThreadId)
			success, message = a.step(args.ThreadId, interp.DebugStepOut)

		case "pause":
			args := m.Arguments.(*dap.PauseArguments)
			a.events.Release(args.ThreadId)
			success = a.debugger.Interrupt(args.ThreadId, interp.DebugPause)

		case "threads":
			success = true
			r := a.debugger.GoRoutines()
			b := &dap.ThreadsResponseBody{Threads: make([]*dap.Thread, len(r))}
			body = b

			for i, r := range r {
				b.Threads[i] = &dap.Thread{Id: r.ID(), Name: r.Name()}
			}

		case "stackTrace":
			args := m.Arguments.(*dap.StackTraceArguments)
			e, ok := a.events.Get(args.ThreadId)
			if !ok {
				message = "Invalid thread ID"
				break
			} else {
				success = true
			}

			b := new(dap.StackTraceResponseBody)
			body = b

			b.TotalFrames = dap.Int(e.FrameDepth())
			end := b.TotalFrames.Get()
			if args.Levels.GetOr(0) > 0 {
				end = args.StartFrame.GetOr(0) + args.Levels.Get()
			}

			frames := e.Frames(args.StartFrame.GetOr(0), end)
			b.StackFrames = make([]*dap.StackFrame, len(frames))
			for i, f := range frames {
				var src *dap.Source
				pos := f.Position()
				if pos != (token.Position{}) {
					if a.ccaps.LinesStartAt1.False() {
						pos.Line--
					}
					if a.ccaps.ColumnsStartAt1.False() {
						pos.Column--
					}

					src = new(dap.Source)
					src.Path = dap.Str(pos.Filename)
					if a.opts.SrcPath != "" && pos.Filename == "_.go" {
						src.Path = dap.Str(a.opts.SrcPath)
					}
					src.Name = dap.Str(filepath.Base(src.Path.Get()))
				}

				b.StackFrames[i] = &dap.StackFrame{
					Id:     a.frames.Add(f),
					Name:   f.Name(),
					Line:   pos.Line,
					Column: pos.Column,
					Source: src,
				}
			}

		case "scopes":
			args := m.Arguments.(*dap.ScopesArguments)
			f, ok := a.frames.Get(args.FrameId)
			if !ok {
				message = "Invalid frame ID"
				break
			} else {
				success = true
			}

			sc := f.Scopes()
			b := &dap.ScopesResponseBody{Scopes: make([]*dap.Scope, len(sc))}
			body = b

			for i, sc := range sc {
				name := "Locals"
				if sc.IsClosure() {
					name = "Closure"
				}

				b.Scopes[i] = &dap.Scope{
					Name:               name,
					PresentationHint:   dap.Str("Locals"),
					VariablesReference: a.vars.Add(&frameVars{sc}),
				}
			}

		case "variables":
			args := m.Arguments.(*dap.VariablesArguments)
			scope, ok := a.vars.Get(args.VariablesReference)
			if !ok {
				message = "Invalid variable reference"
				break
			} else {
				success = true
			}

			body = &dap.VariablesResponseBody{
				Variables: scope.Variables(a),
			}

		case "terminate":
			a.debugger.Terminate()
			success = true

		case "disconnect":
			// Go does not allow forcibly killing a goroutine
			a.debugger.Terminate()
			stop = true
			success = true

		default:
			fmt.Fprintf(os.Stderr, "! unknown %q\n", m.Command)
			message = fmt.Sprintf("Unknown command %q", m.Command)
		}

		if message == "" {
			if success {
				message = "Success"
			} else {
				message = "Failure"
			}
		}

		err := a.session.Respond(m, success, message, body)
		if err != nil {
			return err
		}
	}

	if stop {
		return dap.ErrStop
	}
	return nil
}

// Terminate implements dap.Handler and should not be called directly.
func (a *Adapter) Terminate() {
	if a.debugger != nil {
		a.debugger.Terminate()
		a.debugger.Wait() //nolint:errcheck
	}
}
