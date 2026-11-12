package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	facadePath = "github.com/traefik/yaegi/v2/vreflect"
	vmPath     = "github.com/traefik/yaegi/v2/vm"
)

type pkgKind int

const (
	kNative  pkgKind = iota // kept native: not copied
	kConvert                // copied with reflect rewritten against the twin, plus glue
	kCopy                   // copied as is (imports remapped): an internal package a copy needs
)

func (k pkgKind) String() string {
	return [...]string{"native", "aot", "copy"}[k]
}

// pinfo is the plan and the report of one package.
type pinfo struct {
	p       *packages.Package
	kind    pkgKind
	why     string
	target  bool
	aotPath string // import path of the copy
	dir     string // directory of the copy

	used     map[string]int // reflect API members used
	missing  []string       // used members absent from the twin
	partial  []string       // used members with a documented limitation in the twin
	blockers []string       // what prevents the copy from building or running correctly
	notes    []string

	asserts, switches, eqs, tagSwitches, linknames int
	skippedAsserts, unboxes, vetFixes              int
	unsafeUses, goStmts                            int
	typeCaches                                     int // sync.Map variables made vm.TypeCache
	typeMaps                                       int // map variables made vm.TypeMap
	nestGuards                                     int // recursions counted by vm.NestAOT
	ptrKeyFuncs, ptrKeys                           int // json ptrEncoder.encode, and its keys made vm.PtrKey
	exits                                          int // os.Exit, log.Fatal and os.Args made vm.AOTExit, AOTFatal, AOTArgs
	httpDefaults                                   int // http.Handle, http.DefaultServeMux... made vm.AOTHandle, AOTDefaultServeMux...
	jsonPtrChecks, jsonNoUnboxes, jsonAddrCopies   int // json v2 cycle check (jsonV2Cycle)
	jsonTopCopies                                  int // json v2 top-level copy made vm.AddrCopy (jsonV2Cycle)
	typeMapTargets                                 map[*types.Var]*ast.MapType
	exists                                         constraint.Expr // builds that have the package, nil for all (constraints.go)

	wrapIfaces []types.Type // interfaces needing a generated wrapper
	tsEntries  []*tsEntry
	hooks      int
	noHook     []string
	wrappers   int

	built    bool
	buildErr string
	license  string
}

type tsEntry struct {
	name   string
	static types.Type
	cases  []types.Type
}

type gen struct {
	root, module    string
	outDir, outPath string
	natDir, natPath string
	keep            map[string]bool
	targets         []string
	pkgs            map[string]*pinfo
	fset            *token.FileSet
	facade          *types.Package
	tsSeq, wrapSeq  int
}

func (g *gen) load(targets []string) error {
	g.targets = targets
	g.fset = token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedModule | packages.NeedEmbedFiles,
		Fset: g.fset,
		Dir:  g.root,
	}
	roots, err := packages.Load(cfg, targets...)
	if err != nil {
		return err
	}
	var errs []string
	g.pkgs = map[string]*pinfo{}
	packages.Visit(roots, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
		g.pkgs[p.PkgPath] = &pinfo{p: p, used: map[string]int{}}
	})
	if len(errs) > 0 {
		return fmt.Errorf("load errors:\n%s", strings.Join(errs, "\n"))
	}
	for _, r := range roots {
		g.pkgs[r.PkgPath].target = true
	}
	fp, err := packages.Load(&packages.Config{Mode: packages.NeedTypes | packages.NeedName, Dir: g.root}, facadePath)
	if err != nil || len(fp) != 1 || fp[0].Types == nil {
		return fmt.Errorf("loading %s: %v", facadePath, err)
	}
	// An error here (for example export data that golang.org/x/tools cannot
	// read) leaves the twin with no members: every reflect member would be
	// reported missing, and the report would be wrong. Stop instead.
	if len(fp[0].Errors) > 0 {
		return fmt.Errorf("loading %s: %v", facadePath, fp[0].Errors)
	}
	g.facade = fp[0].Types
	return nil
}

func isReflectPath(p string) bool { return p == "reflect" || p == "internal/reflectlite" }

func isStd(p *packages.Package) bool {
	first, _, _ := strings.Cut(p.PkgPath, "/")
	return p.Module == nil && !strings.Contains(first, ".")
}

// floor: packages that stay native even if they import reflect (runtime,
// system, network, crypto...: they do not inspect user types, or cannot be
// copied out of the standard library).
func floor(p string) bool {
	if p == "internal/fmtsort" {
		return false
	}
	for _, f := range []string{"runtime", "internal/", "reflect", "sync", "syscall", "unsafe", "os", "time", "vendor/",
		"crypto", "iter", "unique", "weak", "math", "context", "net/http", "net/netip", "go/", "testing", "archive/",
		"io", "log/internal", "cmd/"} {
		if p == strings.TrimSuffix(f, "/") || strings.HasPrefix(p, f) {
			return true
		}
	}
	return false
}

func importsReflect(p *packages.Package) bool {
	for imp := range p.Imports {
		if isReflectPath(imp) {
			return true
		}
	}
	return false
}

// mapDir maps an import path to its location under the output root.
func mapDir(p string) string {
	if strings.HasPrefix(p, "vendor/") {
		return "vendored/" + strings.TrimPrefix(p, "vendor/")
	}
	return p
}

// internalParent returns the path prefix allowed to import p ("" for std
// internal packages), and whether p is internal.
func internalParent(p string) (string, bool) {
	switch {
	case p == "internal" || strings.HasPrefix(p, "internal/"):
		return "", true
	case strings.HasSuffix(p, "/internal"):
		return strings.TrimSuffix(p, "/internal"), true
	}
	if i := strings.Index(p, "/internal/"); i >= 0 {
		return p[:i], true
	}
	return "", false
}

// importable reports whether importer may import imp.
func importable(imp, importer string) bool {
	if strings.HasPrefix(imp, "vendor/") {
		return false
	}
	parent, ok := internalParent(imp)
	if !ok {
		return true
	}
	if parent == "" {
		first, _, _ := strings.Cut(importer, "/")
		return !strings.Contains(first, ".")
	}
	return importer == parent || strings.HasPrefix(importer, parent+"/")
}

func (g *gen) plan() {
	for p, pi := range g.pkgs {
		pi.aotPath = g.outPath + "/" + mapDir(p)
		pi.dir = filepath.Join(g.outDir, filepath.FromSlash(mapDir(p)))
		switch {
		case isReflectPath(p) || p == "unsafe":
			pi.kind, pi.why = kNative, "reflect itself (replaced by the twin)"
		case pi.target:
			pi.kind, pi.why = kConvert, "target"
		case g.keep[p]:
			pi.kind, pi.why = kNative, "-keep"
		case floor(p):
			pi.kind, pi.why = kNative, "native floor"
		case importsReflect(pi.p):
			pi.kind, pi.why = kConvert, "imports reflect"
		default:
			pi.kind, pi.why = kNative, "does not use reflect"
		}
	}
	// A converted package passing reflect values to a native one needs it
	// converted too (its API would take the real reflect.Value).
	for changed := true; changed; {
		changed = false
		for _, pi := range g.sorted() {
			if pi.kind != kConvert {
				continue
			}
			for _, obj := range pi.p.TypesInfo.Uses {
				q := obj.Pkg()
				if q == nil || q == pi.p.Types || isReflectPath(q.Path()) {
					continue
				}
				qi := g.pkgs[q.Path()]
				if qi == nil || qi.kind == kConvert {
					continue
				}
				if mentionsReflect(obj.Type()) {
					if floor(q.Path()) && !strings.HasPrefix(q.Path(), "internal/") {
						pi.blockers = append(pi.blockers, fmt.Sprintf("passes reflect values to native package %s (%s)", q.Path(), obj.Name()))
						continue
					}
					qi.kind, qi.why, changed = kConvert, "receives reflect values from "+pi.p.PkgPath, true
				}
			}
		}
	}
	// Internal (and vendored) packages that copies import must be copied.
	// A copy is also needed for a native package whose API, as a copy uses
	// it, has types of a copied internal package: the copy has its own
	// version of these types, and the native package has the other one.
	// Since Go 1.27, encoding/json/v2 calls encoding/json/jsontext with
	// types of encoding/json/internal/jsonopts and jsonflags, so jsontext
	// is copied with them.
	for changed := true; changed; {
		changed = false
		for _, pi := range g.sorted() {
			if pi.kind == kNative || shimFiles(pi.p.PkgPath) != nil {
				continue // a shim only imports public packages
			}
			for imp := range pi.p.Imports {
				qi := g.pkgs[imp]
				if qi == nil || qi.kind != kNative || isReflectPath(imp) || imp == "unsafe" || imp == "C" {
					continue
				}
				if !importable(imp, pi.aotPath) {
					qi.kind, qi.why, changed = kCopy, "internal package imported by "+pi.p.PkgPath, true
					if importsReflect(qi.p) {
						qi.kind = kConvert
					}
				}
			}
			for _, obj := range pi.p.TypesInfo.Uses {
				q := obj.Pkg()
				if q == nil || q == pi.p.Types || isReflectPath(q.Path()) || q.Path() == "unsafe" {
					continue
				}
				qi := g.pkgs[q.Path()]
				if qi == nil || qi.kind != kNative || !g.mentionsCopiedInternal(obj.Type()) {
					continue
				}
				if floor(q.Path()) && !strings.HasPrefix(q.Path(), "internal/") {
					pi.blockers = append(pi.blockers, fmt.Sprintf("uses native package %s with types of a copied internal package (%s)", q.Path(), obj.Name()))
					continue
				}
				qi.kind, qi.why, changed = kCopy, "shares types of a copied internal package with "+pi.p.PkgPath, true
				if importsReflect(qi.p) {
					qi.kind = kConvert
				}
			}
		}
	}
}

// mentionsCopiedInternal reports whether t mentions a named type of an
// internal package that is copied (an AOT copy cannot import the original).
func (g *gen) mentionsCopiedInternal(t types.Type) bool {
	return mentions(t, func(p *types.Package) bool {
		qi := g.pkgs[p.Path()]
		_, internal := internalParent(p.Path())
		return internal && qi != nil && qi.kind != kNative
	})
}

func mentionsReflect(t types.Type) bool {
	return mentions(t, func(p *types.Package) bool { return isReflectPath(p.Path()) })
}

// mentions reports whether t mentions a named type of a package for which
// pkg is true. It does not look inside named types.
func mentions(t types.Type, pkg func(*types.Package) bool) bool {
	found := false
	var walk func(t types.Type, depth int)
	walk = func(t types.Type, depth int) {
		if found || t == nil || depth > 6 {
			return
		}
		switch t := types.Unalias(t).(type) {
		case *types.Named:
			if t.Obj().Pkg() != nil && pkg(t.Obj().Pkg()) {
				found = true
			}
		case *types.Pointer:
			walk(t.Elem(), depth+1)
		case *types.Slice:
			walk(t.Elem(), depth+1)
		case *types.Array:
			walk(t.Elem(), depth+1)
		case *types.Map:
			walk(t.Key(), depth+1)
			walk(t.Elem(), depth+1)
		case *types.Signature:
			for i := 0; i < t.Params().Len(); i++ {
				walk(t.Params().At(i).Type(), depth+1)
			}
			for i := 0; i < t.Results().Len(); i++ {
				walk(t.Results().At(i).Type(), depth+1)
			}
		}
	}
	walk(t, 0)
	return found
}

func (g *gen) sorted() []*pinfo {
	out := make([]*pinfo, 0, len(g.pkgs))
	for _, pi := range g.pkgs {
		out = append(out, pi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].p.PkgPath < out[j].p.PkgPath })
	return out
}

// importPathFor is the import path generated code uses for package p.
func (g *gen) importPathFor(p string) string {
	switch {
	case isReflectPath(p):
		return facadePath
	}
	if pi := g.pkgs[p]; pi != nil && pi.kind != kNative {
		return pi.aotPath
	}
	return p
}

// funcHasBody reports bodyless function declarations (assembly, linkname).
func bodyless(f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Body == nil {
			out = append(out, fd.Name.Name)
		}
	}
	return out
}

func base(p string) string { return path.Base(p) }
