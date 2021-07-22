package dbg

import (
	"context"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/traefik/yaegi/internal/dap"
	"github.com/traefik/yaegi/internal/iox"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

type Options struct {
	SrcPath        string
	GoPath         string
	NewInterpreter func(interp.Options) *interp.Interpreter

	StopAtEntry bool
}

type compileFunc func(*interp.Interpreter, string) (*interp.Program, error)

type Adapter struct {
	opts    Options
	compile compileFunc
	arg     string

	session *dap.Session
	ccaps   *dap.InitializeRequestArguments

	interp   *interp.Interpreter
	debugger *interp.Debugger
	event    *interp.DebugEvent

	varRefs variableReferences
}

func NewEvalAdapter(src string, opts *Options) *Adapter {
	return newAdapter((*interp.Interpreter).Compile, src, opts)
}

func NewEvalPathAdapter(path string, opts *Options) *Adapter {
	return newAdapter((*interp.Interpreter).CompilePath, path, opts)
}

func newAdapter(eval compileFunc, arg string, opts *Options) *Adapter {
	if opts == nil {
		opts = new(Options)
	}
	if opts.NewInterpreter == nil {
		opts.NewInterpreter = func(opts interp.Options) *interp.Interpreter {
			i := interp.New(opts)
			i.Use(stdlib.Symbols)
			i.Use(interp.Exports{
				"dbg/dbg": map[string]reflect.Value{
					"Debug": reflect.ValueOf(func() {
						println("!")
					}),
				},
			})
			return i
		}
	}

	a := new(Adapter)
	a.opts = *opts
	a.compile = eval
	a.arg = arg
	return a
}

func (a *Adapter) stdin(b []byte) (int, error) {
	return 0, io.EOF
}

func (a *Adapter) stdout(b []byte) (int, error) {
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: dap.Str("stdout"),
		Output:   string(b),
	})
	return len(b), err
}

func (a *Adapter) stderr(b []byte) (int, error) {
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: dap.Str("stderr"),
		Output:   string(b),
	})
	return len(b), err
}

// Initialize should not be called, as it is only intended
func (a *Adapter) Initialize(s *dap.Session, ccaps *dap.InitializeRequestArguments) *dap.Capabilities {
	a.session, a.ccaps = s, ccaps
	return &dap.Capabilities{
		SupportsConfigurationDoneRequest: dap.Bool(true),
	}
}

// Process should not be called, as it is only intended
func (a *Adapter) Process(m dap.IProtocolMessage) (stop bool) {
	switch m := m.(type) {
	case *dap.Request:
		success := false
		var message string
		var body dap.ResponseBody
		switch m.Command {
		case "launch", "attach":
			a.interp = a.opts.NewInterpreter(interp.Options{
				Stdin:  iox.ReaderFunc(a.stdin),
				Stdout: iox.WriterFunc(a.stdout),
				Stderr: iox.WriterFunc(a.stderr),
				GoPath: a.opts.GoPath,
			})

			prog, err := a.compile(a.interp, a.arg)
			if err == nil {
				success = true
				a.session.Event("initialized", nil)

			} else {
				stop = true
				a.session.Event("output", &dap.OutputEventBody{
					Category: dap.Str("stderr"),
					Output:   err.Error(),
					Data:     err,
				})
				message = fmt.Sprintf("Failed to compile: %v", err)
				break
			}

			a.debugger = a.interp.Debug(context.Background(), prog, func(e *interp.DebugEvent) {
				defer func() {
					r := recover()
					if r != nil {
						fmt.Fprintf(os.Stderr, "Panicked while processing debug event:\n%v\n", r)
					}
				}()

				a.varRefs.Purge()

				if e.Reason() == interp.DebugTerminate {
					a.session.Event("terminated", nil)
					stop = true
					return
				}

				a.event = e
				body := new(dap.StoppedEventBody)
				body.ThreadId = dap.Int(1)
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
				a.session.Event("stopped", body)
			}, nil)

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
				a.debugger.Step(interp.DebugEntry)
			} else {
				a.debugger.Continue()
			}
			success = true

		case "continue":
			a.debugger.Continue()
			success = true

		case "stepIn":
			a.debugger.Step(interp.DebugStepInto)
			success = true

		case "next":
			a.debugger.Step(interp.DebugStepOver)
			success = true

		case "stepOut":
			a.debugger.Step(interp.DebugStepOut)
			success = true

		case "pause":
			a.debugger.Interrupt(interp.DebugPause)
			success = true

		case "threads":
			success = true
			body = &dap.ThreadsResponseBody{
				Threads: []*dap.Thread{
					{Id: 1, Name: "Main"},
				},
			}

		case "stackTrace":
			success = true
			args := m.Arguments.(*dap.StackTraceArguments)
			b := new(dap.StackTraceResponseBody)
			body = b

			b.TotalFrames = dap.Int(a.event.FrameDepth())
			end := b.TotalFrames.Get()
			if args.Levels.GetOr(0) > 0 {
				end = args.StartFrame.GetOr(0) + args.Levels.Get()
			}

			frames := a.event.Frames(args.StartFrame.GetOr(0), end)
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
					Id:     i,
					Name:   f.Name(),
					Line:   pos.Line,
					Column: pos.Column,
					Source: src,
				}
			}

		case "scopes":
			args := m.Arguments.(*dap.ScopesArguments)
			f := a.event.Frame(args.FrameId)
			if f == nil {
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
					VariablesReference: a.varRefs.Add(&frameVars{sc}),
				}
			}

		case "variables":
			args := m.Arguments.(*dap.VariablesArguments)
			scope := a.varRefs.Get(args.VariablesReference)
			if scope == nil {
				message = "Invalid variable reference"
				break
			} else {
				success = true
			}

			body = &dap.VariablesResponseBody{
				Variables: scope.Variables(a),
			}

		case "terminate":
			a.debugger.Interrupt(interp.DebugTerminate)
			success = true

		case "disconnect":
			// Go does not allow forcibly killing a goroutine
			a.debugger.Interrupt(interp.DebugTerminate)
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

		a.session.Respond(m, success, message, body)
	}

	return stop
}

func (a *Adapter) Terminate() {
	a.debugger.Interrupt(interp.DebugTerminate)
}
