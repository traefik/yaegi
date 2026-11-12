// Command stdscan measures how hard it is to interpret the standard library
// from its sources: it loads packages with full syntax, then counts what an
// interpreter cannot execute from source (bodyless/assembly funcs, linkname,
// unsafe, runtime), statically over the import graph and dynamically over the
// functions reachable (RTA) from a sample program.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	tags := flag.String("tags", "", "build tags")
	flag.Parse()
	t0 := time.Now()
	cfg := &packages.Config{Mode: packages.LoadAllSyntax | packages.NeedFiles}
	if *tags != "" {
		cfg.BuildFlags = []string{"-tags=" + *tags}
	}
	pkgs, err := packages.Load(cfg, flag.Args()...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}
	tLoad := time.Since(t0)

	var all []*packages.Package
	packages.Visit(pkgs, nil, func(p *packages.Package) { all = append(all, p) })

	var nGo, nAsm, nFunc, nBodyless, nLinkname int
	var unsafePkgs, runtimeDeps []string
	bodylessBy := map[string]int{}
	for _, p := range all {
		nGo += len(p.GoFiles)
		for _, f := range p.OtherFiles {
			if strings.HasSuffix(f, ".s") {
				nAsm++
			}
		}
		if _, ok := p.Imports["unsafe"]; ok {
			unsafePkgs = append(unsafePkgs, p.PkgPath)
		}
		if strings.HasPrefix(p.PkgPath, "runtime") || strings.HasPrefix(p.PkgPath, "internal/runtime") {
			runtimeDeps = append(runtimeDeps, p.PkgPath)
		}
		for _, f := range p.Syntax {
			for _, cg := range f.Comments {
				for _, c := range cg.List {
					if strings.HasPrefix(c.Text, "//go:linkname") {
						nLinkname++
					}
				}
			}
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok {
					nFunc++
					if fd.Body == nil {
						nBodyless++
						bodylessBy[p.PkgPath]++
					}
				}
			}
		}
	}
	fmt.Printf("roots=%v tags=%q load=%v\n", flag.Args(), *tags, tLoad.Round(time.Millisecond))
	fmt.Printf("STATIC import graph: %d packages, %d .go files, %d .s files, %d func decls, %d bodyless (asm/linkname/runtime-provided), %d //go:linkname lines\n",
		len(all), nGo, nAsm, nFunc, nBodyless, nLinkname)
	fmt.Printf("  packages importing unsafe: %d/%d\n", len(unsafePkgs), len(all))
	fmt.Printf("  runtime packages in graph: %d %v\n", len(runtimeDeps), runtimeDeps)
	printTop("  bodyless funcs by package", bodylessBy, 12)

	t1 := time.Now()
	prog, spkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	tSSA := time.Since(t1)
	var mains []*ssa.Function
	for _, p := range spkgs {
		if p != nil && p.Pkg.Name() == "main" {
			mains = append(mains, p.Func("main"), p.Func("init"))
		}
	}
	allFns := ssautil.AllFunctions(prog)
	fmt.Printf("SSA: %d functions in program (build %v)\n", len(allFns), tSSA.Round(time.Millisecond))
	if len(mains) == 0 {
		return
	}
	res := rta.Analyze(mains, true)
	reach := map[*ssa.Function]bool{}
	for f := range res.Reachable {
		reach[f] = true
	}
	var nReach, nBody0 int
	instrs := map[string]int{}
	unsafeFns, runtimeFns, atomicCalls := 0, 0, 0
	bodyless := map[string]int{}
	bodylessNames := map[string][]string{}
	for f := range reach {
		nReach++
		if f.Pkg != nil && (strings.HasPrefix(f.Pkg.Pkg.Path(), "runtime") || strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/runtime")) {
			runtimeFns++
		}
		if len(f.Blocks) == 0 {
			nBody0++
			pp := "?"
			if f.Pkg != nil {
				pp = f.Pkg.Pkg.Path()
			}
			bodyless[pp]++
			bodylessNames[pp] = append(bodylessNames[pp], f.Name())
			continue
		}
		usesUnsafe := false
		for _, b := range f.Blocks {
			for _, in := range b.Instrs {
				instrs[reflect.TypeOf(in).Elem().Name()]++
				if cv, ok := in.(*ssa.Convert); ok {
					if isUnsafePtr(cv.Type()) || isUnsafePtr(cv.X.Type()) {
						usesUnsafe = true
					}
				}
				if c, ok := in.(ssa.CallInstruction); ok {
					if cal := c.Common().StaticCallee(); cal != nil && cal.Pkg != nil && cal.Pkg.Pkg.Path() == "sync/atomic" {
						atomicCalls++
					}
				}
			}
		}
		if usesUnsafe {
			unsafeFns++
		}
	}
	fmt.Printf("REACHABLE from sample main (RTA): %d functions; %d bodyless; %d with unsafe.Pointer conversions; %d in runtime pkgs; %d static calls into sync/atomic\n",
		nReach, nBody0, unsafeFns, runtimeFns, atomicCalls)
	printTop("  reachable bodyless by package", bodyless, 15)
	for _, p := range []string{"internal/bytealg", "math", "sync/atomic", "internal/runtime/atomic", "runtime", "time", "syscall", "internal/poll", "reflect", "os"} {
		if ns := bodylessNames[p]; len(ns) > 0 {
			sort.Strings(ns)
			if len(ns) > 12 {
				ns = append(ns[:12], "...")
			}
			fmt.Printf("    %s: %v\n", p, ns)
		}
	}
	printTop("  SSA instruction kinds in reachable code", instrs, 40)

	// Floor analysis: a package is "pure" if it neither imports unsafe nor
	// declares bodyless functions; pure packages can be interpreted from source.
	// Impure ones must stay native: count the edges pure -> impure that the
	// interpreter would have to bridge, and how many target internal/ or
	// unexported symbols (not reachable from a native registry in another module).
	impure := map[string]bool{}
	for _, p := range all {
		if _, ok := p.Imports["unsafe"]; ok || bodylessBy[p.PkgPath] > 0 || p.PkgPath == "unsafe" {
			impure[p.PkgPath] = true
		}
	}
	var pureList []string
	for _, p := range all {
		if !impure[p.PkgPath] {
			pureList = append(pureList, p.PkgPath)
		}
	}
	sort.Strings(pureList)
	fmt.Printf("FLOOR: %d pure packages (no unsafe, no bodyless) / %d impure\n  pure: %v\n", len(pureList), len(impure), pureList)
	var impureList []string
	for p := range impure {
		if !strings.HasPrefix(p, "internal/") && !strings.HasPrefix(p, "runtime") && !strings.Contains(p, "/internal/") && !strings.HasPrefix(p, "vendor/") {
			impureList = append(impureList, p)
		}
	}
	sort.Strings(impureList)
	fmt.Printf("  impure public packages: %v\n", impureList)
	pureFns := 0
	cut := map[*ssa.Function]bool{}
	cutInternal, cutUnexported := map[*ssa.Function]bool{}, map[*ssa.Function]bool{}
	for f := range reach {
		if f.Pkg == nil || impure[f.Pkg.Pkg.Path()] {
			continue
		}
		pureFns++
		for _, b := range f.Blocks {
			for _, in := range b.Instrs {
				c, ok := in.(ssa.CallInstruction)
				if !ok {
					continue
				}
				cal := c.Common().StaticCallee()
				if cal == nil || cal.Pkg == nil || !impure[cal.Pkg.Pkg.Path()] {
					continue
				}
				cut[cal] = true
				pp := cal.Pkg.Pkg.Path()
				if strings.HasPrefix(pp, "internal/") || strings.Contains(pp, "/internal/") || strings.HasPrefix(pp, "vendor/") {
					cutInternal[cal] = true
				} else if !ast.IsExported(cal.Name()) {
					cutUnexported[cal] = true
				}
			}
		}
	}
	fmt.Printf("  reachable funcs in pure pkgs: %d; distinct impure callees (native bridge): %d, of which in internal/vendor pkgs: %d, unexported: %d\n",
		pureFns, len(cut), len(cutInternal), len(cutUnexported))
	byPkg := map[string]int{}
	for f := range cutInternal {
		byPkg[f.Pkg.Pkg.Path()]++
	}
	printTop("  internal bridge targets by package", byPkg, 15)

	// Per-package detail: how much of each package actually touches unsafe /
	// has no body (all functions of the package, not only reachable ones).
	type st struct{ total, unsafeFns, bodyless int }
	per := map[string]*st{}
	for f := range allFns {
		if f.Pkg == nil {
			continue
		}
		p := f.Pkg.Pkg.Path()
		s := per[p]
		if s == nil {
			s = &st{}
			per[p] = s
		}
		s.total++
		if len(f.Blocks) == 0 {
			s.bodyless++
			continue
		}
		u := false
		for _, b := range f.Blocks {
			for _, in := range b.Instrs {
				switch in := in.(type) {
				case *ssa.Convert:
					if isUnsafePtr(in.Type()) || isUnsafePtr(in.X.Type()) {
						u = true
					}
				case ssa.CallInstruction:
					if bi, ok := in.Common().Value.(*ssa.Builtin); ok && strings.HasPrefix(bi.Name(), "String") || ok && strings.HasPrefix(bi.Name(), "Slice") || ok && bi.Name() == "Add" {
						u = true
					}
				}
			}
		}
		if u {
			s.unsafeFns++
		}
	}
	fmt.Println("PER PACKAGE (all SSA funcs): pkg total unsafe-using bodyless")
	for _, p := range []string{"errors", "strings", "strconv", "sort", "fmt", "bytes", "bufio", "io", "unicode/utf8", "net/url", "net/textproto", "net/http", "net", "context", "time", "sync", "sync/atomic", "os", "reflect", "math", "slices", "maps", "encoding/json", "internal/bytealg", "internal/reflectlite", "runtime"} {
		if s := per[p]; s != nil {
			fmt.Printf("  %-22s %5d %4d %4d\n", p, s.total, s.unsafeFns, s.bodyless)
		}
	}
}

func isUnsafePtr(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.UnsafePointer
}

func printTop(title string, m map[string]int, n int) {
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	for k, v := range m {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v || kvs[i].v == kvs[j].v && kvs[i].k < kvs[j].k })
	fmt.Printf("%s (%d entries):", title, len(kvs))
	for i, e := range kvs {
		if i == n {
			fmt.Printf(" ...")
			break
		}
		fmt.Printf(" %s=%d", e.k, e.v)
	}
	fmt.Println()
}
