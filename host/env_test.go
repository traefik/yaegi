package host

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/traefik/yaegi/v2/vm"
)

const (
	envPlug    = "github.com/traefik/yaegi/v2/host/testdata/envplug"
	envSysPlug = "github.com/traefik/yaegi/v2/host/testdata/envsysplug"
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

// TestEnvAllowlist checks the allowlist of the configuration of a plugin:
// names and "PREFIX*", and the entries that would select everything or
// cannot be a name.
func TestEnvAllowlist(t *testing.T) {
	f, err := EnvAllowlist([]string{"PATH", "OIDC_*", "A"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, EnvFilter{Names: []string{"PATH", "A"}, Prefixes: []string{"OIDC_"}}) {
		t.Errorf("filter %+v", f)
	}
	env := []string{"OIDC_ID=1", "PATH=/bin", "HOME=/root", "AB=2", "A=3"}
	if got := f.Filter(env); !reflect.DeepEqual(got, []string{"OIDC_ID=1", "PATH=/bin", "A=3"}) {
		t.Errorf("Filter: %v", got)
	}
	for _, bad := range [][]string{{""}, {"*"}, {"A*B"}, {"A**"}, {"A=B"}, {"OK", "*"}} {
		if _, err := EnvAllowlist(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
		if env, err := PluginEnv(bad); err == nil || env != nil {
			t.Errorf("PluginEnv(%q): %v, %v", bad, env, err)
		}
	}
	t.Setenv("YAEGIENV_ALLOW", "x")
	if env, err := PluginEnv(nil); err != nil || env == nil || len(env) != 0 {
		t.Errorf("PluginEnv(nil): %#v, %v, want an empty, non-nil slice", env, err)
	}
	if env, err := PluginEnv([]string{"YAEGIENV_AL*"}); err != nil || !reflect.DeepEqual(env, []string{"YAEGIENV_ALLOW=x"}) {
		t.Errorf("PluginEnv: %v, %v", env, err)
	}
}

// TestMissingEnv checks the warning of a variable that the host did not
// pass: OnMissingEnv gets its name once per name (never a value), for
// os.Getenv, os.LookupEnv, os.ExpandEnv and syscall.Getenv, and not for a
// variable that the plugin set, unset or cleared itself. Machine.MissingEnv
// lists the same names.
func TestMissingEnv(t *testing.T) {
	t.Setenv("YAEGIENV_A", "host-a")
	t.Setenv("YAEGIENV_SECRET", "host-secret")
	for _, aot := range []bool{true, false} {
		var mu sync.Mutex
		var got []string
		opts := Options()
		opts.Dir, opts.AOT = "..", aot
		opts.OnMissingEnv = func(name string) {
			mu.Lock()
			got = append(got, name)
			mu.Unlock()
		}
		env, err := PluginEnv([]string{"YAEGIENV_A"})
		if err != nil {
			t.Fatal(err)
		}
		SetEnv(&opts, env)
		load := vm.Load // default mode: host.Check refuses it
		if aot {
			load = Load
		}
		p, err := load(opts, envPlug)
		if err != nil {
			t.Fatal(err)
		}
		if v := call(t, p, "Get", "YAEGIENV_A")[0]; v != "host-a" {
			t.Errorf("AOT %v: Getenv of a passed variable: %q", aot, v)
		}
		call(t, p, "Get", "YAEGIENV_SECRET")
		call(t, p, "Get", "YAEGIENV_SECRET") // once per name
		call(t, p, "Lookup", "YAEGIENV_LOOKUP")
		call(t, p, "Expand", "$YAEGIENV_EXPAND")
		// What the plugin sets, unsets or clears itself is not missing.
		call(t, p, "Set", "YAEGIENV_OWN", "v")
		call(t, p, "Unset", "YAEGIENV_OWN")
		call(t, p, "Get", "YAEGIENV_OWN")
		call(t, p, "Unset", "YAEGIENV_A")
		call(t, p, "Get", "YAEGIENV_A")
		want := []string{"YAEGIENV_SECRET", "YAEGIENV_LOOKUP", "YAEGIENV_EXPAND"}
		mu.Lock()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("AOT %v: OnMissingEnv got %v, want %v", aot, got, want)
		}
		mu.Unlock()
		if m := p.M.MissingEnv(); !reflect.DeepEqual(m, want) {
			t.Errorf("AOT %v: MissingEnv %v, want %v", aot, m, want)
		}
		call(t, p, "Clear")
		call(t, p, "Get", "YAEGIENV_AFTER_CLEAR")
		if m := p.M.MissingEnv(); len(m) != len(want) {
			t.Errorf("AOT %v: after Clearenv: %v", aot, m)
		}
		if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
			continue
		}
		opts.UseUnsafe = true
		got = nil
		q, err := load(opts, envSysPlug)
		if err != nil {
			t.Fatal(err)
		}
		call(t, q, "Get", "YAEGIENV_SECRET")
		mu.Lock()
		if !reflect.DeepEqual(got, []string{"YAEGIENV_SECRET"}) {
			t.Errorf("AOT %v: syscall.Getenv: OnMissingEnv got %v", aot, got)
		}
		mu.Unlock()
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
			opts.Dir, opts.AOT = "..", aot
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
			opts.Dir, opts.AOT, opts.UseUnsafe = "..", aot, true
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

// TestMissingEnvMessage checks that the warning quotes the name: the plugin
// chooses it, and a newline or a terminal escape sequence in it must not
// forge a log line of the host.
func TestMissingEnvMessage(t *testing.T) {
	msg := MissingEnvMessage("demo", "FOO\nlevel=error msg=\"forged\"\x1b[2J")
	if strings.ContainsAny(msg, "\n\x1b") {
		t.Errorf("the message holds a raw control character: %q", msg)
	}
	want := `plugin "demo" read env "FOO\nlevel=error msg=\"forged\"\x1b[2J", not passed; add it to envs`
	if msg != want {
		t.Errorf("got %s\nwant %s", msg, want)
	}
	if msg := MissingEnvMessage("demo", vm.MissingEnvMore); !strings.Contains(msg, "not reported") {
		t.Errorf("MissingEnvMore: %s", msg)
	}
}

// TestMissingEnvEnviron checks the note of a plugin that reads its whole
// environment (os.Environ, syscall.Environ) while the host did not pass
// every variable of its process: OnMissingEnv gets vm.MissingEnvEnviron
// once. Such a plugin (jwt-middleware puts os.Environ() into its
// templates) reads no name, so before nothing told the host that it
// missed variables. With the whole host environment (HostEnv), or after
// the plugin cleared its environment itself, there is no note.
func TestMissingEnvEnviron(t *testing.T) {
	t.Setenv("YAEGIENV_A", "host-a")
	t.Setenv("YAEGIENV_SECRET", "host-secret")
	load := func(pkg string, env []string, got *[]string) *vm.Plugin {
		t.Helper()
		opts := Options()
		opts.Dir = ".."
		opts.UseUnsafe = pkg == envSysPlug
		var mu sync.Mutex
		opts.OnMissingEnv = func(name string) {
			mu.Lock()
			*got = append(*got, name)
			mu.Unlock()
		}
		SetEnv(&opts, env)
		p, err := Load(opts, pkg)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	filtered, err := PluginEnv([]string{"YAEGIENV_A"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		env  []string
		want []string
	}{
		{"filtered", filtered, []string{vm.MissingEnvEnviron}},
		{"empty", DefaultEnv(), []string{"YAEGIENV_A", vm.MissingEnvEnviron}}, // YAEGIENV_A: read at init
		{"host", HostEnv(), nil},
	} {
		var got []string
		p := load(envPlug, c.env, &got)
		call(t, p, "Environ", "YAEGIENV_")
		call(t, p, "Environ", "YAEGIENV_") // once
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: OnMissingEnv got %q, want %q", c.name, got, c.want)
		}
		if m := p.M.MissingEnv(); !reflect.DeepEqual(m, c.want) {
			t.Errorf("%s: MissingEnv %q, want %q", c.name, m, c.want)
		}
		if n := p.M.Stats().MissingEnv; n != len(c.want) {
			t.Errorf("%s: Stats.MissingEnv %d, want %d", c.name, n, len(c.want))
		}
	}
	var got []string
	p := load(envPlug, filtered, &got)
	call(t, p, "Clear")
	call(t, p, "Environ", "YAEGIENV_")
	if got != nil {
		t.Errorf("after Clearenv: OnMissingEnv got %q, want nothing", got)
	}
	if msg := MissingEnvMessage("jwt", vm.MissingEnvEnviron); !strings.Contains(msg, "whole environment") {
		t.Errorf("MissingEnvMessage: %q", msg)
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		return
	}
	got = nil
	q := load(envSysPlug, filtered, &got)
	call(t, q, "Environ", "YAEGIENV_")
	if !reflect.DeepEqual(got, []string{vm.MissingEnvEnviron}) {
		t.Errorf("syscall.Environ: OnMissingEnv got %q", got)
	}
}
