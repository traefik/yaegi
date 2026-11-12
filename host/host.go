// Package host is the entry point of a host that loads plugins it does not
// trust, such as Traefik plugins. It holds the safe options in one place
// (Options), refuses a set of options that is not safe (Check), and loads
// plugins only after that check (Load, LoadContext).
//
// It imports the AOT copies (github.com/traefik/yaegi/v2/aot/all): a host that
// imports this package cannot forget them. With package vm alone, Options.AOT
// without the copies silently runs VM mode. Package vm stays permissive (the
// yaegi command, the tests); this package is the strict one.
package host

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	// The AOT copies: Options is AOT mode, which needs them.
	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/vm"
)

// The limits of Options. They are the only copy of these values: other
// packages (internal/pluginhost) read them from here.
const (
	// Budget is the Budget of Options: steps (interpreted calls and
	// backward jumps) per call from the host. Every plugin of the catalog
	// runs the same with a budget 100 times smaller (see docs/SANDBOX.md).
	// A `for {}` stops after about 0.2 s.
	Budget = 100_000_000
	// BackgroundRate is the BackgroundRate of Options: the steps per second
	// of all the background goroutines of a plugin together (cache
	// janitors, refresh loops). A loop runs about 5e8 steps per second on
	// one core, so a plugin whose background goroutines spin uses about a
	// tenth of a core; a refresh loop that works for a few milliseconds per
	// period is not slowed.
	BackgroundRate = 50_000_000
	// MaxMemory is the MaxMemory of Options (1 GiB): far above what the
	// plugins of the catalog keep; it stops a plugin that keeps large
	// buffers until the host runs out of memory.
	MaxMemory = 1 << 30
	// MaxGoroutines is the MaxGoroutines of Options.
	MaxGoroutines = 1000
	// LoadTimeout is the LoadTimeout of Options.
	LoadTimeout = 30 * time.Second
)

// Options returns the options of a host that loads plugins it does not
// trust: AOT mode, the limits above, no variable of the host environment,
// PanicOnFatal (a plugin that unlocks an unlocked mutex or writes a map from
// two goroutines does not end the host), StubAssembly (zap and grpc load as
// restricted plugins; a call of a function the VM cannot run, written in
// assembly or a system call through the C library, fails at run time, and
// the VM never runs it), and every option that accepts what yaegi accepted
// (Traefik plugins were written for yaegi): unused names, unused expression
// values, the test files and the identical copies of a declaration left
// out, the "// yaegi:tags" lines, a go directive newer than the host, the
// packages of the archive that a nested module does not vendor, and a
// vendor directory that does not match go.mod. The restriction level is the
// default one (restricted).
//
// The host then sets Dir, Root, UseUnsafe (from its own configuration,
// never from plugin data), Env (the variables the plugin needs), Stdout,
// Stderr and GoPanic. Each call returns a new value.
func Options() vm.Options {
	return vm.Options{
		AOT:                         true,
		Budget:                      Budget,
		BackgroundRate:              BackgroundRate,
		PanicOnFatal:                true,
		MaxMemory:                   MaxMemory,
		MaxGoroutines:               MaxGoroutines,
		LoadTimeout:                 LoadTimeout,
		Env:                         []string{},
		IgnoreSoftErrors:            true,
		IgnoreUnusedExprs:           true,
		IgnoreTestFiles:             true,
		IgnoreDuplicateDecls:        true,
		YaegiTags:                   true,
		AllowNewerGo:                true,
		ImportFromArchive:           true,
		IgnoreInconsistentVendoring: true,
		StubAssembly:                true,
	}
}

// Check returns an error when o is not safe for plugins the host does not
// trust, or nil. It refuses:
//   - VM mode (AOT false): native fmt and text/template see a value of the
//     plugin as a native copy without its methods, which is not always
//     checked, so a result can be silently wrong (README, "Execution
//     modes");
//   - AOT mode without the AOT copies, or with copies that cannot run in
//     this host (vm.CheckAOT);
//   - Unrestricted, RealProcess, and Exec set to vm.ExecReal: they give the
//     plugin the host process;
//   - a zero or negative limit (Budget, BackgroundRate, MaxGoroutines,
//     MaxMemory, LoadTimeout): the vm takes it as no limit;
//   - ModeNative in Modes for a package that has an AOT copy (except flag):
//     the native package behaves as in VM mode, above;
//   - AllowStaleAOT: the copies can miss the security fixes of the newer
//     patch releases of Go;
//   - NoPreflight: code the VM cannot run panics in the middle of a request
//     instead of failing the load;
//   - a nil Env (a copy of the whole host environment: its secrets) and
//     PanicOnFatal off.
//
// Options passes it. Load and LoadContext call it.
func Check(o vm.Options) error {
	return check(o, len(vm.AOTGoVersions()) > 0)
}

// check is Check; copies tells whether the host has AOT copies.
func check(o vm.Options, copies bool) error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	switch {
	case !o.AOT:
		bad("AOT is off: plugins must run in AOT mode")
	case !copies:
		bad("AOT is on, but the host has no AOT copies: import github.com/traefik/yaegi/v2/aot/all")
	default:
		if err := vm.CheckAOT(o.AllowStaleAOT); err != nil {
			errs = append(errs, err)
		}
	}
	if o.Unrestricted {
		bad("Unrestricted is on: the plugin is not restricted")
	}
	if o.RealProcess {
		bad("RealProcess is on: the plugin can end the host and run programs")
	}
	if o.Exec == vm.ExecReal {
		bad("Exec is ExecReal: the plugin can run programs")
	}
	for _, l := range []struct {
		name string
		off  bool
	}{
		{"Budget", o.Budget <= 0},
		{"BackgroundRate", o.BackgroundRate <= 0},
		{"MaxGoroutines", o.MaxGoroutines <= 0},
		{"MaxMemory", o.MaxMemory <= 0},
		{"LoadTimeout", o.LoadTimeout <= 0},
	} {
		if l.off {
			bad("%s is 0 or negative: no limit", l.name)
		}
	}
	if o.AllowStaleAOT {
		bad("AllowStaleAOT is on: the AOT copies can miss the security fixes of the Go version of the host")
	}
	if o.NoPreflight {
		bad("NoPreflight is on: code the VM cannot run panics in the middle of a request instead of failing the load")
	}
	for _, p := range slices.Sorted(maps.Keys(o.Modes)) {
		// With no explicit modes, ModeFor gives ModeAOT only to a package
		// that has an AOT copy. A package without a copy (a std package
		// such as os, golang.org/x/sys/unix, a native module package of
		// the host) runs natively in AOT mode anyway.
		if o.Modes[p] == vm.ModeNative && vm.ModeFor(nil, true, p) == vm.ModeAOT && !nativeOK[p] {
			bad("Modes[%q] is ModeNative, but %s has an AOT copy: the native package sees a value of the plugin without its methods, as in VM mode", p, p)
		}
	}
	if o.Env == nil {
		bad("Env is nil: the plugin gets a copy of the whole host environment")
	}
	if !o.PanicOnFatal {
		bad("PanicOnFatal is off: a plugin that unlocks an unlocked mutex ends the host")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("host: options not safe for untrusted plugins: %w", errors.Join(errs...))
}

// nativeOK lists the packages with an AOT copy that a host can still run
// natively: a restricted plugin runs flag natively in AOT mode too, with a
// flag set of its own (vm.stdAOTModes).
var nativeOK = map[string]bool{"flag": true}

// Load is vm.Load after Check: it refuses options that are not safe.
func Load(opts vm.Options, pkgPath string) (*vm.Plugin, error) {
	return LoadContext(context.Background(), opts, pkgPath)
}

// LoadContext is vm.LoadContext after Check: it refuses options that are
// not safe.
func LoadContext(ctx context.Context, opts vm.Options, pkgPath string) (*vm.Plugin, error) {
	if err := Check(opts); err != nil {
		return nil, err
	}
	return vm.LoadContext(ctx, opts, pkgPath)
}
