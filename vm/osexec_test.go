package vm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestExecOption checks Options.Exec: os/exec is refused by default, the
// error names the option; with ExecMock the plugin loads and every attempt
// to run a program returns an error, with no process started; with
// ExecReal, programs run as in Go.
func TestExecOption(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/restricted/execmodeplug"
	_, err := Load(Options{Dir: ".."}, pkg)
	if err == nil || !strings.Contains(err.Error(), `import "os/exec" is not allowed in a plugin (Options.Exec allows it`) {
		t.Fatalf("default: %v, want the import refused", err)
	}
	_, err = Load(Options{Dir: "..", Unrestricted: true}, pkg)
	if err == nil || !strings.Contains(err.Error(), "os/exec symbols only with Options.Exec") {
		t.Fatalf("Unrestricted: %v, want no os/exec symbols", err)
	}

	call := func(p *Plugin, name string, args ...any) string {
		t.Helper()
		res, err := p.Call(context.Background(), name, args...)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		v, _ := res[0].Interface()
		s, _ := v.(string)
		return s
	}
	unix := runtime.GOOS != "windows" && runtime.GOOS != "plan9"
	env := []string{"YAEGI_EXEC_TEST=1"}
	for _, opts := range []Options{
		{Dir: "..", Exec: ExecMock, Env: env},
		{Dir: "..", Exec: ExecMock, AOT: true, Env: env},
		{Dir: "..", Unrestricted: true, Exec: ExecMock},
		{Dir: "..", Exec: ExecReal, Sandbox: &Sandbox{Env: env}}, // a Sandbox never runs programs
	} {
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatalf("mock %+v: %v", opts, err)
		}
		if got := call(p, "GoOS"); !strings.Contains(got, "running programs is not allowed") || !strings.Contains(got, "vm.ExecMock") {
			t.Errorf("mock GoOS: %q", got)
		}
		if got := call(p, "Errors"); got != "notfound notfound notfound notfound" {
			t.Errorf("mock Errors: %q", got)
		}
		// an interface, a method value, a method expression, reflect and a
		// FuncMap reach the replacements too
		if got := call(p, "Dyn"); !strings.HasPrefix(got, "notfound notfound notfound notfound notfound ") || !strings.Contains(got, "vm.ExecMock") {
			t.Errorf("mock Dyn: %q", got)
		}
		// Cmd.Environ gives the environment the plugin sees, never the
		// host's; a native template may not call it (the AOT copy of
		// text/template, which default mode also uses when the host has
		// it, calls the replacement)
		got := call(p, "Environ")
		if f := strings.Fields(got); len(f) < 3 || f[0] != "env" || f[1] != "env" ||
			!(f[2] == "env" || strings.Contains(got, "calls the method Environ, which is not allowed")) {
			t.Errorf("mock Environ: %q", got)
		}
		// the native template is refused; the AOT copy (the twin, in AOT
		// mode and in default mode when the host has the copies) calls
		// the replacement of Run, which returns the error
		if got := call(p, "Template"); !strings.Contains(got, "calls the method Run, which is not allowed") &&
			!strings.Contains(got, "running programs is not allowed") {
			t.Errorf("mock Template: %q", got)
		}
		if unix {
			file := filepath.Join(t.TempDir(), "x")
			if got := call(p, "Write", file); !strings.Contains(got, "not allowed") {
				t.Errorf("mock Write: %q", got)
			}
			if _, err := os.Stat(file); err == nil {
				t.Errorf("mock Write: the process ran")
			}
		}
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	for _, opts := range []Options{
		{Dir: "..", Exec: ExecReal},
		{Dir: "..", Unrestricted: true, Exec: ExecReal},
		{Dir: "..", Unrestricted: true, RealProcess: true, Exec: ExecMock}, // RealProcess wins
	} {
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatalf("real %+v: %v", opts, err)
		}
		if got := call(p, "GoOS"); got != runtime.GOOS {
			t.Errorf("real %+v GoOS: %q, want %q", opts, got, runtime.GOOS)
		}
		want := "nil nil nil nil"
		if !unix {
			want = "nil other nil nil" // os.StartProcess of /bin/sh, as in Go
		}
		if got := call(p, "Errors"); got != want {
			t.Errorf("real %+v Errors: %q, want %q", opts, got, want)
		}
		if got := call(p, "Dyn"); !strings.HasPrefix(got, "nil nil nil nil nil ran:") {
			t.Errorf("real %+v Dyn: %q", opts, got)
		}
		if unix {
			file := filepath.Join(t.TempDir(), "x")
			if got := call(p, "Write", file); got != "ok" {
				t.Errorf("real Write: %q", got)
			} else if _, err := os.Stat(file); err != nil {
				t.Errorf("real Write: %v", err)
			}
		}
	}
}
