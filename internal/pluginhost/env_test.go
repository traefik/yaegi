package pluginhost

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	_ "github.com/traefik/yaegi/v2/aot/all" // AOT copies, for the AOT mode of TestPluginEnv
	"github.com/traefik/yaegi/v2/vm"
)

const (
	envPlug    = "github.com/traefik/yaegi/v2/internal/pluginhost/testdata/envplug"
	envSysPlug = "github.com/traefik/yaegi/v2/internal/pluginhost/testdata/envsysplug"
)

// call calls the function name of p and returns its results as native values.
func call(t *testing.T, p *vm.Plugin, name string, args ...any) []any {
	t.Helper()
	res, err := p.Call(context.Background(), name, args...)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := make([]any, len(res))
	for i, r := range res {
		if out[i], err = r.Interface(); err != nil {
			t.Fatalf("%s: result %d: %v", name, i, err)
		}
	}
	return out
}

// TestEnvFilter checks the selection of EnvFilter, and that it never gives a
// nil slice (a nil vm.Options.Env is the whole host environment).
func TestEnvFilter(t *testing.T) {
	env := []string{"OIDC_ID=1", "OIDC_SECRET=s", "PATH=/bin", "HOME=/root", "OIDC=x", "=C:=C:\\", "NOEQ"}
	for _, c := range []struct {
		f    EnvFilter
		want []string
	}{
		{EnvFilter{}, []string{}},
		{EnvFilter{Names: []string{"PATH", "MISSING"}}, []string{"PATH=/bin"}},
		{EnvFilter{Prefixes: []string{"OIDC_"}}, []string{"OIDC_ID=1", "OIDC_SECRET=s"}},
		{EnvFilter{Names: []string{"HOME"}, Prefixes: []string{"OIDC"}}, []string{"OIDC_ID=1", "OIDC_SECRET=s", "HOME=/root", "OIDC=x"}},
		// An empty prefix (an empty entry in the settings) selects nothing:
		// it selected every variable, all the secrets of the host.
		{EnvFilter{Prefixes: []string{""}}, []string{}},
		{EnvFilter{Names: []string{"PATH"}, Prefixes: []string{"", "OIDC_"}}, []string{"OIDC_ID=1", "OIDC_SECRET=s", "PATH=/bin"}},
	} {
		got := c.f.Filter(env)
		if got == nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: got %#v, want %#v", c.f, got, c.want)
		}
	}
	if got := (EnvFilter{Names: []string{"YAEGIENV_NONE"}}).Env(); got == nil || len(got) != 0 {
		t.Errorf("Env with no match: %#v, want an empty, non-nil slice", got)
	}
	var o vm.Options
	SetEnv(&o, nil)
	if o.Env == nil || len(o.Env) != 0 {
		t.Errorf("SetEnv(nil): Env %#v, want an empty, non-nil slice", o.Env)
	}
	o.Sandbox = &vm.Sandbox{}
	SetEnv(&o, []string{"A=1"})
	if !reflect.DeepEqual(o.Env, []string{"A=1"}) || !reflect.DeepEqual(o.Sandbox.Env, []string{"A=1"}) {
		t.Errorf("SetEnv with a Sandbox: Env %v, Sandbox.Env %v", o.Env, o.Sandbox.Env)
	}
}

// TestPluginEnv loads two plugins at once with the environments a host can
// give (HostEnv, as Traefik with yaegi; an EnvFilter; the empty default of
// Options), in AOT mode and default mode, and checks every function of
// package os that works on the environment: what the plugins read, and
// that what one plugin changes changes neither the host nor the other
// plugin.
func TestPluginEnv(t *testing.T) {
	t.Setenv("YAEGIENV_A", "host-a")
	t.Setenv("YAEGIENV_SECRET", "host-secret")
	t.Setenv("OTHER_YAEGIENV", "other")
	hostEnv := os.Environ()
	for _, aot := range []bool{true, false} {
		mode := map[bool]string{true: "aot", false: "default"}[aot]
		load := func(env []string) *vm.Plugin {
			t.Helper()
			opts := Options()
			opts.Dir, opts.AOT = "../..", aot
			if env != nil {
				SetEnv(&opts, env)
			}
			p, err := vm.Load(opts, envPlug)
			if err != nil {
				t.Fatalf("%s: %v", mode, err)
			}
			return p
		}
		t.Run(mode+"/default", func(t *testing.T) {
			p := load(nil) // Options: no variable of the host
			if got := call(t, p, "Get", "YAEGIENV_A")[0]; got != "" {
				t.Errorf("Getenv: %q, want \"\"", got)
			}
			if got := call(t, p, "Environ", "")[1]; got != 0 {
				t.Errorf("Environ: %v entries, want 0", got)
			}
		})
		t.Run(mode+"/host", func(t *testing.T) {
			p, q := load(HostEnv()), load(HostEnv())
			if got := call(t, p, "AtInit")[0]; got != "host-a" {
				t.Errorf("Getenv at init: %q, want host-a", got)
			}
			for _, f := range []string{"Get", "GetValue"} {
				if got := call(t, p, f, "YAEGIENV_SECRET")[0]; got != "host-secret" {
					t.Errorf("%s: %q, want host-secret", f, got)
				}
			}
			if got := call(t, p, "Lookup", "YAEGIENV_A"); got[0] != "host-a" || got[1] != true {
				t.Errorf("LookupEnv: %v, want [host-a true]", got)
			}
			if got := call(t, p, "Lookup", "YAEGIENV_MISSING"); got[0] != "" || got[1] != false {
				t.Errorf("LookupEnv of a missing variable: %v", got)
			}
			got := call(t, p, "Environ", "YAEGIENV_")
			if !reflect.DeepEqual(got[0], []string{"YAEGIENV_A=host-a", "YAEGIENV_SECRET=host-secret"}) || got[1] != len(hostEnv) {
				t.Errorf("Environ: %v, want the 2 YAEGIENV_ variables of %d", got, len(hostEnv))
			}
			if got := call(t, p, "Expand", "${YAEGIENV_A}/$YAEGIENV_MISSING."); got[0] != "host-a/." {
				t.Errorf("ExpandEnv: %q", got[0])
			}

			// Changes stay in the copy of p.
			if err := call(t, p, "Set", "YAEGIENV_A", "plugin-a")[0]; err != nil {
				t.Fatalf("Setenv: %v", err)
			}
			if err := call(t, p, "Set", "YAEGIENV_NEW", "new")[0]; err != nil {
				t.Fatalf("Setenv: %v", err)
			}
			if err := call(t, p, "Set", "BAD=KEY", "x")[0]; err == nil {
				t.Errorf("Setenv with a '=' in the key: no error")
			}
			if err := call(t, p, "Unset", "YAEGIENV_SECRET")[0]; err != nil {
				t.Fatalf("Unsetenv: %v", err)
			}
			if got := call(t, p, "Expand", "$YAEGIENV_A $YAEGIENV_NEW [$YAEGIENV_SECRET]")[0]; got != "plugin-a new []" {
				t.Errorf("after Setenv and Unsetenv: %q", got)
			}
			if got := call(t, p, "Environ", "YAEGIENV_")[0]; !reflect.DeepEqual(got, []string{"YAEGIENV_A=plugin-a", "YAEGIENV_NEW=new"}) {
				t.Errorf("Environ after Setenv and Unsetenv: %v", got)
			}
			if os.Getenv("YAEGIENV_A") != "host-a" || os.Getenv("YAEGIENV_SECRET") != "host-secret" {
				t.Errorf("the plugin changed the host environment")
			}
			if _, ok := os.LookupEnv("YAEGIENV_NEW"); ok {
				t.Errorf("the plugin set a variable of the host environment")
			}
			if got := call(t, q, "Expand", "$YAEGIENV_A [$YAEGIENV_NEW] $YAEGIENV_SECRET")[0]; got != "host-a [] host-secret" {
				t.Errorf("the other plugin sees the changes of the first: %q", got)
			}

			call(t, p, "Clear")
			if got := call(t, p, "Environ", "")[1]; got != 0 {
				t.Errorf("Environ after Clearenv: %v entries, want 0", got)
			}
			if !reflect.DeepEqual(os.Environ(), hostEnv) {
				t.Errorf("Clearenv in the plugin changed the host environment")
			}
			if got := call(t, q, "Environ", "")[1]; got != len(hostEnv) {
				t.Errorf("Clearenv in a plugin cleared the other plugin: %v entries", got)
			}
		})
		t.Run(mode+"/syscall", func(t *testing.T) {
			if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
				t.Skip("the syscall subset of useUnsafe is for unix")
			}
			opts := Options()
			opts.Dir, opts.AOT, opts.UseUnsafe = "../..", aot, true
			SetEnv(&opts, EnvFilter{Prefixes: []string{"YAEGIENV_A"}}.Env())
			p, err := vm.Load(opts, envSysPlug)
			if err != nil {
				t.Fatal(err)
			}
			if got := call(t, p, "Get", "YAEGIENV_A"); got[0] != "host-a" || got[1] != true {
				t.Errorf("syscall.Getenv: %v, want [host-a true]", got)
			}
			if got := call(t, p, "Get", "YAEGIENV_SECRET"); got[1] != false {
				t.Errorf("syscall.Getenv of a variable the filter leaves out: %v", got)
			}
			if got := call(t, p, "Environ", "YAEGIENV_")[0]; !reflect.DeepEqual(got, []string{"YAEGIENV_A=host-a"}) {
				t.Errorf("syscall.Environ: %v", got)
			}
		})
		t.Run(mode+"/filter", func(t *testing.T) {
			p := load(EnvFilter{Names: []string{"YAEGIENV_A"}, Prefixes: []string{"OTHER_"}}.Env())
			if got := call(t, p, "Expand", "$YAEGIENV_A [$YAEGIENV_SECRET] $OTHER_YAEGIENV")[0]; got != "host-a [] other" {
				t.Errorf("ExpandEnv: %q", got)
			}
			got := call(t, p, "Environ", "")
			if all, _ := got[0].([]string); !reflect.DeepEqual(all, []string{"OTHER_YAEGIENV=other", "YAEGIENV_A=host-a"}) {
				t.Errorf("Environ: %v, want the 2 selected variables", got)
			}
			if got := call(t, p, "Lookup", "YAEGIENV_SECRET"); got[1] != false {
				t.Errorf("LookupEnv of a variable the filter leaves out: %v", got)
			}
		})
	}
	if strings.Join(os.Environ(), "\n") != strings.Join(hostEnv, "\n") {
		t.Errorf("the host environment changed")
	}
}
