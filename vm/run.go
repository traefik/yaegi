package vm

// Program entry point used by cmd/yaegi and the conformance runner: load a main
// package from source with go/packages, build its SSA form, then run its
// init and main functions in the VM.

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/traefik/yaegi/v2/internal/stdsrc"
)

// RunOptions configures RunMain.
type RunOptions struct {
	// Dir is the directory go/packages runs in (empty: current directory).
	Dir string
	// Budget is the maximum number of calls plus backward jumps (0: unlimited).
	Budget int64
	// Src lists packages interpreted from their sources instead of being called
	// natively. "all" means every package outside the native floor (see IsFloor).
	Src []string
	// OpaqueAny passes interpreted values to native `any` parameters as opaque
	// handles and enables the VM-internal reflect, which interpreting errors,
	// fmt, sort or encoding/json from source requires. Implied by AOT.
	OpaqueAny bool
	// AOT runs the packages compiled ahead of time against the VM value model
	// (aot/..., registered with RegisterAOT) in ModeAOT.
	AOT bool
	// AllowStaleAOT is Options.AllowStaleAOT: AOT mode accepts copies made
	// with an older patch release of the Go release of the host.
	AllowStaleAOT bool
	// StaleAOTFallback is Options.StaleAOTFallback: with copies that cannot
	// run in this host, AOT mode is off for the run, which goes on in
	// default mode. OnStaleAOT, when set, receives the note that says so.
	StaleAOTFallback bool
	OnStaleAOT       func(note string)
	// Tags are build tags, as "go run -tags t1,t2".
	Tags []string
	// YaegiTags adds the build tags of the comment lines "// yaegi:tags t1
	// t2" of the files of pattern (a directive of yaegi that Go ignores), as
	// Options.YaegiTags does for a plugin. The conformance runner sets it:
	// the programs of yaegi use it.
	YaegiTags bool
	// Modes sets the mode of individual import paths (see ModeFor); it
	// overrides AOT and Src.
	Modes map[string]Mode
	// Restricted applies the import restrictions of plugins
	// (restrict.go) to the main package and its non-standard dependencies;
	// UseUnsafe then allows unsafe and syscall (with the read-only syscall
	// subset of plugins, unsafesyscall.go).
	Restricted, UseUnsafe bool
	// Exec is Options.Exec for a program run with Restricted or
	// RestrictedStd (without them, os/exec is the real one).
	Exec ExecMode
	// RestrictedStd gives the program the standard library of plugins
	// (yaegi's restricted stdlib: os.Exit and log.Fatal panic, os/exec and
	// syscall have no symbols) without the import checks of Restricted,
	// which implies it. Without both, the program gets the standard library
	// as in Go (stdsyms.Unrestricted).
	RestrictedStd bool
	// PrintOut is where the print and println builtins write (nil:
	// os.Stderr, like gc). yaegi writes them to its Stdout: the yaegi
	// conformance programs expect them there.
	PrintOut io.Writer
	// BeforeExit, with the standard library as in Go (no Restricted and no
	// RestrictedStd), is called when the program calls os.Exit, just before
	// the process ends, with the exit code and the timings so far: RunMain
	// never returns in that case. log.Fatal and the Fatal methods of the
	// loggers call os.Exit inside the native log package: they end the
	// process without calling BeforeExit.
	BeforeExit func(code int, tm Timings)

	// hooks are set by the tests only (loadhooks.go).
	hooks *loadHooks
}

// LoadError reports that the program could not be loaded or type-checked.
type LoadError struct {
	Msg string
	// Err is the cause, or nil. LoadContext sets it to ErrCanceled when its
	// context is done while the package init runs.
	Err error
}

func (e *LoadError) Error() string { return e.Msg }

func (e *LoadError) Unwrap() error { return e.Err }

// PanicError reports an unrecovered panic (interpreted or native) of the program.
type PanicError struct {
	Value any
	// Trace is the traceback of the interpreted frames at the panic, in
	// the format of gc ("goroutine 1 [running]:" and the frames), or "".
	Trace string
}

// Error is the first line that gc prints for the panic ("panic: x").
func (e *PanicError) Error() string { return panicLine(e.Value) }

// panicLine is the first line that gc prints for an unrecovered panic r:
// "panic: x". The Error method of a *VMPanic already starts with "panic: ".
func panicLine(r any) string {
	if p, ok := r.(*VMPanic); ok {
		return p.Error()
	}
	return fmt.Sprint("panic: ", r)
}

// GoexitError is returned by RunMain when the main goroutine of the program
// calls runtime.Goexit (errors.Is(err, ErrGoexit) is true). The deferred
// calls of main have run, but the other goroutines of the program may still
// run: with gc, the program ends when they end (Wait).
type GoexitError struct{ m *Machine }

func (e *GoexitError) Error() string { return ErrGoexit.Error() }
func (e *GoexitError) Unwrap() error { return ErrGoexit }

// Wait blocks until the goroutines started by the program have ended.
func (e *GoexitError) Wait() { e.m.waitGoroutines() }

// Timings are the durations of the phases of RunMain.
type Timings struct {
	Load time.Duration // go/packages + SSA construction
	Run  time.Duration // init + main in the VM
}

// runMaxStack and runMaxDepth are the stack limits of RunMain programs:
// 256 MB of registers and 2M frames (64 MB) per goroutine, so a runaway
// recursion stops with ErrStackOverflow before it uses more memory than
// the 1 GB stack of gc.
const (
	runMaxStack = 1 << 28
	runMaxDepth = 1 << 21
)

// RunMain loads the main package designated by pattern (a .go file, a
// directory or an import path) and runs it. The returned error is a
// *LoadError, a *CompileError (construct not supported by the VM), a
// *PanicError, a *GoexitError (the main goroutine called runtime.Goexit;
// the other goroutines may still run), or nil. A runtime.Goexit called by
// native code (not by the program) ends the goroutine of the caller.
func RunMain(opts RunOptions, pattern string) (tm Timings, err error) {
	var note string
	opts.AOT, opts.Modes, note, err = aotGate(opts.hooks.hostGoVersion(), opts.AOT, opts.Modes, opts.AllowStaleAOT, opts.StaleAOTFallback)
	if err != nil {
		return tm, err
	}
	if note != "" && opts.OnStaleAOT != nil {
		opts.OnStaleAOT(note)
	}
	m, mainPkg, t1, err := loadMain(opts, pattern, &tm)
	if err != nil {
		return tm, err
	}
	cx := m.mainCtx()
	defer func() {
		tm.Run = time.Since(t1)
		if r := recover(); r != nil {
			if r == errGoexit {
				// runtime.Goexit in the main goroutine. With gc, the
				// program runs until its other goroutines end: the
				// caller decides (see GoexitError).
				err = &GoexitError{m: m}
				return
			}
			if e, ok := r.(error); ok {
				var ce *CompileError
				if errors.As(e, &ce) {
					err = ce
					return
				}
			}
			err = &PanicError{Value: r, Trace: cx.traceback()}
		}
	}()
	m.compileAhead(mainPkg)
	for _, initFn := range m.initOrder(mainPkg) {
		cx.runMain(m.funcOf(initFn))
	}
	cx.runMain(m.funcOf(mainPkg.Func("main")))
	return tm, nil
}

// loadMain loads the main package of RunMain and returns the machine that
// runs it, its SSA package and the end of the load (the start of the run).
// It sets tm.Load.
func loadMain(opts RunOptions, pattern string, tm *Timings) (*Machine, *ssa.Package, time.Time, error) {
	t0 := time.Now()
	allSyntax := len(opts.Src) > 0 || hasMode(opts.Modes, ModeInterp)
	tags := append([]string{}, opts.Tags...)
	if opts.YaegiTags {
		tags = append(tags, yaegiTags(opts.Dir, pattern, nil)...)
	}
	pkgs, err := load(opts.Dir, pattern, tags, allSyntax)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	// Packages outside the standard library have no native registry entry:
	// they are always interpreted, which requires their syntax. go list cannot
	// mix .go files and import paths, so reload everything from source.
	nonStd := nonStdDeps(pkgs)
	if (len(nonStd) > 0 || usesInterpretedStd(pkgs)) && !allSyntax {
		allSyntax = true
		if pkgs, err = load(opts.Dir, pattern, tags, true); err != nil {
			return nil, nil, time.Time{}, err
		}
		nonStd = nonStdDeps(pkgs)
	}
	// A package with assembly takes its pure Go files, as with the default
	// loader of plugins (goasm.go).
	stdSrc := func(path string) bool {
		return InterpretedStd[path] || slices.Contains(opts.Src, path) || slices.Contains(opts.Src, "all") && !IsFloor(path)
	}
	if needsAsmTags(pkgs, func(path string) bool { return nonStd[path] }, stdSrc, tags) {
		tags = append(tags, asmTags...)
		if pkgs, err = load(opts.Dir, pattern, tags, allSyntax); err != nil {
			return nil, nil, time.Time{}, err
		}
		nonStd = nonStdDeps(pkgs)
	}

	var prog *ssa.Program
	var spkgs []*ssa.Package
	if allSyntax {
		prog, spkgs = ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
		prog.Build()
	} else {
		prog, spkgs = ssautil.Packages(pkgs, ssa.InstantiateGenerics)
		spkgs[0].Build()
	}
	mainPkg := spkgs[0]
	if mainPkg.Pkg.Name() != "main" || mainPkg.Func("main") == nil {
		return nil, nil, time.Time{}, &LoadError{Msg: fmt.Sprintf("load: %s is not a main package", mainPkg.Pkg.Path())}
	}
	var fallback, natFallback map[string]string
	info := goListInfo(pkgs)
	opts.Modes, fallback = aotFallbacks(opts.Modes, opts.AOT, mapKeys(nonStd), info)
	realStd := !opts.Restricted && !opts.RestrictedStd
	src := sourcePkgsOf(pkgs, func(path string) bool { return path == mainPkg.Pkg.Path() || nonStd[path] })
	opts.Modes, natFallback = xsysModes(opts.Modes, realStd || opts.Restricted && opts.UseUnsafe, src, info)
	src = dropNative(src, opts.Modes, opts.AOT)
	want := map[string]bool{}
	for _, p := range opts.Src {
		want[p] = true
	}
	var paths []string
	for _, p := range prog.AllPackages() {
		paths = append(paths, p.Pkg.Path())
	}
	for p := range srcFamily(opts.Src, paths) {
		want[p] = true
	}
	fromSrc := func(p string) bool { return InterpretedStd[p] || want[p] || want["all"] && !IsFloor(p) }
	opts.Modes = stdAOTModes(opts.Modes, opts.AOT, src, opts.hooks.hostGoVersion(), opts.AllowStaleAOT, fromSrc, !realStd) // aot.go
	interp := map[*types.Package]bool{mainPkg.Pkg: true}
	for _, p := range prog.AllPackages() {
		path := p.Pkg.Path()
		md := ModeFor(opts.Modes, opts.AOT, path)
		if opts.Modes[path] == ModeInterp || opts.Modes[path] == ModeDefault && (want[path] || want["all"] && !IsFloor(path) || InterpretedStd[path]) || nonStd[path] && md == ModeInterp {
			interp[p.Pkg] = true
		}
	}
	switch {
	case opts.Restricted:
		if err := checkSource(src, opts.UseUnsafe, opts.Exec != ExecRefused); err != nil {
			return nil, nil, time.Time{}, &LoadError{Msg: err.Error()}
		}
	case realStd:
		if err := checkRealStd(src, hasStdTable); err != nil {
			return nil, nil, time.Time{}, &LoadError{Msg: err.Error()}
		}
	}
	if err := checkSideEffectStd(src); err != nil {
		return nil, nil, time.Time{}, &LoadError{Msg: err.Error()}
	}
	if err := checkAOTSymbols(opts.Modes, opts.AOT, src); err != nil {
		return nil, nil, time.Time{}, &LoadError{Msg: err.Error()}
	}
	embeds, err := embedsOf(src, DefaultLoadLimits)
	if err != nil {
		return nil, nil, time.Time{}, &LoadError{Msg: err.Error()}
	}
	links := linknamesOf(src)
	fieldTrack := goListFieldTrack(opts.Dir, pkgs)
	dropSyntaxAll(pkgs) // after the checks (shrink.go)
	t1 := time.Now()
	tm.Load = t1.Sub(t0)

	m := NewMachine(prog, interp)
	m.embeds = embeds
	m.linknames = links
	m.FieldTrack = fieldTrack
	m.Budget = opts.Budget
	m.PrintOut = opts.PrintOut
	if !opts.Restricted {
		// A program run like "go run" may recurse much deeper than a plugin:
		// gc grows goroutine stacks up to 1 GB. Restricted runs keep the
		// NewMachine limits of the plugins.
		m.MaxStack, m.MaxDepth = runMaxStack, runMaxDepth
	}
	if opts.Restricted {
		m.SetEnv(os.Environ())
		m.restricted = true
		m.unsafeSyscall = opts.UseUnsafe
	}
	m.realStd = realStd
	m.execMode = opts.Exec
	m.setupExec()
	m.guardHostFiles()
	if realStd && opts.BeforeExit != nil {
		load := tm.Load
		m.addOverride("os.Exit", func(code int) {
			opts.BeforeExit(code, Timings{Load: load, Run: time.Since(t1)})
			os.Exit(code)
		})
	}
	m.OpaqueAny = opts.OpaqueAny || opts.AOT || hasMode(opts.Modes, ModeAOT)
	m.AOT = opts.AOT
	m.Modes = opts.Modes
	m.AOTFallback = fallback
	m.NativeFallback = natFallback
	m.ownReflect = ownReflect(interp, func(p *types.Package) bool { return p == mainPkg.Pkg || nonStd[p.Path()] })
	m.setupReflect()
	m.track = true // the traceback of a panic shows where it happened
	return m, mainPkg, t1, nil
}

// compileAhead compiles the functions of the main package pkg before the
// program runs: its functions, the methods of its types, and the function
// literals inside them, as gc compiles a program before it runs. A
// function is otherwise compiled at its first call, and the compiler
// allocates: a program that counts its allocations around a first call
// (runtime.ReadMemStats, testing.AllocsPerRun) saw the work of the
// compiler, 25 allocations for a function that returns a closure (GOROOT
// test closure.go). Compiling runs no code, and a construct that the VM
// cannot run still fails only when it runs (instrLazy): the program does
// what it did. Plugins (Load) keep the compile at the first call, which
// keeps the memory of the code that never runs; the main package of a
// program is small next to its imports.
func (m *Machine) compileAhead(pkg *ssa.Package) {
	var add func(f *ssa.Function)
	add = func(f *ssa.Function) {
		if f == nil || len(f.Blocks) == 0 || f.TypeParams().Len() > 0 {
			return
		}
		_ = m.compileChecked(m.funcOf(f))
		for _, a := range f.AnonFuncs {
			add(a)
		}
	}
	for _, mem := range pkg.Members {
		switch mem := mem.(type) {
		case *ssa.Function:
			add(mem)
		case *ssa.Type:
			named, ok := mem.Type().(*types.Named)
			if !ok || named.TypeParams().Len() > 0 {
				continue
			}
			for i := 0; i < named.NumMethods(); i++ {
				add(m.prog.FuncValue(named.Method(i)))
			}
		}
	}
}

// yaegiTags returns the build tags of yaegi's "// yaegi:tags t1 t2"
// directives found in the comments of pattern, when it is a .go file or a
// directory (RunOptions.YaegiTags, Options.YaegiTags). open reads a file;
// nil means os.Open.
func yaegiTags(dir, pattern string, open func(string) (io.ReadCloser, error)) []string {
	if !filepath.IsAbs(pattern) && dir != "" {
		pattern = filepath.Join(dir, pattern)
	}
	files := []string{pattern}
	if !strings.HasSuffix(pattern, ".go") {
		files, _ = filepath.Glob(filepath.Join(pattern, "*.go"))
	}
	var tags []string
	for _, name := range files {
		var src any // nil: the parser reads the file
		if open != nil {
			r, err := open(name)
			if err != nil {
				continue
			}
			b, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				continue
			}
			src = b
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments|parser.PackageClauseOnly)
		if err != nil {
			continue
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if t, ok := strings.CutPrefix(c.Text, "// yaegi:tags "); ok {
					tags = append(tags, strings.Fields(t)...)
				}
			}
		}
	}
	return tags
}

// goListYaegiTags returns the build tags of the "// yaegi:tags" lines of
// the plugin package pkgPath, for the go list loader with Options.YaegiTags.
// Only go list knows the directory of the package: a load of the names and
// files gives it.
func goListYaegiTags(ctx context.Context, dir string, env []string, pkgPath string, hooks *loadHooks) ([]string, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedFiles, Dir: dir, Context: ctx, Env: env}
	pkgs, err := hooks.loadPackages(cfg, pkgPath)
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		if p.PkgPath != pkgPath {
			continue
		}
		for _, files := range [][]string{p.GoFiles, p.IgnoredFiles, p.OtherFiles} {
			if len(files) > 0 {
				return yaegiTags("", filepath.Dir(files[0]), nil), nil
			}
		}
	}
	return nil, nil
}

// goListEnv is the environment of go list: the platform of the VM, not the
// one of GOOS and GOARCH in the environment or the default of the
// toolchain. A 386 VM run by an amd64 toolchain (GOARCH=386 go build) got
// the amd64 files, export data and type sizes: int had 64 bits in go/types.
// pluginGoListEnv returns the environment of go list for a plugin
// (Options.GoList) whose module is in dir, made from the host environment
// host. go list must not download anything or run another toolchain for a
// plugin: the go.mod of the plugin can ask for a newer Go release (the go
// command then downloads and runs that toolchain when GOTOOLCHAIN allows
// it) or for modules that are not vendored (with GOFLAGS=-mod=mod and a
// GOPROXY, the go command downloads them). So the environment is minimal
// and explicit:
//   - from the host, only what the go command needs to find itself, its
//     caches and the build configuration of the host: PATH, HOME and the
//     other home and temporary directories, GOROOT, GOPATH, GOCACHE,
//     GOMODCACHE, GOTMPDIR, GOENV (the go env file, where "go env -w" keeps
//     these directories: the forced variables below win over it), CGO_ENABLED,
//     GOEXPERIMENT and the GOARCH variants (GOAMD64, GOARM...);
//   - forced: GOOS and GOARCH of the host, GOTOOLCHAIN=local, GOPROXY=off,
//     GOSUMDB=off, and GOFLAGS -mod=vendor when the module has a vendor
//     directory, else -mod=readonly (the module cache only, go.mod never
//     changes; also the default of the go command).
//
// Other variables (GOFLAGS, GOPROXY, GOPRIVATE, GOINSECURE, GOWORK, proxies
// of the network, credentials) are not passed.
func pluginGoListEnv(host []string, dir string) []string {
	keep := map[string]bool{
		"PATH": true, "HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true,
		"XDG_CACHE_HOME": true, "XDG_CONFIG_HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true,
		"SystemRoot": true, "SYSTEMROOT": true, "ComSpec": true, "PATHEXT": true, "home": true, "path": true,
		"GOROOT": true, "GOPATH": true, "GOCACHE": true, "GOMODCACHE": true, "GOTMPDIR": true, "GOENV": true,
		"CGO_ENABLED": true, "GOEXPERIMENT": true,
		"GO386": true, "GOAMD64": true, "GOARM": true, "GOARM64": true, "GOMIPS": true, "GOMIPS64": true,
		"GOPPC64": true, "GORISCV64": true, "GOWASM": true,
	}
	var env []string
	for _, kv := range host {
		if k, _, ok := strings.Cut(kv, "="); ok && keep[k] {
			env = append(env, kv)
		}
	}
	mod := "-mod=readonly"
	if fi, err := os.Stat(filepath.Join(dir, "vendor", "modules.txt")); err == nil && fi.Mode().IsRegular() {
		mod = "-mod=vendor"
	}
	return append(env, "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH,
		"GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOFLAGS="+mod)
}

// goListEnv is the environment of go list for a program (RunMain): the
// environment of the process, as for go run.
func goListEnv() []string {
	return append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
}

func load(dir, pattern string, tags []string, allSyntax bool) ([]*packages.Package, error) {
	cfg := &packages.Config{Mode: packages.LoadSyntax | packages.NeedModule, Dir: dir, Env: goListEnv()}
	cfg.ParseFile = parseFile // the syntax errors of gc (parseerr.go)
	if len(tags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(tags, ",")}
	}
	if allSyntax {
		// Keep NeedModule: nonStdDeps needs p.Module to see the packages
		// of the main module whose path has no dot (module "repro").
		cfg.Mode = packages.LoadAllSyntax | packages.NeedModule
	}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, &LoadError{Msg: "load: " + err.Error()}
	}
	dropTypeErrorsAfterSyntax(pkgs)
	joinSubErrors(pkgs)
	var msgs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			msgs = append(msgs, e.Error())
		}
	})
	if len(msgs) > 0 {
		return nil, &LoadError{Msg: strings.Join(msgs, "\n")}
	}
	if len(pkgs) != 1 {
		return nil, &LoadError{Msg: fmt.Sprintf("load: %d packages match %q, want 1", len(pkgs), pattern)}
	}
	return pkgs, nil
}

// mapKeys returns the keys of m.
func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// nonStdDeps returns the import paths of the dependencies of pkgs that are
// not part of the standard library (module packages, or packages that are
// not in the list of the release, as GOPATH packages).
func nonStdDeps(pkgs []*packages.Package) map[string]bool {
	deps := map[string]bool{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, root := range pkgs {
			if p == root {
				return
			}
		}
		path := p.PkgPath
		if path == "" {
			path = p.ID // import stub (no NeedDeps)
		}
		if p.Module != nil || !isStdPath(path) {
			deps[path] = true
		}
	})
	return deps
}

// ownReflect reports whether one of the interpreted packages for which own
// is true (the program or plugin and its non-standard dependencies) imports
// reflect. The VM reflect must then be used for the whole program: native
// reflect cannot see interpreted methods, and it cannot create named types,
// so a value made by native reflect (a result of Value.Call, a Type.Out)
// would lose the named type of another interpreted package, the one that
// defines it.
func ownReflect(interp map[*types.Package]bool, own func(*types.Package) bool) bool {
	for p, ok := range interp {
		if ok && own(p) && importsReflect(p) {
			return true
		}
	}
	return false
}

// importsReflect reports whether p imports reflect.
func importsReflect(p *types.Package) bool {
	for _, imp := range p.Imports() {
		if imp.Path() == "reflect" {
			return true
		}
	}
	return false
}

// InterpretedStd are the standard library packages the VM always interprets:
// containers that store interpreted values (as a native copy, an element's
// Value field would come back without its interpreted type, and the copy-out
// after the call would replace interface fields with native copies). The
// list is internal/stdsrc.Interpreted, shared with cmd/stdexport.
var InterpretedStd = stdsrc.Set(stdsrc.Interpreted)

// srcFamily returns the packages of paths that are interpreted with the
// standard library packages of src (Options.Src, RunOptions.Src): when a
// package of src is in a tree that has its own internal package (R and
// R/..., with R/internal...), every package of that tree in paths. The
// native registry has no internal package (Go code outside R cannot import
// it), and the packages of the tree share its state: with Go 1.27,
// encoding/json is built on encoding/json/v2; it passes
// &internal.AllowInternalUse to jsontext, which checks the address, and
// it sets the error hooks of encoding/json/internal that v2 calls. So
// interpreting encoding/json alone failed ("native global
// encoding/json/internal.AllowInternalUse not in registry"), and a native
// v2 would not see the hooks of the interpreted package. The packages that
// always stay native (IsFloor: internal/..., crypto/internal...) are not
// counted.
func srcFamily(src, paths []string) map[string]bool {
	out := map[string]bool{}
	for _, q := range paths {
		root, ok := internalRoot(q)
		if !ok || !isStdPath(q) || IsFloor(q) {
			continue
		}
		for _, w := range src {
			if w != root && !strings.HasPrefix(w, root+"/") {
				continue
			}
			for _, x := range paths {
				if (x == root || strings.HasPrefix(x, root+"/")) && !IsFloor(x) {
					out[x] = true
				}
			}
			break
		}
	}
	return out
}

// loadedPaths returns the paths of pkgs and of their dependencies.
func loadedPaths(pkgs []*packages.Package) []string {
	var out []string
	packages.Visit(pkgs, nil, func(p *packages.Package) { out = append(out, p.PkgPath) })
	return out
}

// internalRoot returns R for an internal package R/internal or
// R/internal/...; ok is false for the others and for the internal packages
// at the top of the standard library (internal/...).
func internalRoot(p string) (root string, ok bool) {
	if i := strings.Index(p, "/internal/"); i > 0 {
		return p[:i], true
	}
	if r, found := strings.CutSuffix(p, "/internal"); found && r != "" {
		return r, true
	}
	return "", false
}

// genericStd are the standard library packages whose generic functions
// programs call: their instances have no native counterpart, so the VM
// interprets them, which needs the package syntax (the toolchain-free
// plugin loader embeds these sources, see stdtypes). The list is
// internal/stdsrc.Generic, shared with cmd/stdexport.
var genericStd = stdsrc.Set(stdsrc.Generic)

// usesInterpretedStd reports whether pkgs depend on an InterpretedStd package
// or call generic functions of genericStd: either needs a full-syntax load.
func usesInterpretedStd(pkgs []*packages.Package) bool {
	found := false
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if InterpretedStd[p.PkgPath] {
			found = true
		}
		for path := range p.Imports {
			if genericStd[path] && !genericStd[p.PkgPath] && !isStdPath(p.PkgPath) {
				found = true // a program package imports a generic std package
			}
		}
	})
	return found
}

// IsFloor reports whether package path p always stays native: the packages
// that implement the runtime, reflection, synchronization and system
// interfaces.
func IsFloor(p string) bool {
	for _, f := range []string{"runtime", "internal/", "reflect", "sync", "syscall", "unsafe", "os", "time", "vendor/", "crypto/internal", "iter", "unique", "weak", "math"} {
		if p == f || strings.HasPrefix(p, f) {
			return true
		}
	}
	return false
}

func hasMode(modes map[string]Mode, md Mode) bool {
	for _, x := range modes {
		if x == md {
			return true
		}
	}
	return false
}
