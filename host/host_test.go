package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/vm"
)

// refused returns the *OptionError of err for option, or nil. err must be a
// *CheckError.
func refused(t *testing.T, err error, option string) *OptionError {
	t.Helper()
	var ce *CheckError
	if !errors.As(err, &ce) {
		t.Errorf("%s: %T %v, want a *CheckError", option, err, err)
		return nil
	}
	var le *vm.LoadError
	if errors.As(err, &le) {
		t.Errorf("%s: a *CheckError is also a *vm.LoadError", option)
	}
	for _, p := range ce.Problems {
		if p.Option == option {
			var oe *OptionError
			if !errors.As(err, &oe) {
				t.Errorf("%s: errors.As finds no *OptionError", option)
			}
			return p
		}
	}
	t.Errorf("%s: not refused: %v", option, err)
	return nil
}

// TestCheck checks that Options passes Check (this package imports the AOT
// copies), and that each unsafe change of it is refused with a message that
// names the option.
func TestCheck(t *testing.T) {
	if err := Check(Options()); err != nil {
		t.Fatalf("Options: %v", err)
	}
	o := Options()
	if !o.StubAssembly || o.UseUnsafe {
		t.Errorf("StubAssembly %v, UseUnsafe %v: want true, false (the host sets UseUnsafe from its configuration)", o.StubAssembly, o.UseUnsafe)
	}
	for _, c := range []struct {
		want   string
		change func(o *vm.Options)
	}{
		{"MaxStack is negative", func(o *vm.Options) { o.MaxStack = -1 }},
		{"Sandbox.Budget is 0", func(o *vm.Options) { o.Sandbox = &vm.Sandbox{MaxGoroutines: 10} }},
		{"Sandbox.MaxGoroutines is 0", func(o *vm.Options) { o.Sandbox = &vm.Sandbox{Budget: 10} }},
		{"Sandbox.Budget is 0 or negative", func(o *vm.Options) { o.Sandbox = &vm.Sandbox{Budget: -1, MaxGoroutines: 10} }},
		{"LoadLimits.MaxTotalSourceBytes is 0", func(o *vm.Options) { o.LoadLimits = &vm.LoadLimits{MaxFiles: 1, MaxPackages: 1} }},
		{"LoadLimits.MaxFiles is 0", func(o *vm.Options) { o.LoadLimits = &vm.LoadLimits{MaxTotalSourceBytes: 1, MaxPackages: 1} }},
		{"LoadLimits.MaxPackages is 0", func(o *vm.Options) { o.LoadLimits = &vm.LoadLimits{MaxTotalSourceBytes: 1, MaxFiles: 1} }},
		{"GoList is on", func(o *vm.Options) { o.GoList = true }},
		{"Src is set", func(o *vm.Options) { o.Src = []string{"strings"} }},
		{"AOT is off", func(o *vm.Options) { o.AOT = false }},
		{"Unrestricted", func(o *vm.Options) { o.Unrestricted = true }},
		{"RealProcess", func(o *vm.Options) { o.RealProcess = true }},
		{"ExecReal", func(o *vm.Options) { o.Exec = vm.ExecReal }},
		{"Budget is 0", func(o *vm.Options) { o.Budget = 0 }},
		{"BackgroundRate is 0", func(o *vm.Options) { o.BackgroundRate = 0 }},
		{"MaxGoroutines is 0", func(o *vm.Options) { o.MaxGoroutines = 0 }},
		{"MaxMemory is 0", func(o *vm.Options) { o.MaxMemory = 0 }},
		{"LoadTimeout is 0", func(o *vm.Options) { o.LoadTimeout = 0 }},
		{"Budget is 0 or negative", func(o *vm.Options) { o.Budget = -1 }},
		{"BudgetRate is 0 or negative", func(o *vm.Options) { o.BudgetRate = 0 }},
		{"BudgetRate is 0 or negative", func(o *vm.Options) { o.BudgetRate = -1 }},
		{"BackgroundRate is 0 or negative", func(o *vm.Options) { o.BackgroundRate = -1 }},
		{"MaxGoroutines is 0 or negative", func(o *vm.Options) { o.MaxGoroutines = -1 }},
		{"MaxMemory is 0 or negative", func(o *vm.Options) { o.MaxMemory = -1 }},
		{"LoadTimeout is 0 or negative", func(o *vm.Options) { o.LoadTimeout = -1 }},
		{`Modes["log/slog"] is ModeNative`, func(o *vm.Options) { o.Modes = map[string]vm.Mode{"log/slog": vm.ModeNative} }},
		{`Modes["fmt"] is ModeNative, but fmt has an AOT copy`, func(o *vm.Options) { o.Modes = map[string]vm.Mode{"fmt": vm.ModeNative} }},
		{"AllowStaleAOT is on", func(o *vm.Options) { o.AllowStaleAOT = true }},
		{"NoPreflight is on", func(o *vm.Options) { o.NoPreflight = true }},
		{"Env is nil", func(o *vm.Options) { o.Env = nil }},
		{"PanicOnFatal is off", func(o *vm.Options) { o.PanicOnFatal = false }},
	} {
		o := Options()
		c.change(&o)
		err := Check(o)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v", c.want, err)
			continue
		}
		// The option is the start of the message ("Budget is 0...").
		option, _, _ := strings.Cut(c.want, " ")
		if c.want == "ExecReal" {
			option = "Exec"
		}
		if p := refused(t, err, option); p != nil && !strings.Contains(p.Reason, c.want) {
			t.Errorf("%s: reason %q", option, p.Reason)
		}
	}
	// A Sandbox with limits, and with or without an environment, passes:
	// the catalog host loads with one (reloadsandbox).
	o = Options()
	o.Sandbox = &vm.Sandbox{Budget: Budget, MaxGoroutines: MaxGoroutines}
	o.LoadLimits = &vm.DefaultLoadLimits
	if err := Check(o); err != nil {
		t.Errorf("Sandbox with limits: %v", err)
	}
	// The modes that AOT mode gives anyway are accepted.
	o = Options()
	o.Modes = map[string]vm.Mode{"flag": vm.ModeNative, "log/slog": vm.ModeAOT, "fmt": vm.ModeDefault}
	if err := Check(o); err != nil {
		t.Errorf("default modes: %v", err)
	}
	// ModeNative is accepted for a package without an AOT copy: it runs
	// natively in AOT mode anyway.
	o = Options()
	o.Modes = map[string]vm.Mode{"golang.org/x/sys/unix": vm.ModeNative, "example.com/hostmod/native": vm.ModeNative, "os": vm.ModeNative}
	if err := Check(o); err != nil {
		t.Errorf("ModeNative without an AOT copy: %v", err)
	}
	// A host without the AOT copies (vm.Load would run VM mode): refused,
	// also with StaleAOTFallback.
	if err := check(Options(), false, nil); err == nil || !strings.Contains(err.Error(), "import github.com/traefik/yaegi/v2/aot/all") {
		t.Errorf("no AOT copies: got %v", err)
	} else {
		refused(t, err, "AOT")
	}
}

// TestCheckStaleAOT checks a host built with a newer Go patch release than
// the AOT copies (vm.CheckAOT fails): Options (StaleAOTFallback) passes,
// since the load falls back to default mode with the native standard
// library; without StaleAOTFallback it is refused, with the error of
// CheckAOT in the chain. Before, Check refused every plugin there, even with
// StaleAOTFallback, and a host that disables every plugin on the first error
// (Traefik) lost all of them at each Go patch release.
func TestCheckStaleAOT(t *testing.T) {
	stale := fmt.Errorf("vm: the AOT copies were made with go1.26.0 and the host is built with go1.26.1")
	o := Options()
	if !o.StaleAOTFallback || o.AllowStaleAOT {
		t.Fatalf("StaleAOTFallback %v, AllowStaleAOT %v: want true, false", o.StaleAOTFallback, o.AllowStaleAOT)
	}
	if err := check(o, true, stale); err != nil {
		t.Errorf("stale copies with StaleAOTFallback: %v", err)
	}
	o.StaleAOTFallback = false
	err := check(o, true, stale)
	if p := refused(t, err, "AOT"); p != nil && !errors.Is(err, stale) {
		t.Errorf("stale copies without StaleAOTFallback: the CheckAOT error is not in the chain: %v", err)
	}
}

// TestOptionsLimits checks the limits of the host profile that Check alone
// does not demand: MaxStack 256 MiB (the vm default is 1 GiB, all of
// MaxMemory), and an empty, non-nil environment.
func TestOptionsLimits(t *testing.T) {
	o := Options()
	if o.MaxStack != 256<<20 {
		t.Errorf("MaxStack %d, want %d (256 MiB)", o.MaxStack, 256<<20)
	}
	if o.Env == nil || len(o.Env) != 0 {
		t.Errorf("Env %#v, want an empty, non-nil slice", o.Env)
	}
}

// TestLoad checks that Load refuses unsafe options before it loads
// anything, and loads a plugin with Options.
func TestLoad(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/isoplug"
	o := Options()
	o.Dir = ".."
	o.AOT = false
	if _, err := Load(o, pkg); err == nil || !strings.Contains(err.Error(), "AOT is off") {
		t.Errorf("VM mode: got %v", err)
	}
	o.AOT = true
	p, err := LoadContext(context.Background(), o, pkg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call(context.Background(), "CreateConfig"); err != nil {
		t.Error(err)
	}
}

// TestLoadErrors checks the type of each error of Load: a
// *CheckError for unsafe options (before anything is read), a *vm.LoadError
// for a plugin that does not load (a type error, no go.mod), so that a host
// can fall back to another engine on the second kind only.
func TestLoadErrors(t *testing.T) {
	write := func(dir, name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	write(dir, "go.mod", "module example.com/p\n\ngo 1.22\n")
	write(dir, "p.go", "package p\n\nvar X = undefinedName\n")
	o := Options()
	o.Dir, o.Root = dir, dir
	var le *vm.LoadError
	if _, err := Load(o, "example.com/p"); !errors.As(err, &le) {
		t.Errorf("type error: %T %v, want a *vm.LoadError", err, err)
	} else if _, ok := err.(*vm.LoadError); !ok {
		t.Errorf("type error: %T, want a *vm.LoadError at the top", err)
	}
	noMod := t.TempDir()
	write(noMod, "p.go", "package p\n")
	o.Dir, o.Root = noMod, noMod
	if _, err := Load(o, "example.com/p"); !errors.As(err, &le) {
		t.Errorf("no go.mod: %T %v, want a *vm.LoadError", err, err)
	}
	o.Budget = 0
	if _, err := Load(o, "example.com/p"); err != nil {
		refused(t, err, "Budget")
	} else {
		t.Error("Budget 0: loaded")
	}

}
