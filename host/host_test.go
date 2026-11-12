package host

import (
	"context"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/vm"
)

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
		}
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
	// A host without the AOT copies (vm.Load would run VM mode).
	if err := check(Options(), false); err == nil || !strings.Contains(err.Error(), "import github.com/traefik/yaegi/v2/aot/all") {
		t.Errorf("no AOT copies: got %v", err)
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
