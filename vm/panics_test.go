package vm

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestGoNilFunc checks that "go f()" with a nil f panics at the go
// statement, in the calling goroutine (gc stops there with "go of nil func
// value"), and not later in the new goroutine.
func TestGoNilFunc(t *testing.T) {
	m, spkg := buildSrc(t, `package p
func F() (s string) {
	defer func() {
		if e, ok := recover().(error); ok {
			s = e.Error()
		}
	}()
	var f func()
	go f()
	return "no panic"
}
`)
	got := m.Call(m.funcOf(spkg.Func("F")), nil, nil)
	if s, _ := got.R.(string); s != "go of nil func value" {
		t.Errorf("go of a nil func: got %#v, want the panic %q", got.R, "go of nil func value")
	}
}

// TestRunMainGoexit checks that RunMain returns ErrGoexit, after the
// deferred calls, when main calls runtime.Goexit: it must not end the
// goroutine of its caller.
func TestRunMainGoexit(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n\nimport \"runtime\"\n\nfunc main() {\n\tdefer println(\"deferred\")\n\truntime.Goexit()\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		defer close(done)
		_, err := RunMain(RunOptions{Dir: dir}, filepath.Join(dir, "main.go"))
		done <- err
	}()
	if err, ok := <-done; !ok || !errors.Is(err, ErrGoexit) {
		t.Errorf("RunMain: error %v (returned: %v), want %v", err, ok, ErrGoexit)
	}
}

// TestHostRuntimeErrors: the errors taken from real panics of the host
// runtime when package vm starts are the ones of gc (the negative shift
// is evaluated only for its panic since staticcheck found its sink unused).
func TestHostRuntimeErrors(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{errNegShift, "runtime error: negative shift amount"},
		{errNilMapAssign, "assignment to entry in nil map"},
		{errCloseNilChan, "close of nil channel"},
	} {
		if _, ok := c.err.(runtime.Error); !ok || c.err.Error() != c.want {
			t.Errorf("%T %q, want a runtime.Error %q", c.err, c.err, c.want)
		}
	}
}
