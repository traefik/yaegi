package vm

// os/exec for plugins, without the rest of RealProcess (Options.Exec).
//
// By default a restricted plugin may not import os/exec, and an
// Unrestricted one gets no os/exec symbols: only RealProcess gives the real
// package. Options.Exec gives os/exec alone, in one of two modes:
//
//   - ExecMock: the plugin loads and uses the API of os/exec (exec.Command,
//     the fields of Cmd...), but nothing runs: LookPath, os.StartProcess and
//     the methods of *exec.Cmd that start a process (Start, Run, Output,
//     CombinedOutput) return an *ExecError, without starting a process. The
//     machine finds the replacements where it finds the other replaced
//     methods (lookupMethod, restrictedMethodOf), and a native template
//     that names one of these methods is refused (templateRefused).
//   - ExecReal: the real os/exec, and os.StartProcess. The other parts of
//     RealProcess do not come with it: os.Exit still panics, syscall keeps
//     its rules. The command gets the environment of the plugin (its copy,
//     Options.Env) when Cmd.Env is not set, as the plugin sees it.

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"sync"
)

// ExecMode says what a plugin can do with os/exec (Options.Exec).
type ExecMode int

const (
	// ExecRefused is the default: a restricted plugin may not import
	// os/exec, and an Unrestricted plugin without RealProcess gets no
	// symbols of it.
	ExecRefused ExecMode = iota
	// ExecMock lets the plugin import and use os/exec, but every attempt to
	// run a program returns an *ExecError: no process is ever started.
	ExecMock
	// ExecReal gives the plugin the real os/exec (and os.StartProcess):
	// it can run any program with the rights of the host process.
	ExecReal
)

func (e ExecMode) String() string {
	switch e {
	case ExecRefused:
		return "ExecRefused"
	case ExecMock:
		return "ExecMock"
	case ExecReal:
		return "ExecReal"
	}
	return "ExecMode(" + strconv.Itoa(int(e)) + ")"
}

// ExecError is the error of an attempt to run a program in a plugin loaded
// with Options.Exec = ExecMock. errors.Is(err, exec.ErrNotFound) is true, so
// that a plugin that handles a missing command handles this case too.
type ExecError struct {
	Name string // the program
}

func (e *ExecError) Error() string {
	return "exec: " + strconv.Quote(e.Name) + ": running programs is not allowed: the host loaded the plugin with vm.Options.Exec = vm.ExecMock"
}

func (e *ExecError) Unwrap() error { return exec.ErrNotFound }

// cmdType is *exec.Cmd.
var cmdType = reflect.TypeFor[*exec.Cmd]()

// execMethods are the methods of *exec.Cmd that start a process.
var execMethods = []string{"Start", "Run", "Output", "CombinedOutput"}

// execTemplateNames are the methods of *exec.Cmd that a native template
// may not call with ExecMock: execMethods, and Environ, which reads the
// environment of the host process when Cmd.Env is not set (the plugin only
// has its own copy, mockEnviron).
var execTemplateNames = append(execMethods[:len(execMethods):len(execMethods)], "Environ")

// mockEnviron returns the replacement of (*exec.Cmd).Environ for a machine
// with ExecMock: a Cmd whose Env is not set gets the environment copy of
// the plugin (Options.Env), as its os.Environ, not the one of the host.
func mockEnviron(m *Machine) reflect.Value {
	env := m.env
	return reflect.ValueOf(func(c *exec.Cmd) []string {
		if c != nil && c.Env == nil && env != nil {
			c = &exec.Cmd{Env: env.environ(), Dir: c.Dir, SysProcAttr: c.SysProcAttr}
		}
		return c.Environ()
	})
}

// mockCmdMethods are the replacements of execMethods (methodFunc: the
// receiver comes first).
var mockCmdMethods = sync.OnceValue(func() map[string]reflect.Value {
	name := func(c *exec.Cmd) string {
		if c == nil {
			return ""
		}
		if c.Path != "" {
			return c.Path
		}
		if len(c.Args) > 0 {
			return c.Args[0]
		}
		return ""
	}
	return map[string]reflect.Value{
		"Start":          reflect.ValueOf(func(c *exec.Cmd) error { return &ExecError{Name: name(c)} }),
		"Run":            reflect.ValueOf(func(c *exec.Cmd) error { return &ExecError{Name: name(c)} }),
		"Output":         reflect.ValueOf(func(c *exec.Cmd) ([]byte, error) { return nil, &ExecError{Name: name(c)} }),
		"CombinedOutput": reflect.ValueOf(func(c *exec.Cmd) ([]byte, error) { return nil, &ExecError{Name: name(c)} }),
	}
})

// execMethodOf returns the replacement of the method name of the native
// type t for a machine with ExecMock. An unrestricted machine also gets
// the checked Execute methods of the native templates (refusedmeth.go),
// which refuse a template that names a method of execMethods.
func execMethodOf(m *Machine, t reflect.Type, name string) (reflect.Value, bool) {
	if m == nil || m.execMode != ExecMock {
		return reflect.Value{}, false
	}
	if t == cmdType {
		if name == "Environ" {
			return m.execEnviron, true
		}
		fn, ok := mockCmdMethods()[name]
		return fn, ok
	}
	if name == "Execute" || name == "ExecuteTemplate" {
		fn, ok := refusedMethods()[methodKey{t, name}]
		return fn, ok
	}
	return reflect.Value{}, false
}

// execTable returns the os/exec table of a machine without the real
// standard library: nil for ExecRefused.
func (m *Machine) execTable() map[string]reflect.Value {
	m.execOnce.Do(func() {
		real := unrestrictedSyms()["os/exec/exec"]
		if real == nil || m.execMode == ExecRefused {
			return
		}
		syms := maps.Clone(real)
		env := m.env
		withEnv := func(c *exec.Cmd) *exec.Cmd {
			if env != nil && c.Env == nil {
				c.Env = env.environ() // the plugin's environment, not the host's
			}
			return c
		}
		if m.execMode == ExecMock {
			mock := func(c *exec.Cmd, name string) *exec.Cmd {
				c.Err = &ExecError{Name: name}
				return c
			}
			syms["Command"] = reflect.ValueOf(func(name string, arg ...string) *exec.Cmd {
				return mock(&exec.Cmd{Path: name, Args: append([]string{name}, arg...)}, name)
			})
			syms["CommandContext"] = reflect.ValueOf(func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				if ctx == nil {
					panic("nil Context")
				}
				return mock(&exec.Cmd{Path: name, Args: append([]string{name}, arg...)}, name)
			})
			syms["LookPath"] = reflect.ValueOf(func(file string) (string, error) {
				return "", &exec.Error{Name: file, Err: &ExecError{Name: file}}
			})
		} else {
			syms["Command"] = reflect.ValueOf(func(name string, arg ...string) *exec.Cmd {
				return withEnv(exec.Command(name, arg...))
			})
			syms["CommandContext"] = reflect.ValueOf(func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return withEnv(exec.CommandContext(ctx, name, arg...))
			})
		}
		m.execSyms = syms
	})
	return m.execSyms
}

// setupExec installs, for a machine with ExecMock, an os.StartProcess that
// returns an *ExecError. With ExecReal the plugin gets the real one (the
// load-time check allows it: os/exec gives the same power).
func (m *Machine) setupExec() {
	if m.realStd {
		m.execMode = ExecReal // the standard library as in Go
		return
	}
	switch m.execMode {
	case ExecMock:
		m.addOverride("os.StartProcess", func(name string, argv []string, attr *os.ProcAttr) (*os.Process, error) {
			return nil, &os.PathError{Op: "fork/exec", Path: name, Err: &ExecError{Name: name}}
		})
		m.execEnviron = mockEnviron(m) // after SetEnv
	}
}

// execNamesRefused reports whether a native template of the machine may
// not call the methods of execMethods (ExecMock: a *exec.Cmd in the data
// of a native template would start its process with the native method).
func execNamesRefused(m *Machine) bool { return m != nil && m.execMode == ExecMock }
