package vm

// The restrictions of plugins: those of yaegi's restricted stdlib, and a
// few more. They apply to the
// packages run from source (the plugin, its other packages and its vendored
// modules) when the plugin loads: plugins (Load) unless
// Options.Unrestricted, RunMain only with RunOptions.Restricted.
//
//   - Some imports are refused: plugin always, os/exec without
//     Options.Exec (osexec.go), unsafe and
//     syscall without Options.UseUnsafe, net/http/cgi, go/build
//     and go/importer (they run programs), net/http/pprof (it serves the
//     state of the host), go/types (the type checker of the
//     loader, shared with the plugin) and the internal packages of the
//     standard library (go list refuses them, go/types does not).
//   - Some functions of allowed packages are refused because they reach
//     outside the plugin: starting or finding processes, changing the
//     working directory, changing process-wide runtime settings. The
//     check is static (the functions that the code names); the methods
//     among them are also refused at run time, when the plugin reaches
//     them through reflect, an interface or a template (refusedmeth.go).
//   - The variables of the packages not run from source (the standard
//     library) are the host's: a plugin may not assign them, take their
//     address or write through them by name (checkWrites), except the ones
//     a restricted machine has its own copy of (ownVars). This check only
//     sees the names in the code; the run-time check of hostmem.go refuses
//     the writes that go through a copy of a pointer, reflect or a pointer
//     that a native function returns. Its limits are listed there.
//   - Without Options.UseUnsafe, no expression may have type unsafe.Pointer and the
//     reflect functions that turn addresses into pointers are refused:
//     reflect.Value.UnsafePointer and reflect.NewAt give the same power as
//     importing unsafe (writing the unexported fields of host objects).
//
// The runtime restrictions come with the native registry, which is yaegi's
// restricted symbol table (stdSym): os.Exit, log.Fatal and the Fatal methods
// of the loggers panic instead of exiting, and the environment of a
// restricted plugin is its own copy (env.go). TestRestrictions checks them,
// so that a registry built differently cannot lose them. A machine with
// the standard library as in Go (RunMain without Restricted and
// RestrictedStd, Load with Unrestricted and RealProcess) gets the real
// functions and os/exec and syscall instead (stdTable,
// stdsyms.Unrestricted). Such a program is refused at load time when it
// imports syscall on a platform without a syscall table (checkRealStd).

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	pathpkg "path"
	"runtime"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/traefik/yaegi/v2/stdsyms"
	"github.com/traefik/yaegi/v2/stdtypes"
)

// refusedImports are never available to a plugin. yaegi's restricted stdlib
// has no symbols for os/exec and plugin; net/http/cgi runs any program
// (cgi.Handler), which defeats the os/exec ban. The others act on the host
// process, and no plugin of the catalog (docs/CATALOG.md) imports them:
//
//   - go/build runs the go command (Context.Import calls "go list" in
//     module mode), another way around the os/exec ban, and go/importer
//     runs it too (the gc importer finds export data with "go list").
//   - net/http/pprof registers the profiles of the whole host on
//     http.DefaultServeMux when it is imported, and serves its command
//     line, its goroutine stacks and its memory profile. Its table is not
//     even in the default registry (stdsyms/sideeffect).
//
// expvar is allowed: a restricted plugin gets a table of its own that
// starts empty (hostregs.go), without the command line and the memory
// statistics of the host, and its /debug/vars route is on its own
// http.DefaultServeMux. Two plugins can publish the same name.
// prometheus/client_golang imports it.
//   - go/types is the type checker that the VM itself uses to load
//     plugins. Its package variables (types.Universe, types.Unsafe,
//     types.Typ) and the objects they reach are shared with the loader:
//     types.Universe.Insert(obj) declares a new name for every plugin
//     loaded later, types.Unsafe.SetName renames the unsafe package, and
//     (*types.Named).SetUnderlying on the universe's error type changes
//     it. These are method calls on host objects, which the run-time check
//     of hostmem.go does not see (native code writes), and Scope.Insert
//     also races with a plugin that loads at the same time. The objects
//     come from unexported fields too (Universe.Lookup("error")), so a
//     check of the receiver is not enough: the import is refused.
var refusedImports = map[string]bool{
	"os/exec": true, "plugin": true, "net/http/cgi": true,
	"go/build": true, "go/importer": true, "net/http/pprof": true,
	"go/types": true,
}

// unsafeImports need the Options.UseUnsafe opt-in.
var unsafeImports = map[string]bool{"unsafe": true, "syscall": true}

// refusedFuncs are the functions and methods of allowed packages a plugin
// may not use, by types.Func.FullName: they act on the host process.
var refusedFuncs = map[string]bool{
	"os.StartProcess":  true, // runs any program, like os/exec
	"os.FindProcess":   true, // signals or kills other processes
	"os.Chdir":         true, // working directory of the whole process
	"(*os.File).Chdir": true,

	// runtime.GOMAXPROCS stays: libraries call GOMAXPROCS(0) to size
	// their worker pools (klauspost/compress). With n > 0 it does not
	// change the setting for a restricted machine (hostregs.go).
	"runtime.Breakpoint": true,

	"runtime/debug.SetGCPercent":    true,
	"runtime/debug.SetMemoryLimit":  true,
	"runtime/debug.SetMaxStack":     true,
	"runtime/debug.SetMaxThreads":   true,
	"runtime/debug.SetPanicOnFault": true,
	"runtime/debug.SetTraceback":    true,
	"runtime/debug.SetCrashOutput":  true,
	"runtime/debug.WriteHeapDump":   true,

	// Signal handling of the whole process: Ignore and Reset also undo
	// the signal.Notify of the host (SIGINT and SIGTERM no longer stop
	// it gracefully). Notify, NotifyContext and Stop act on the plugin's
	// own channels and stay (plugins of the catalog use NotifyContext).
	"os/signal.Ignore": true,
	"os/signal.Reset":  true,

	// One CPU profile and one execution trace at a time for the whole
	// process: a plugin that starts one takes it from the host, and it
	// records the host too. The profiles that are a snapshot
	// (WriteHeapProfile, Lookup) stay: a plugin of the catalog
	// (traefikoidc) uses them to watch its memory.
	"runtime/pprof.StartCPUProfile": true,
	"runtime/trace.Start":           true,

	// The test driver of package testing: it parses the host's
	// flag.CommandLine with the host's os.Args, ends the process with
	// os.Exit (native code: the restricted os.Exit of the plugin is not
	// used), or replaces os.Stdout (RunExamples). Init defines the test
	// flags in the host's flag.CommandLine.
	"testing.Init":          true,
	"testing.Main":          true,
	"testing.MainStart":     true,
	"testing.RunTests":      true,
	"testing.RunBenchmarks": true,
	"testing.RunExamples":   true,
}

// unsafeFuncs need Options.UseUnsafe: they make pointers from addresses.
var unsafeFuncs = map[string]bool{
	"reflect.NewAt":                 true,
	"(reflect.Value).UnsafePointer": true,
	"(reflect.Value).SetPointer":    true,
}

// ImportError reports an import a restricted plugin may not use.
type ImportError struct {
	Pkg, Path   string
	NeedsUnsafe bool // allowed with Options.UseUnsafe
}

func (e *ImportError) Error() string {
	if e.NeedsUnsafe {
		return fmt.Sprintf("%s: import %q is restricted: the plugin needs Options.UseUnsafe", e.Pkg, e.Path)
	}
	if e.Path == "os/exec" {
		return fmt.Sprintf("%s: import %q is not allowed in a plugin (Options.Exec allows it: vm.ExecMock, or vm.ExecReal for trusted code)", e.Pkg, e.Path)
	}
	return fmt.Sprintf("%s: import %q is not allowed in a plugin", e.Pkg, e.Path)
}

// UseError reports a function or an unsafe.Pointer expression a restricted
// plugin may not use.
type UseError struct {
	Pos         string // file:line:col
	What        string
	NeedsUnsafe bool // allowed with Options.UseUnsafe
}

func (e *UseError) Error() string {
	if e.NeedsUnsafe {
		return fmt.Sprintf("%s: %s is restricted: the plugin needs Options.UseUnsafe", e.Pos, e.What)
	}
	return fmt.Sprintf("%s: %s is not allowed in a plugin", e.Pos, e.What)
}

// sourcePkg is a package run from source, with what the checks read.
type sourcePkg struct {
	fset  *token.FileSet
	types *types.Package
	files []*ast.File
	info  *types.Info // Uses and Types
}

// isStdInternal reports whether path is a package of the standard library
// that only the standard library may import.
func isStdInternal(path string) bool {
	if !isStdPath(path) {
		return false
	}
	if strings.HasPrefix(path, "vendor/") {
		return true
	}
	for _, el := range strings.Split(path, "/") {
		if el == "internal" {
			return true
		}
	}
	return false
}

// checkSource applies the restrictions to the packages run from source.
// exec is true when the plugin may use os/exec (Options.Exec): its import
// and os.StartProcess are allowed.
func checkSource(pkgs []sourcePkg, useUnsafe, exec bool) error {
	src := map[string]bool{}
	for _, p := range pkgs {
		src[p.types.Path()] = true
	}
	var errs []error
	for _, p := range pkgs {
		errs = append(errs, checkImports(p.types, useUnsafe, exec)...)
		if p.info != nil {
			errs = append(errs, checkUses(p, useUnsafe, exec)...)
			errs = append(errs, checkWrites(p, src)...)
		}
	}
	return errors.Join(errs...)
}

func checkImports(p *types.Package, useUnsafe, exec bool) []error {
	imps := p.Imports()
	paths := make([]string, 0, len(imps))
	for _, imp := range imps {
		paths = append(paths, imp.Path())
	}
	sort.Strings(paths)
	var errs []error
	for _, path := range paths {
		switch {
		case path == "os/exec" && exec:
		case refusedImports[path] || isStdInternal(path):
			errs = append(errs, &ImportError{Pkg: p.Path(), Path: path})
		case unsafeImports[path] && !useUnsafe:
			errs = append(errs, &ImportError{Pkg: p.Path(), Path: path, NeedsUnsafe: true})
		}
	}
	return errs
}

// realStdOnly are the packages whose symbols only the machines with the
// standard library as in Go get (stdsyms.Unrestricted). os/exec has a table
// on every platform; syscall only on the platforms of stdtypes.Platforms,
// because its API is different on each one.
var realStdOnly = []string{"os/exec", "syscall"}

// hasStdTable reports whether the native registry has a table for the
// package path of realStdOnly on this platform.
func hasStdTable(path string) bool {
	_, ok := stdsyms.Unrestricted[path+"/"+pathpkg.Base(path)]
	return ok
}

// checkRealStd refuses the imports of realStdOnly that have no table on
// this platform (has is hasStdTable, or a fake in tests). Without this check,
// a program that imports syscall on linux/386 or freebsd would load, then
// panic at its first call with "not in registry".
func checkRealStd(pkgs []sourcePkg, has func(path string) bool) error {
	var errs []error
	for _, p := range pkgs {
		for _, imp := range p.types.Imports() {
			if slices.Contains(realStdOnly, imp.Path()) && !has(imp.Path()) {
				errs = append(errs, fmt.Errorf("%s: import %q is not supported on %s/%s: yaegi v2 has its symbols only on %s",
					p.types.Path(), imp.Path(), runtime.GOOS, runtime.GOARCH, strings.Join(stdtypes.Platforms, ", ")))
			}
		}
	}
	return errors.Join(errs...)
}

// sideEffectStd are the standard library packages whose tables are in
// package stdsyms/sideeffect (their init changes the global state of the
// process, see cmd/stdsyms).
var sideEffectStd = []string{"expvar", "net/http/pprof", "testing/quick"}

// checkSideEffectStd refuses the imports of sideEffectStd when the host
// did not import stdsyms/sideeffect: without their table, the program
// would load, then panic at its first call with "not in registry".
func checkSideEffectStd(pkgs []sourcePkg) error {
	var errs []error
	for _, p := range pkgs {
		for _, imp := range p.types.Imports() {
			if slices.Contains(sideEffectStd, imp.Path()) && stdsyms.Symbols[imp.Path()+"/"+imp.Name()] == nil {
				errs = append(errs, fmt.Errorf("%s: import %q needs the host to import github.com/traefik/yaegi/v2/stdsyms/sideeffect",
					p.types.Path(), imp.Path()))
			}
		}
	}
	return errors.Join(errs...)
}

// checkUses reports the refused functions a package refers to and, without
// useUnsafe, its expressions of type unsafe.Pointer (an unsafe.Pointer can be
// had without importing unsafe, from reflect.Value.UnsafePointer).
func checkUses(p sourcePkg, useUnsafe, exec bool) []error {
	type found struct {
		pos int
		err error
	}
	var out []found
	fset := p.fset
	report := func(n ast.Node, what string, needsUnsafe bool) {
		out = append(out, found{int(n.Pos()), &UseError{Pos: fset.Position(n.Pos()).String(), What: what, NeedsUnsafe: needsUnsafe}})
	}
	for id, obj := range p.info.Uses {
		f, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		name := f.Origin().FullName()
		switch {
		case name == "os.StartProcess" && exec:
		case refusedFuncs[name]:
			report(id, name, false)
		case unsafeFuncs[name] && !useUnsafe:
			report(id, name, true)
		}
	}
	if !useUnsafe {
		for e, tv := range p.info.Types {
			if tv.IsType() || tv.Type == nil {
				continue
			}
			if b, ok := tv.Type.(*types.Basic); ok && b.Kind() == types.UnsafePointer {
				report(e, "an expression of type unsafe.Pointer", true)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })
	errs := make([]error, 0, len(out))
	for i, f := range out {
		// One report per position (an expression and its operand).
		if i > 0 && f.pos == out[i-1].pos {
			continue
		}
		errs = append(errs, f.err)
	}
	return errs
}

// sourcePkgsOf returns the packages loaded by go list whose path keep
// accepts, with their syntax when loaded.
func sourcePkgsOf(pkgs []*packages.Package, keep func(path string) bool) []sourcePkg {
	var out []sourcePkg
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Types == nil || !keep(p.Types.Path()) {
			return
		}
		out = append(out, sourcePkg{fset: p.Fset, types: p.Types, files: p.Syntax, info: p.TypesInfo})
	})
	return out
}

// ownVars are the variables of native packages that a restricted machine
// has its own copy of (restrictedVar): a plugin may assign them and write
// through them, it changes only its copy.
var ownVars = map[string]bool{
	"os.Args":                   true,
	"os.Stdin":                  true, // stdio.go
	"os.Stdout":                 true,
	"os.Stderr":                 true,
	"flag.CommandLine":          true,
	"flag.Usage":                true,
	"net/http.DefaultClient":    true,
	"net/http.DefaultTransport": true,
	"net/http.DefaultServeMux":  true,
	"net/rpc.DefaultServer":     true, // hostregs.go
	"net.DefaultResolver":       true,
}

// sharedObjVars are the variables of ownVars whose copy pointed to an
// object of the host: with Options.Stdout and Options.Stderr nil (the
// default, also with Sandbox), the plugin's os.Stdout was a copy of the
// host's *os.File pointer. It is its own duplicate of the host file now
// (stdio.go, hostfd.go); the checks stay: the plugin may assign the
// variable, but not write through it (*os.Stdout = x): checkWrites refuses
// it, and the walk of hostmem.go records the host's *os.File objects, so a
// write through a copy of the host pointer is refused at run time.
var sharedObjVars = map[string]bool{
	"os.Stdin":  true,
	"os.Stdout": true,
	"os.Stderr": true,
}

// checkWrites reports the writes of a package to the variables of the
// packages that are not run from source (src): an assignment (also op=,
// increments, decrements and the variables of a range clause), the address of the
// variable (&v, or a method with a pointer receiver called on it), and the
// builtins delete, clear and copy on it. A write through the variable
// counts too: v.f = x, *v = x, v[i] = x write the objects of the host that
// v points to (time.Local, unicode.Categories).
//
// This check sees only the writes that name the variable. It does not see
// a write through a copy of the pointer (l := time.Local; *l = x; m :=
// unicode.Categories; delete(m, k)), through a pointer that a native
// function returns (*time.Now().Location() = x), or through reflect
// (reflect.ValueOf(io.EOF).Elem().Set(...)). The run-time check of
// hostmem.go refuses these. Writes made by native code are a known limit
// (hostmem.go).
func checkWrites(p sourcePkg, src map[string]bool) []error {
	var errs []error
	report := func(e ast.Expr, how string) {
		v := hostVarOf(p.info, e, src)
		if v == nil {
			return
		}
		what := fmt.Sprintf("%s the host variable %s.%s", how, v.Pkg().Path(), v.Name())
		errs = append(errs, &UseError{Pos: p.fset.Position(e.Pos()).String(), What: what})
	}
	for _, f := range p.files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if n.Tok != token.DEFINE {
					for _, e := range n.Lhs {
						report(e, "assigning")
					}
				}
			case *ast.IncDecStmt:
				report(n.X, "assigning")
			case *ast.RangeStmt:
				if n.Tok == token.ASSIGN {
					for _, e := range []ast.Expr{n.Key, n.Value} {
						if e != nil {
							report(e, "assigning")
						}
					}
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND {
					report(n.X, "taking the address of")
				}
			case *ast.SliceExpr:
				// Slicing an array variable shares its memory.
				if _, ok := typeUnder(p.info.TypeOf(n.X)).(*types.Array); ok {
					report(n.X, "slicing")
				}
			case *ast.SelectorExpr:
				// A method with a pointer receiver on an addressable value
				// takes its address.
				sel := p.info.Selections[n]
				if sel == nil || sel.Kind() == types.FieldVal || sel.Indirect() {
					break
				}
				if _, isPtr := typeUnder(p.info.TypeOf(n.X)).(*types.Pointer); isPtr {
					break
				}
				if sig, ok := sel.Obj().Type().(*types.Signature); ok && sig.Recv() != nil {
					if _, ptrRecv := typeUnder(sig.Recv().Type()).(*types.Pointer); ptrRecv {
						report(n.X, "calling a pointer method on")
					}
				}
			case *ast.CallExpr:
				id, ok := ast.Unparen(n.Fun).(*ast.Ident)
				if !ok || len(n.Args) == 0 {
					break
				}
				if b, ok := p.info.Uses[id].(*types.Builtin); ok {
					switch b.Name() {
					case "delete", "clear", "copy":
						report(n.Args[0], "changing")
					}
				}
			}
			return true
		})
	}
	return errs
}

// typeUnder is t.Underlying(), or nil for a nil t.
func typeUnder(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	return t.Underlying()
}

// hostVarOf returns the package variable of a package not in src that the
// expression e writes when it is written: e is that variable, or a field,
// an element, a slice, a type assertion or the pointed-to value of it, at
// any depth. It
// returns nil for the variables of ownVars and when e does not start with
// such a variable (a call, a local variable). A write through a variable of
// sharedObjVars (*os.Stdout = x) is not a write of the plugin's copy: it
// returns the variable.
func hostVarOf(info *types.Info, e ast.Expr, src map[string]bool) *types.Var {
	through := false // e writes through the variable, not the variable itself
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.StarExpr:
			e, through = x.X, true
		case *ast.IndexExpr:
			e, through = x.X, true
		case *ast.SliceExpr:
			e, through = x.X, true
		case *ast.TypeAssertExpr:
			e, through = x.X, true
		case *ast.Ident:
			return pkgVar(info.Uses[x], src, through)
		case *ast.SelectorExpr:
			if _, isSel := info.Selections[x]; isSel {
				e, through = x.X, true // a field or a method value
				continue
			}
			return pkgVar(info.Uses[x.Sel], src, through) // a qualified identifier
		default:
			return nil
		}
	}
}

// pkgVar returns obj when it is a package variable of a package not in src
// and not in ownVars. through says that the write goes through the
// variable: then a variable of sharedObjVars is returned too.
func pkgVar(obj types.Object, src map[string]bool, through bool) *types.Var {
	v, ok := obj.(*types.Var)
	if !ok || v.Pkg() == nil || v.Parent() != v.Pkg().Scope() || src[v.Pkg().Path()] {
		return nil
	}
	if name := v.Pkg().Path() + "." + v.Name(); ownVars[name] && !(through && sharedObjVars[name]) {
		return nil
	}
	return v
}
