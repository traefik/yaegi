package vm

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// The VM compiles a function at its first call, and an instruction it
// cannot compile becomes an instruction that panics when it runs
// (instrLazy). So a new SSA instruction (a golang.org/x/tools update) or a
// new form of a known one (a new Go release) only showed up when the code
// that uses it ran. The two tests below show it at once.

// ssaNeverCompiled are the instructions of package ssa that compile.go does
// not handle, and why they never reach it.
var ssaNeverCompiled = map[string]string{
	// Only in the body of a generic function, for a conversion whose
	// operand or result is a type parameter. The VM builds with
	// ssa.InstantiateGenerics and compiles the instances, where every type
	// is known: a conversion there is a Convert, ChangeType...
	"MultiConvert": "only in generic bodies, which the VM never compiles",
}

// TestSSAInstructionsHandled: every instruction type of the
// golang.org/x/tools/go/ssa of go.mod has a case in compiler.instr
// (compile.go), or is in ssaNeverCompiled with the reason.
func TestSSAInstructionsHandled(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: "."}
	pkgs, err := packages.Load(cfg, "golang.org/x/tools/go/ssa")
	if err != nil || len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("load golang.org/x/tools/go/ssa: %v %v", err, pkgs)
	}
	scope := pkgs[0].Types.Scope()
	instr := scope.Lookup("Instruction").Type().Underlying().(*types.Interface)
	var all []string
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() || types.IsInterface(tn.Type()) {
			continue
		}
		if types.Implements(types.NewPointer(tn.Type()), instr) {
			all = append(all, name)
		}
	}
	if len(all) < 30 {
		t.Fatalf("found only %d instruction types in package ssa: %v", len(all), all)
	}
	handled := instrCases(t)
	for name := range ssaNeverCompiled {
		if handled[name] {
			t.Errorf("ssaNeverCompiled has %s, which compiler.instr handles: remove it", name)
		}
	}
	for _, name := range all {
		if !handled[name] && ssaNeverCompiled[name] == "" {
			t.Errorf("ssa.%s is an instruction that compiler.instr (compile.go) does not handle: compile it, or add it to ssaNeverCompiled with the reason", name)
		}
	}
}

// instrCases returns the names of the ssa types of the cases of the type
// switch of compiler.instr.
func instrCases(t *testing.T) map[string]bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "compile.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "instr" || fd.Recv == nil {
			continue
		}
		for _, st := range fd.Body.List {
			sw, ok := st.(*ast.TypeSwitchStmt)
			if !ok {
				continue
			}
			for _, c := range sw.Body.List {
				for _, e := range c.(*ast.CaseClause).List {
					if s, ok := e.(*ast.StarExpr); ok {
						if sel, ok := s.X.(*ast.SelectorExpr); ok {
							if id, ok := sel.X.(*ast.Ident); ok && id.Name == "ssa" {
								out[sel.Sel.Name] = true
							}
						}
					}
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no type switch found in compiler.instr")
	}
	return out
}

// TestCompileCorpus compiles ahead of time every interpreted function of
// every program of testdata/ and of the packages they import from outside
// the standard library (as preflight does for a plugin), and fails on any
// instruction that the VM cannot compile. All these programs run in the VM
// (TestTestdata of cmd/runtests): an instruction that does not compile
// there is a new SSA form that the VM must learn, found here even when it
// is in code that the programs do not run.
//
// A symbol missing from the native registry ("not in registry": a function
// without Go body, linked to the runtime) is not counted: it depends on the
// registry, not on the SSA form, and preflight already reports it for the
// code of a plugin.
func TestCompileCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("loads every testdata program")
	}
	files, err := filepath.Glob(filepath.Join("..", "testdata", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no testdata programs: %v", err)
	}
	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			if ok, err := build.Default.MatchFile(filepath.Dir(file), name); err == nil && !ok {
				t.Skip("excluded by its build constraint with this toolchain")
			}
			t.Parallel()
			for _, p := range compileCorpusProgram(t, "./testdata/"+name) {
				if !strings.Contains(p, " not in registry") {
					t.Error(p)
				}
			}
		})
	}
}

// compileCorpusProgram loads the program pattern as RunMain does (default
// mode, the standard library as in Go), compiles every function of the
// program that the VM would interpret, and returns the problems.
func compileCorpusProgram(t *testing.T, pattern string) []string {
	t.Helper()
	var tm Timings
	m, _, _, err := loadMain(RunOptions{Dir: ".."}, pattern, &tm)
	if err != nil {
		if strings.Contains(err.Error(), "needs the host to import github.com/traefik/yaegi/v2/stdsyms/sideeffect") {
			// The vm tests do not import it on purpose (TestLoadPlugin
			// checks this refusal); yaegi v2 does.
			t.Skip(err)
		}
		if typeErrorsOnly("..", pattern) {
			// A program of another platform: testdata/sockopt.go gives an
			// int descriptor to syscall.SetsockoptInt, which takes a
			// syscall.Handle on windows. The conformance runner does not
			// run it there either.
			t.Skipf("the program does not build on %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
		}
		t.Fatal(err)
	}
	prog := m.prog
	var probs []string
	for fn := range ssautil.AllFunctions(prog) {
		if len(fn.Blocks) == 0 || fn.TypeParams().Len() > 0 && len(fn.TypeArgs()) == 0 || !m.isInterp(fn) || !programFunc(m, fn) {
			continue
		}
		f := m.funcOf(fn)
		if err := m.compileChecked(f); err != nil {
			probs = append(probs, fn.String()+": "+err.Error())
		}
		for _, p := range f.problems {
			probs = append(probs, prog.Fset.Position(p.pos).String()+": "+p.msg)
		}
	}
	sort.Strings(probs)
	return probs
}

// typeErrorsOnly reports whether go list refuses the program pattern (in
// dir) with type errors only (and at least one): it does not build on this
// platform. For a file pattern, go list also reports the compiler output
// of the same errors as one list error ("# command-line-arguments\n..."),
// which counts as a type error. Any other error (a missing file, a parse
// error, an import that is not there) is not a platform matter, and the
// load error stays a failure.
func typeErrorsOnly(dir, pattern string) bool {
	cfg := &packages.Config{Mode: packages.LoadSyntax, Dir: dir, Env: goListEnv()}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return false
	}
	n, only := 0, true
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			switch {
			case e.Kind == packages.TypeError:
				n++
			case e.Kind == packages.ListError && strings.HasPrefix(e.Msg, "# "):
			default:
				only = false
			}
		}
	})
	return n > 0 && only
}

// programFunc reports whether fn is code of the program or of its
// non-standard packages: a function of an interpreted package, an instance
// of one, or a wrapper of a method of one of their types. The instances and
// wrappers of standard library code are left out: most of them never run,
// since the VM calls the native code or a hook instead.
func programFunc(m *Machine, fn *ssa.Function) bool {
	if o := fn.Origin(); o != nil {
		fn = o
	}
	if fn.Pkg != nil {
		return m.interp[fn.Pkg.Pkg]
	}
	if recv := fn.Signature.Recv(); recv != nil {
		t := recv.Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil {
			return m.interp[n.Obj().Pkg()]
		}
	}
	if obj := fn.Object(); obj != nil && obj.Pkg() != nil {
		return m.interp[obj.Pkg()]
	}
	return false
}

// TestTypeErrorsOnly: a program with a type error is one of another
// platform for TestCompileCorpus; a program that go list cannot read at
// all, or a program that builds, is not.
func TestTypeErrorsOnly(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"go.mod":      "module example.com/corpus\n\ngo 1.22\n",
		"typeerr.go":  "package main\n\nfunc main() { var n int = \"s\"; _ = n }\n",
		"parseerr.go": "package main\n\nfunc main() {\n",
		"ok.go":       "package main\n\nfunc main() {}\n",
		// a missing import is a list error and a type error
		"badimport.go": "package main\n\nimport _ \"example.com/corpus/missing\"\n\nfunc main() {}\n",
	})
	for name, want := range map[string]bool{"typeerr.go": true, "parseerr.go": false, "ok.go": false, "missing.go": false, "badimport.go": false} {
		if got := typeErrorsOnly(dir, "./"+name); got != want {
			t.Errorf("typeErrorsOnly(%s) = %v, want %v", name, got, want)
		}
	}
}
