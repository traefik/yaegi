package dbg

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/traefik/yaegi/internal/dap"
	"github.com/traefik/yaegi/internal/iox"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

type Options struct {
	GoPath         string
	NewInterpreter func(interp.Options) *interp.Interpreter
}

type evalFunc func(*interp.Interpreter, context.Context, string) (reflect.Value, error)

type Adapter struct {
	opts    Options
	eval    evalFunc
	arg     string
	session *dap.Session
	context context.Context
	cancel  context.CancelFunc
}

func NewEvalAdapter(src string, opts *Options) *Adapter {
	return newAdapter((*interp.Interpreter).EvalWithContext, src, opts)
}

func NewEvalPathAdapter(path string, opts *Options) *Adapter {
	return newAdapter((*interp.Interpreter).EvalPathWithContext, path, opts)
}

func newAdapter(eval evalFunc, arg string, opts *Options) *Adapter {
	if opts == nil {
		opts = new(Options)
	}
	if opts.NewInterpreter == nil {
		opts.NewInterpreter = func(opts interp.Options) *interp.Interpreter {
			i := interp.New(opts)
			i.Use(stdlib.Symbols)
			return i
		}
	}

	a := new(Adapter)
	a.opts = *opts
	a.eval = eval
	a.arg = arg
	a.context = context.Background()
	a.cancel = func() {}
	return a
}

func (a *Adapter) launch() {
	a.context, a.cancel = context.WithCancel(context.Background())

	interp := a.opts.NewInterpreter(interp.Options{
		Stdin:  iox.ReaderFunc(a.stdin),
		Stdout: iox.WriterFunc(a.stdout),
		Stderr: iox.WriterFunc(a.stderr),
		GoPath: a.opts.GoPath,
	})

	go func() {
		defer a.session.Event("terminated", new(dap.TerminatedEventBody))
		defer a.cancel()

		_, err := a.eval(interp, a.context, a.arg)
		if err == nil {
			return
		}

		if e, ok := err.(interface{ Unwrap() error }); ok {
			err = e.Unwrap()
		}
		fmt.Fprintf(os.Stderr, "%T\n", err)

		a.session.Event("output", &dap.OutputEventBody{
			Category: "stderr",
			Output:   err.Error(),
			Data:     err,
		})
	}()
}

func (a *Adapter) stdin(b []byte) (int, error) {
	return 0, io.EOF
}

func (a *Adapter) stdout(b []byte) (int, error) {
	fmt.Fprintf(os.Stderr, "stdout %d byte(s)\n", len(b))
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: "stdout",
		Output:   string(b),
	})
	return len(b), err
}

func (a *Adapter) stderr(b []byte) (int, error) {
	fmt.Fprintf(os.Stderr, "stderr %d byte(s)\n", len(b))
	err := a.session.Event("output", &dap.OutputEventBody{
		Category: "stderr",
		Output:   string(b),
	})
	return len(b), err
}

// Initialize should not be called, as it is only intended
func (a *Adapter) Initialize(s *dap.Session, ccaps *dap.InitializeRequestArguments) *dap.Capabilities {
	a.session = s
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
		case "launch":
			success = true
			a.session.Event("initialized", nil)

		case "configurationDone":
			success = true
			a.launch()

		case "threads":
			success = true
			body = &dap.ThreadsResponseBody{
				Threads: []*dap.Thread{
					{Id: 1, Name: "Main"},
				},
			}

		case "terminate":
			success = true
			a.cancel()

		case "disconnect":
			// Go does not allow forcibly killing a goroutine
			stop = true
			success = true

		default:
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

func (a *Adapter) Terminate() {
	a.cancel()
}
