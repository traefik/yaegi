// Package host is the entry point of a host that loads plugins it does not
// trust, such as Traefik plugins. It holds the safe options in one place
// (Options), refuses a set of options that is not safe (Check, a
// *CheckError), loads plugins only after that check (Load, LoadContext),
// selects the environment of a plugin (DefaultEnv, HostEnv, EnvFilter,
// SetEnv: env.go), and calls the entry points of a Traefik plugin
// (LoadKind, CheckEntryPoints, CreateConfig, NewInstanceHandler,
// NewProvider, CallMethod: plugin.go).
//
// It imports the AOT copies (github.com/traefik/yaegi/v2/aot/all): a host that
// imports this package cannot forget them. With package vm alone, Options.AOT
// without the copies silently runs VM mode. Package vm stays permissive (the
// yaegi command, the tests); this package is the strict one.
//
// Call plugin code only through vm.Plugin.Call, vm.Object.Call or the
// handler that New returns: they return the panics and the errors of the
// plugin (vm.ErrBudget, vm.ErrStackOverflow, vm.ErrGoexit). A func value of
// the plugin (from vm.Object.Interface) that the host calls directly on
// its own goroutine follows the rule of the native callbacks: after a
// panic or the end of the budget (given to Options.GoPanic), a func whose
// results are not just an error ends the goroutine of the host with
// runtime.Goexit. See the doc of vm.Object.Interface and docs/SANDBOX.md.
package host

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	// The AOT copies: Options is AOT mode, which needs them.
	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/vm"
)

// The limits of Options. They are the only copy of these values: other
// packages (internal/pluginhost) read them from here.
const (
	// Budget is the Budget of Options: the steps (interpreted calls and
	// backward jumps, more for a native call or a select) that a call from
	// the host may use at once, the size of its bucket (see BudgetRate).
	// Every plugin of the catalog runs the same with a budget 100 times
	// smaller (see docs/SANDBOX.md). A `for {}` stops after about 0.3 s.
	Budget = 100_000_000
	// BudgetRate is the BudgetRate of Options: the steps per second that
	// the bucket of a call from the host gets back. A call that uses on
	// average less runs for ever (a long stream, a large download through
	// a wrapper of the ResponseWriter that reads each byte); a loop runs
	// about 4e8 steps per second on one core, so `for {}` still empties the
	// bucket in about 0.3 s, and a loop that never waits stops after
	// Budget steps whatever its speed.
	BudgetRate = 100_000_000
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
	// MaxStack is the MaxStack of Options: the stack of one goroutine of
	// the plugin (registers, frame-local variables, deferred calls), past
	// which it stops with vm.ErrStackOverflow (256 MiB, a decision of the
	// host profile). The vm package keeps 1 GiB, as gc, for a host without
	// other limits: that is all of MaxMemory. 256 MiB stops a runaway
	// recursion of one goroutine long before it fills a quarter of the
	// quota. The cost: a valid program with deep frames can stop with
	// ErrStackOverflow here and run under the vm default. Each element of a
	// frame-local array is a 24-byte Value, so 3000 frames with a local
	// [4096]byte (12 MB of stack under gc) need about 290 MiB
	// (TestDefaultMaxStackDeep runs it with the vm default).
	MaxStack = 256 << 20
	// MaxGoroutines is the MaxGoroutines of Options.
	MaxGoroutines = 1000
	// LoadTimeout is the LoadTimeout of Options.
	LoadTimeout = 30 * time.Second
)

// Options returns the options of a host that loads plugins it does not
// trust: AOT mode, the limits above, no variable of the host environment
// (DefaultEnv), PanicOnFatal (a plugin that unlocks an unlocked mutex or
// writes a map from two goroutines does not end the host), QuietBudget (a
// request that the budget stops does not log a stack), StubAssembly (zap
// and grpc load as restricted plugins; a call of a function the VM cannot
// run, written in assembly or a system call through the C library, fails at
// run time, and the VM never runs it), RefuseAtCall (a plugin whose
// dependencies name refused functions in code it does not run loads; a call
// of a refused function fails at run time, and the VM never runs it),
// Exec set to vm.ExecMock (a plugin whose dependencies import os/exec loads;
// running a program returns an error, and no process starts), and every
// option that accepts what
// yaegi accepted (Traefik plugins were written for yaegi): unused names,
// unused expression values, the test files and the identical copies of a
// declaration left out, the "// yaegi:tags" lines, a go directive newer than
// the host, the packages of the archive that a nested module does not
// vendor, and a vendor directory that does not match go.mod. The
// restriction level is the default one (restricted).
//
// It also sets StaleAOTFallback: a host built with a newer Go patch release
// than the AOT copies, which changed the sources of a copied package, loads
// the plugins in default mode instead of refusing all of them (a host such
// as Traefik builds its releases with the latest Go patch release, and
// disables every plugin on the first error). The host logs Plugin.Notes,
// which says so. The trade-off: default mode is the mode that Check
// otherwise refuses (the native standard library does not see the methods
// of the plugin types, see Check); it runs only until yaegi v2 is updated for
// the new patch release, and no stale copy runs (the copies may miss its
// security fixes).
//
// The host then sets Dir, Root, UseUnsafe (from its own configuration,
// never from plugin data), Env (SetEnv, with HostEnv or an EnvFilter: the
// variables the plugin needs), Stdout, Stderr and GoPanic. Each call returns
// a new value. Package internal/pluginhost (the catalog host, the tests)
// uses it as is.
func Options() vm.Options {
	return vm.Options{
		AOT:                         true,
		StaleAOTFallback:            true,
		Budget:                      Budget,
		BudgetRate:                  BudgetRate,
		BackgroundRate:              BackgroundRate,
		PanicOnFatal:                true,
		QuietBudget:                 true,
		MaxMemory:                   MaxMemory,
		MaxStack:                    MaxStack,
		MaxGoroutines:               MaxGoroutines,
		LoadTimeout:                 LoadTimeout,
		Env:                         DefaultEnv(),
		IgnoreSoftErrors:            true,
		IgnoreUnusedExprs:           true,
		IgnoreTestFiles:             true,
		IgnoreDuplicateDecls:        true,
		YaegiTags:                   true,
		AllowNewerGo:                true,
		ImportFromArchive:           true,
		IgnoreInconsistentVendoring: true,
		StubAssembly:                true,
		RefuseAtCall:                true,
		Exec:                        vm.ExecMock,
	}
}

// CheckError is the error of Check: the options are not safe for plugins
// the host does not trust. Problems has one *OptionError per refused
// option; errors.As finds them (Unwrap). It is not a *vm.LoadError: the
// configuration of the host is wrong, not the plugin, so a host does not
// try another engine on it.
type CheckError struct {
	Problems []*OptionError
}

func (e *CheckError) Error() string {
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		msgs[i] = p.Error()
	}
	return "host: options not safe for untrusted plugins: " + strings.Join(msgs, "\n")
}

// Unwrap returns the problems, for errors.Is and errors.As.
func (e *CheckError) Unwrap() []error {
	out := make([]error, len(e.Problems))
	for i, p := range e.Problems {
		out[i] = p
	}
	return out
}

// OptionError is one option that Check refuses.
type OptionError struct {
	// Option is the name of the field of vm.Options: "Budget",
	// "Sandbox.Budget", "LoadLimits.MaxFiles", `Modes["fmt"]`...
	Option string
	// Reason says why, with the name of the option.
	Reason string
	// Err is the cause, or nil (the error of vm.CheckAOT for "AOT").
	Err error
}

func (e *OptionError) Error() string { return e.Reason }

func (e *OptionError) Unwrap() error { return e.Err }

// Check returns a *CheckError when o is not safe for plugins the host does
// not trust, or nil. It refuses:
//   - VM mode (AOT false): native fmt and text/template see a value of the
//     plugin as a native copy without its methods, which is not always
//     checked, so a result can be silently wrong (README, "Execution
//     modes");
//   - AOT mode without the AOT copies;
//   - AOT copies that cannot run in this host (vm.CheckAOT: copies made
//     with an older Go patch release that changed a copied package, or
//     another release), unless StaleAOTFallback is on: the load then runs
//     in default mode, with the native standard library of the host (the
//     trade-off is in Options);
//   - Unrestricted, RealProcess, and Exec set to vm.ExecReal: they give the
//     plugin the host process;
//   - a zero or negative limit (Budget, BackgroundRate, MaxGoroutines,
//     MaxMemory, LoadTimeout), a negative MaxStack, and a zero or negative
//     field of LoadLimits: the vm takes them as no limit (a nil LoadLimits
//     is vm.DefaultLoadLimits, a zero MaxStack is vm.DefaultMaxStack);
//   - a zero or negative BudgetRate: Budget is then a total per call, and
//     a long stream through the plugin dies after Budget steps;
//   - a Sandbox with a zero or negative Budget or MaxGoroutines: the
//     Sandbox replaces Budget and MaxGoroutines of o (its Env replaces Env:
//     nil is an empty environment there, which is safe);
//   - GoList and Src: the GoList loader runs the Go toolchain on the
//     plugin, and has neither LoadLimits nor the checks of the load bombs;
//   - ModeNative in Modes for a package that has an AOT copy (except flag):
//     the native package behaves as in VM mode, above;
//   - AllowStaleAOT: the copies can miss the security fixes of the newer
//     patch releases of Go;
//   - NoPreflight: code the VM cannot run panics in the middle of a request
//     instead of failing the load;
//   - a nil Env (a copy of the whole host environment: its secrets) and
//     PanicOnFatal off.
//
// The frames of a goroutine (vm.Machine.MaxDepth) are not an option: the vm
// always bounds them. Options passes Check. Load and LoadContext call it.
func Check(o vm.Options) error {
	var aotErr error
	if o.AOT {
		aotErr = vm.CheckAOT(o.AllowStaleAOT)
	}
	return check(o, len(vm.AOTGoVersions()) > 0, aotErr)
}

// check is Check; copies tells whether the host has AOT copies, aotErr is
// the error of vm.CheckAOT.
func check(o vm.Options, copies bool, aotErr error) error {
	var errs []*OptionError
	bad := func(option, format string, a ...any) {
		errs = append(errs, &OptionError{Option: option, Reason: fmt.Sprintf(format, a...)})
	}
	switch {
	case !o.AOT:
		bad("AOT", "AOT is off: plugins must run in AOT mode")
	case !copies:
		bad("AOT", "AOT is on, but the host has no AOT copies: import github.com/traefik/yaegi/v2/aot/all")
	case aotErr != nil && !o.StaleAOTFallback:
		errs = append(errs, &OptionError{Option: "AOT", Reason: aotErr.Error() + " (StaleAOTFallback loads the plugins in default mode instead)", Err: aotErr})
	}
	if o.Unrestricted {
		bad("Unrestricted", "Unrestricted is on: the plugin is not restricted")
	}
	if o.RealProcess {
		bad("RealProcess", "RealProcess is on: the plugin can end the host and run programs")
	}
	if o.Exec == vm.ExecReal {
		bad("Exec", "Exec is ExecReal: the plugin can run programs")
	}
	type limit struct {
		name string
		off  bool
	}
	limits := []limit{
		{"Budget", o.Budget <= 0},
		{"BackgroundRate", o.BackgroundRate <= 0},
		{"MaxGoroutines", o.MaxGoroutines <= 0},
		{"MaxMemory", o.MaxMemory <= 0},
		{"LoadTimeout", o.LoadTimeout <= 0},
	}
	if l := o.LoadLimits; l != nil {
		limits = append(limits,
			limit{"LoadLimits.MaxTotalSourceBytes", l.MaxTotalSourceBytes <= 0},
			limit{"LoadLimits.MaxFiles", l.MaxFiles <= 0},
			limit{"LoadLimits.MaxPackages", l.MaxPackages <= 0})
	}
	if sb := o.Sandbox; sb != nil {
		limits = append(limits,
			limit{"Sandbox.Budget", sb.Budget <= 0},
			limit{"Sandbox.MaxGoroutines", sb.MaxGoroutines <= 0})
	}
	for _, l := range limits {
		if l.off {
			bad(l.name, "%s is 0 or negative: no limit", l.name)
		}
	}
	if o.BudgetRate <= 0 {
		bad("BudgetRate", "BudgetRate is 0 or negative: Budget is a total per call, so a long response through the plugin (a stream) dies")
	}
	if o.MaxStack < 0 {
		bad("MaxStack", "MaxStack is negative: no bound on the stack of a goroutine")
	}
	if o.GoList {
		bad("GoList", "GoList is on: go list runs the Go toolchain on the plugin, without LoadLimits nor the checks of the load bombs")
	}
	if len(o.Src) > 0 {
		bad("Src", "Src is set: it loads with go list (GoList), without LoadLimits nor the checks of the load bombs")
	}
	if o.AllowStaleAOT {
		bad("AllowStaleAOT", "AllowStaleAOT is on: the AOT copies can miss the security fixes of the Go version of the host")
	}
	if o.NoPreflight {
		bad("NoPreflight", "NoPreflight is on: code the VM cannot run panics in the middle of a request instead of failing the load")
	}
	for _, p := range slices.Sorted(maps.Keys(o.Modes)) {
		// With no explicit modes, ModeFor gives ModeAOT only to a package
		// that has an AOT copy. A package without a copy (a std package
		// such as os, golang.org/x/sys/unix, a native module package of
		// the host) runs natively in AOT mode anyway.
		if o.Modes[p] == vm.ModeNative && vm.ModeFor(nil, true, p) == vm.ModeAOT && !nativeOK[p] {
			bad(fmt.Sprintf("Modes[%q]", p), "Modes[%q] is ModeNative, but %s has an AOT copy: the native package sees a value of the plugin without its methods, as in VM mode", p, p)
		}
	}
	if o.Env == nil {
		bad("Env", "Env is nil: the plugin gets a copy of the whole host environment")
	}
	if !o.PanicOnFatal {
		bad("PanicOnFatal", "PanicOnFatal is off: a plugin that unlocks an unlocked mutex ends the host")
	}
	if len(errs) == 0 {
		return nil
	}
	return &CheckError{Problems: errs}
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
// not safe with a *CheckError. Every other error is a *vm.LoadError (the
// plugin does not load: a host may try another engine).
func LoadContext(ctx context.Context, opts vm.Options, pkgPath string) (*vm.Plugin, error) {
	if err := Check(opts); err != nil {
		return nil, err
	}
	return vm.LoadContext(ctx, opts, pkgPath)
}
