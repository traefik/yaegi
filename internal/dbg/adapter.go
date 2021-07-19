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
}

type compileFunc func(*interp.Interpreter, string) (*interp.Program, error)

type Adapter struct {
	opts    Options
	compile compileFunc
	arg     string

	session *dap.Session
	ccaps   *dap.InitializeRequestArguments
	context context.Context
	cancel  context.CancelFunc

	interp *interp.Interpreter
	prog   *interp.Program
	cont   chan struct{}
	stmt   interp.Statement
	frame  interp.Frame
}

type interpDebugger struct{ *Adapter }

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
	a.context, a.cancel = context.WithCancel(context.Background())
	a.cont = make(chan struct{})
	return a
}

func (a *Adapter) stdin(b []byte) (int, error) {
	return 0, io.EOF
}

func (a *Adapter) stdout(b []byte) (int, error) {
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: "stdout",
		Output:   string(b),
	})
	return len(b), err
}

func (a *Adapter) stderr(b []byte) (int, error) {
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: "stderr",
		Output:   string(b),
	})
	return len(b), err
}

// Initialize should not be called, as it is only intended
func (a *Adapter) Initialize(s *dap.Session, ccaps *dap.InitializeRequestArguments) *dap.Capabilities {
	a.session, a.ccaps = s, ccaps
	return &dap.Capabilities{
		SupportsConfigurationDoneRequest: true,
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
			var err error
			a.interp = a.opts.NewInterpreter(interp.Options{
				Stdin:  iox.ReaderFunc(a.stdin),
				Stdout: iox.WriterFunc(a.stdout),
				Stderr: iox.WriterFunc(a.stderr),
				GoPath: a.opts.GoPath,
			})

			a.prog, err = a.compile(a.interp, a.arg)
			if err == nil {
				success = true
				a.session.Event("initialized", nil)

			} else {
				stop = true
				a.session.Event("output", &dap.OutputEventBody{
					Category: "stderr",
					Output:   err.Error(),
					Data:     err,
				})
				message = fmt.Sprintf("Failed to compile: %v", err)
			}

		case "setBreakpoints":
			success = true

		case "configurationDone":
			a.prog.SetDebugger(&interpDebugger{a})

			success = true
			go func() {
				defer a.session.Event("terminated", new(dap.TerminatedEventBody))
				defer a.cancel()

				_, err := a.interp.ExecuteWithContext(a.context, a.prog)
				if err == nil {
					return
				}

				a.session.Event("output", &dap.OutputEventBody{
					Category: "stderr",
					Output:   err.Error(),
					Data:     err,
				})
			}()

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
			b.StackFrames = make([]*dap.StackFrame, 0, args.Levels)

			for f := a.frame; f != nil; f = f.Previous() {
				b.TotalFrames++
				if len(b.StackFrames) == cap(b.StackFrames) {
					continue
				}

				var src *dap.Source
				pos := a.stmt.Position(a.interp)
				if pos != (token.Position{}) {
					if !a.ccaps.LinesStartAt1 {
						pos.Line--
					}
					if !a.ccaps.ColumnsStartAt1 {
						pos.Column--
					}

					src = new(dap.Source)
					src.Path = pos.Filename
					if src.Path == "_.go" && a.opts.SrcPath != "" {
						src.Path = a.opts.SrcPath
					}
					src.Name = filepath.Base(src.Path)
				}

				b.StackFrames = append(b.StackFrames, &dap.StackFrame{
					Id:     b.TotalFrames,
					Name:   fmt.Sprintf("Frame %d", b.TotalFrames),
					Line:   pos.Line,
					Column: pos.Column,
					Source: src,
				})
			}

		case "scopes":
			success = true
			args := m.Arguments.(*dap.ScopesArguments)
			body = &dap.ScopesResponseBody{
				Scopes: []*dap.Scope{
					{Name: "Frame", PresentationHint: "Locals", VariablesReference: args.FrameId},
				},
			}

		case "variables":
			success = true
			args := m.Arguments.(*dap.VariablesArguments)
			b := &dap.VariablesResponseBody{
				Variables: []*dap.Variable{},
			}
			body = b

			f := a.frame
			for id := args.VariablesReference; f != nil && id > 1; id-- {
				f = f.Previous()
			}
			if f == nil {
				break
			}

			for i, rv := range f.Variables() {
				v := new(dap.Variable)
				b.Variables = append(b.Variables, v)
				v.Name = fmt.Sprint(i)
				v.Value = fmt.Sprint(rv)
				v.Type = fmt.Sprint(rv.Type())
			}

		case "continue", "next", "stepIn", "stepOut", "stepOver":
			success = true
			select {
			case a.cont <- struct{}{}:
			case <-a.context.Done():
			}

		case "pause", "evaluate", "source":
			success = true

		case "terminate":
			success = true
			a.cancel()

		case "disconnect":
			// Go does not allow forcibly killing a goroutine
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

	if stop {
		return true
	}

	select {
	case <-a.context.Done():
		return true
	default:
		return false
	}
}

func (d *interpDebugger) Exec(s interp.Statement, f interp.Frame) {
	d.stmt, d.frame = s, f

	d.session.Event("stopped", &dap.StoppedEventBody{
		Reason:      "breakpoint",
		Description: "Stepping",
		ThreadId:    1,
	})

	select {
	case <-d.cont:
	case <-d.context.Done():
	}
}

func (a *Adapter) Terminate() {
	a.cancel()
}
