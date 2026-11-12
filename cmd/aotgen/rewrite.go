package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

const vmName = "aotvm" // local name of package vm in rewritten files

// generate writes the copies of every converted or copied package.
func (g *gen) generate() error {
	for _, pi := range g.sorted() {
		if pi.kind == kNative {
			continue
		}
		if err := g.writePackage(pi); err != nil {
			return fmt.Errorf("%s: %w", pi.p.PkgPath, err)
		}
	}
	if err := g.writeAll(); err != nil {
		return err
	}
	return g.writeAllConstrained()
}

func (g *gen) writePackage(pi *pinfo) error {
	if err := os.MkdirAll(pi.dir, 0o755); err != nil {
		return err
	}
	p := pi.p
	// All the assembly files, also the ones of other platforms, so that the
	// report does not depend on the machine that runs aotgen.
	var asm []string
	for _, f := range append(slices.Clone(p.OtherFiles), p.IgnoredFiles...) {
		if strings.HasSuffix(f, ".s") {
			asm = append(asm, filepath.Base(f))
		}
	}
	slices.Sort(asm)
	for _, f := range slices.Compact(asm) {
		pi.notes = append(pi.notes, "assembly file "+f+" not copied")
	}
	for imp := range p.Imports {
		if imp == "C" {
			pi.blockers = append(pi.blockers, "cgo")
		}
	}
	shim := shimFiles(p.PkgPath)
	if shim != nil {
		pi.notes = append(pi.notes, "replaced by a hand-written shim (cmd/aotgen/shims)")
		for name, src := range shim {
			if err := os.WriteFile(filepath.Join(pi.dir, name), src, 0o644); err != nil {
				return err
			}
		}
		return g.writeGlue(pi)
	}
	g.scanUsage(pi)
	if pi.kind == kConvert {
		pi.typeMapTargets = typeMapTargets(p.Syntax, p.TypesInfo, p.Types)
	}
	files := map[string]constraint.Expr{} // file of the copy -> its build constraint
	decls := map[string][]string{}
	for i, f := range p.Syntax {
		name := filepath.Base(p.CompiledGoFiles[i])
		kept, ok, err := keptConstraint(f, genTag)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !ok {
			return fmt.Errorf("%s: selected by go/packages, but its build constraint is false for aotgen (custom tags?)", name)
		}
		files[name], decls[name] = and(nameExpr(name), kept), declared(f)
		if bl := bodyless(f); len(bl) > 0 {
			pi.blockers = append(pi.blockers, fmt.Sprintf("bodyless functions (assembly/linkname) in %s: %s", name, strings.Join(bl, ", ")))
		}
		rw := &rewriter{g: g, pi: pi, f: f, info: p.TypesInfo, kept: kept}
		if err := rw.processExits(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if pi.kind == kConvert {
			rw.typeMaps() // first: it needs the type information of the original nodes
			rw.rewrite()
			rw.typeCaches()
			rw.errorsNest()
			rw.jsonPtrKey()
			rw.jsonV2Cycle()
		}
		rw.fixFile()
		src, err := rw.print(name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(pi.dir, name), src, 0o644); err != nil {
			return err
		}
	}
	if pi.kind == kConvert && p.PkgPath == "errors" && pi.nestGuards != 3 {
		// A new Go version changed is, as or asType: update errorsNest.
		return fmt.Errorf("errors: %d recursion guards added, want 3 (is, as, asType)", pi.nestGuards)
	}
	if pi.kind == kConvert {
		_, usesV2 := p.Imports["encoding/json/v2"]
		v2 := g.pkgs["encoding/json/v2"]
		if err := checkJSONCycle(p.PkgPath, usesV2, v2 != nil && v2.kind == kConvert, pi); err != nil {
			return err
		}
	}
	for _, ef := range p.EmbedFiles {
		rel, err := filepath.Rel(filepath.Dir(p.GoFiles[0]), ef)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(ef)
		if err != nil {
			return err
		}
		dst := filepath.Join(pi.dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	vs, err := g.variants(pi)
	if err != nil {
		return err
	}
	copied, guard, err := g.writeVariants(pi, vs)
	if err != nil {
		return err
	}
	for _, v := range copied {
		files[v.name], decls[v.name] = v.when, declared(v.f)
	}
	if pi.exists, err = packageExists(files, vs); err != nil {
		return err
	}
	if err := g.writeGlue(pi); err != nil {
		return err
	}
	glue, err := os.ReadFile(filepath.Join(pi.dir, "zz_aotglue.go"))
	if err != nil {
		return err
	}
	return checkGlueSymbols(glue, files, decls, guard, pi.exists)
}

// ---- reflect API usage ----

// memberKey names a reflect object as the report does: "ValueOf",
// "Value.Elem", "Type.Implements", "StructField.Tag".
func memberKey(obj types.Object, rpkg *types.Package) string {
	switch o := obj.(type) {
	case *types.Func:
		sig := o.Type().(*types.Signature)
		if r := sig.Recv(); r != nil {
			t := r.Type()
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			if n, ok := types.Unalias(t).(*types.Named); ok {
				return n.Obj().Name() + "." + o.Name()
			}
			// method of an interface literal: find the named interface
			for _, name := range rpkg.Scope().Names() {
				tn, ok := rpkg.Scope().Lookup(name).(*types.TypeName)
				if !ok {
					continue
				}
				if it, ok := tn.Type().Underlying().(*types.Interface); ok {
					for i := 0; i < it.NumMethods(); i++ {
						if it.Method(i) == o {
							return name + "." + o.Name()
						}
					}
				}
			}
			return "?." + o.Name()
		}
	case *types.Var:
		if o.IsField() {
			for _, name := range rpkg.Scope().Names() {
				tn, ok := rpkg.Scope().Lookup(name).(*types.TypeName)
				if !ok {
					continue
				}
				if st, ok := tn.Type().Underlying().(*types.Struct); ok {
					for i := 0; i < st.NumFields(); i++ {
						if st.Field(i) == o {
							return name + "." + o.Name()
						}
					}
				}
			}
			return "?." + o.Name()
		}
	}
	return obj.Name()
}

// hasMember reports whether the facade provides member key.
func (g *gen) hasMember(key string) bool {
	tname, member, ok := strings.Cut(key, ".")
	if !ok {
		return g.facade.Scope().Lookup(key) != nil
	}
	tn, ok := g.facade.Scope().Lookup(tname).(*types.TypeName)
	if !ok {
		return false
	}
	t := tn.Type()
	obj, _, _ := types.LookupFieldOrMethod(t, true, nil, member)
	if obj == nil {
		obj, _, _ = types.LookupFieldOrMethod(t, true, g.facade, member)
	}
	return obj != nil
}

func (g *gen) scanUsage(pi *pinfo) {
	info := pi.p.TypesInfo
	for _, obj := range info.Uses {
		q := obj.Pkg()
		if q == nil {
			continue
		}
		switch {
		case isReflectPath(q.Path()):
			pi.used[q.Name()+":"+memberKey(obj, q)]++
		case q.Path() == "unsafe":
			pi.unsafeUses++
		}
	}
	for _, f := range pi.p.Syntax {
		ast.Inspect(f, func(n ast.Node) bool {
			if _, ok := n.(*ast.GoStmt); ok {
				pi.goStmts++
			}
			return true
		})
	}
	seen := map[string]bool{}
	for k := range pi.used {
		_, key, _ := strings.Cut(k, ":")
		if seen[key] {
			continue
		}
		seen[key] = true
		if !g.hasMember(key) {
			pi.missing = append(pi.missing, key)
		}
		if why, ok := twinLimits[key]; ok {
			pi.partial = append(pi.partial, key+" ("+why+")")
		}
	}
	sort.Strings(pi.missing)
	sort.Strings(pi.partial)
}

// twinLimits documents the members of the twin that do not have reflect's
// full semantics on interpreted values (mirrors the unsupported(...) calls
// of vm/twin_*.go).
var twinLimits = map[string]string{
	"Value.Complex":       "complex numbers are not supported by the VM",
	"Value.SetComplex":    "complex numbers are not supported by the VM",
	"Value.UnsafeAddr":    "address of the VM cell: identity only",
	"Value.UnsafePointer": "VM values: identity only, the memory is not Go memory",
	"Value.Pointer":       "VM values: identity only",
	"Value.SetPointer":    "native values only",
	"Value.InterfaceData": "not supported",
	"Value.Bytes":         "copy (no aliasing) for interpreted []byte held as []Value",
	"NewAt":               "native types only",
	"SliceAt":             "native types only",
	"StructOf":            "native field types only",
	"Type.Size":           "interpreted types: gc sizes (types.SizesFor)",
}

// ---- rewriting ----

type rewriter struct {
	g      *gen
	pi     *pinfo
	f      *ast.File
	info   *types.Info
	needVM bool
	kept   constraint.Expr // platform part of the build constraint (constraints.go)

	commaOk  map[*ast.TypeAssertExpr]bool
	asserts  map[*ast.TypeAssertExpr]types.Type
	switches map[*ast.TypeSwitchStmt]*tsEntry
	eqs      map[*ast.BinaryExpr]bool
	tags     map[*ast.SwitchStmt]bool
	unbox    map[*ast.TypeAssertExpr]bool
}

func isIface(t types.Type) bool {
	if t == nil {
		return false
	}
	if _, ok := t.(*types.TypeParam); ok {
		return false
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// unnamedComposite reports whether t is an unnamed slice, map, array,
// struct, func or chan type: the interpreted values of such types reach AOT
// code as opaque handles (see vm.Unbox).
func unnamedComposite(t types.Type) bool {
	switch t.(type) {
	case *types.Slice, *types.Map, *types.Array, *types.Struct, *types.Signature, *types.Chan:
		return true
	}
	return false
}

func emptyIface(t types.Type) bool {
	return isIface(t) && t.Underlying().(*types.Interface).Empty()
}

func nonEmptyIface(t types.Type) bool {
	if !isIface(t) {
		return false
	}
	return t.Underlying().(*types.Interface).NumMethods() > 0
}

// pos gives a position as the import path of the package, the file name,
// the line and the column ("encoding/asn1/marshal.go:459:2"): unlike an
// absolute path, it does not depend on the GOROOT or the module cache of the
// machine that runs aotgen, so aot/REPORT.txt can be checked by regenerating.
func (rw *rewriter) pos(p token.Pos) string {
	ps := rw.g.fset.Position(p)
	return fmt.Sprintf("%s:%d:%d", path.Join(rw.pi.p.PkgPath, filepath.Base(ps.Filename)), ps.Line, ps.Column)
}

func (rw *rewriter) isNil(e ast.Expr) bool {
	tv, ok := rw.info.Types[e]
	return ok && tv.IsNil()
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// localType reports whether t refers to a type declared inside a function
// (not visible from package level).
func localType(t types.Type, pkg *types.Package) bool {
	found := false
	var walk func(t types.Type)
	walk = func(t types.Type) {
		if found {
			return
		}
		switch t := t.(type) {
		case *types.Named:
			if t.Obj().Pkg() == pkg && t.Obj().Parent() != pkg.Scope() && t.Obj().Parent() != nil {
				found = true
			}
		case *types.Pointer:
			walk(t.Elem())
		case *types.Interface:
			for i := 0; i < t.NumMethods(); i++ {
				walk(t.Method(i).Type())
			}
		case *types.Signature:
			for i := 0; i < t.Params().Len(); i++ {
				walk(t.Params().At(i).Type())
			}
			for i := 0; i < t.Results().Len(); i++ {
				walk(t.Results().At(i).Type())
			}
		case *types.Slice:
			walk(t.Elem())
		case *types.Map:
			walk(t.Key())
			walk(t.Elem())
		}
	}
	walk(t)
	return found
}

func (rw *rewriter) rewrite() {
	rw.commaOk = map[*ast.TypeAssertExpr]bool{}
	rw.asserts = map[*ast.TypeAssertExpr]types.Type{}
	rw.switches = map[*ast.TypeSwitchStmt]*tsEntry{}
	rw.eqs = map[*ast.BinaryExpr]bool{}
	rw.tags = map[*ast.SwitchStmt]bool{}
	rw.unbox = map[*ast.TypeAssertExpr]bool{}
	pkg := rw.pi.p.Types
	gl := newGlue(rw.g, rw.pi) // only to test printability
	// Pass 1: decide with the type information of the original nodes.
	ast.Inspect(rw.f, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			if len(s.Lhs) == 2 && len(s.Rhs) == 1 {
				if ta, ok := unparen(s.Rhs[0]).(*ast.TypeAssertExpr); ok {
					rw.commaOk[ta] = true
				}
			}
		case *ast.ValueSpec:
			if len(s.Names) == 2 && len(s.Values) == 1 {
				if ta, ok := unparen(s.Values[0]).(*ast.TypeAssertExpr); ok {
					rw.commaOk[ta] = true
				}
			}
		case *ast.TypeAssertExpr:
			if s.Type == nil {
				return true
			}
			t := rw.info.TypeOf(s.Type)
			if unnamedComposite(t) && emptyIface(rw.info.TypeOf(s.X)) {
				rw.unbox[s] = true
				return true
			}
			if !nonEmptyIface(t) || !isIface(rw.info.TypeOf(s.X)) {
				return true
			}
			if localType(t, pkg) || !gl.printable(t) {
				rw.pi.skippedAsserts++
				return true
			}
			rw.asserts[s] = t
		case *ast.TypeSwitchStmt:
			var ta *ast.TypeAssertExpr
			switch a := s.Assign.(type) {
			case *ast.ExprStmt:
				ta, _ = unparen(a.X).(*ast.TypeAssertExpr)
			case *ast.AssignStmt:
				ta, _ = unparen(a.Rhs[0]).(*ast.TypeAssertExpr)
			}
			if ta == nil {
				return true
			}
			static := rw.info.TypeOf(ta.X)
			var cases []types.Type
			ok := true
			for _, c := range s.Body.List {
				for _, e := range c.(*ast.CaseClause).List {
					t := rw.info.TypeOf(e)
					if !rw.isNil(e) && unnamedComposite(t) && emptyIface(static) {
						rw.unbox[ta] = true
					}
					if rw.isNil(e) || !nonEmptyIface(t) {
						continue
					}
					if localType(t, pkg) || !gl.printable(t) || !gl.printable(static) {
						ok = false
					}
					cases = append(cases, t)
				}
			}
			if len(cases) == 0 {
				return true
			}
			if !ok {
				rw.pi.skippedAsserts++
				return true
			}
			rw.g.tsSeq++
			rw.switches[s] = &tsEntry{name: fmt.Sprintf("aotTS%d", rw.g.tsSeq), static: static, cases: cases}
		case *ast.BinaryExpr:
			if s.Op != token.EQL && s.Op != token.NEQ {
				return true
			}
			if isIface(rw.info.TypeOf(s.X)) && isIface(rw.info.TypeOf(s.Y)) && !rw.isNil(s.X) && !rw.isNil(s.Y) {
				rw.eqs[s] = true
			}
		case *ast.SwitchStmt:
			if s.Tag == nil || !isIface(rw.info.TypeOf(s.Tag)) {
				return true
			}
			switch unparen(s.Tag).(type) {
			case *ast.Ident, *ast.SelectorExpr:
				rw.tags[s] = true
			default:
				rw.pi.notes = append(rw.pi.notes, "switch on an interface value with a complex tag left native at "+rw.pos(s.Pos()))
			}
		}
		return true
	})
	// Pass 2: rewrite (post-order: inner expressions first).
	astutil.Apply(rw.f, nil, func(c *astutil.Cursor) bool {
		switch n := c.Node().(type) {
		case *ast.TypeAssertExpr:
			if rw.unbox[n] {
				n.X = &ast.CallExpr{Fun: rw.vmSel("Unbox"), Args: []ast.Expr{n.X}}
				rw.pi.unboxes++
				return true
			}
			t, ok := rw.asserts[n]
			if !ok {
				return true
			}
			name := "MustAssertI"
			if rw.commaOk[n] {
				name = "AssertI"
			}
			c.Replace(&ast.CallExpr{
				Fun:  &ast.IndexExpr{X: rw.vmSel(name), Index: n.Type},
				Args: []ast.Expr{n.X},
			})
			rw.pi.asserts++
			rw.pi.wrapIfaces = append(rw.pi.wrapIfaces, t)
		case *ast.TypeSwitchStmt:
			e, ok := rw.switches[n]
			if !ok {
				return true
			}
			var ta *ast.TypeAssertExpr
			switch a := n.Assign.(type) {
			case *ast.ExprStmt:
				ta = unparen(a.X).(*ast.TypeAssertExpr)
			case *ast.AssignStmt:
				ta = unparen(a.Rhs[0]).(*ast.TypeAssertExpr)
			}
			ta.X = &ast.CallExpr{Fun: rw.vmSel("TSG"), Args: []ast.Expr{ta.X, ast.NewIdent(e.name)}}
			rw.pi.switches++
			rw.pi.tsEntries = append(rw.pi.tsEntries, e)
		case *ast.BinaryExpr:
			if !rw.eqs[n] {
				return true
			}
			var r ast.Expr = &ast.CallExpr{Fun: rw.vmSel("EqualAny"), Args: []ast.Expr{n.X, n.Y}}
			if n.Op == token.NEQ {
				r = &ast.UnaryExpr{Op: token.NOT, X: r}
			}
			c.Replace(r)
			rw.pi.eqs++
		case *ast.CallExpr:
			rw.vetHygiene(c, n)
		case *ast.SwitchStmt:
			if !rw.tags[n] {
				return true
			}
			tag := n.Tag
			n.Tag = nil
			for _, cc := range n.Body.List {
				cl := cc.(*ast.CaseClause)
				if cl.List == nil {
					continue
				}
				var cond ast.Expr
				for _, e := range cl.List {
					var x ast.Expr = &ast.CallExpr{Fun: rw.vmSel("EqualAny"), Args: []ast.Expr{tag, e}}
					if cond == nil {
						cond = x
					} else {
						cond = &ast.BinaryExpr{X: cond, Op: token.LOR, Y: x}
					}
				}
				cl.List = []ast.Expr{cond}
			}
			rw.pi.tagSwitches++
		}
		return true
	})
}

// vetHygiene rewrites, with the same semantics, the constructs of the copy
// that go vet rejects although it accepts them in the original:
//
//   - unsafe.Pointer(v.Pointer()) and unsafe.Pointer(v.UnsafeAddr()) are only
//     allowed by vet's unsafeptr check on reflect.Value: they become
//     v.UnsafePointer() and vm.UnsafeAddrOf(v);
//   - a printf wrapper called with a non-constant format and no arguments
//     (vet's printf check, stricter since Go 1.24) gets an explicit empty
//     argument list (nil...).
func (rw *rewriter) vetHygiene(c *astutil.Cursor, n *ast.CallExpr) {
	if len(n.Args) == 1 && rw.isUnsafePointerConv(n) {
		call, ok := unparen(n.Args[0]).(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isReflectValue(rw.info.TypeOf(sel.X)) {
			return
		}
		switch sel.Sel.Name {
		case "Pointer":
			c.Replace(&ast.CallExpr{Fun: &ast.SelectorExpr{X: sel.X, Sel: ast.NewIdent("UnsafePointer")}})
			rw.pi.vetFixes++
		case "UnsafeAddr":
			c.Replace(&ast.CallExpr{Fun: rw.vmSel("UnsafeAddrOf"), Args: []ast.Expr{sel.X}})
			rw.pi.vetFixes++
		}
		return
	}
	if n.Ellipsis.IsValid() || len(n.Args) == 0 {
		return
	}
	sig, ok := rw.info.TypeOf(n.Fun).(*types.Signature)
	if !ok || !sig.Variadic() || sig.Params().Len() != len(n.Args)+1 || sig.Params().Len() < 2 {
		return
	}
	last := sig.Params().At(sig.Params().Len() - 1).Type().(*types.Slice)
	if !emptyIface(last.Elem()) {
		return
	}
	format := sig.Params().At(sig.Params().Len() - 2)
	if !strings.Contains(strings.ToLower(format.Name()), "format") || !types.Identical(format.Type(), types.Typ[types.String]) {
		return
	}
	if tv, ok := rw.info.Types[n.Args[len(n.Args)-1]]; !ok || tv.Value != nil {
		return // constant format: accepted by vet
	}
	n.Args = append(n.Args, ast.NewIdent("nil"))
	n.Ellipsis = n.Rparen - 1
	rw.pi.vetFixes++
}

// isUnsafePointerConv reports whether n converts to unsafe.Pointer.
func (rw *rewriter) isUnsafePointerConv(n *ast.CallExpr) bool {
	tv, ok := rw.info.Types[n.Fun]
	if !ok || !tv.IsType() {
		return false
	}
	b, ok := tv.Type.(*types.Basic)
	return ok && b.Kind() == types.UnsafePointer
}

func isReflectValue(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "reflect" && n.Obj().Name() == "Value"
}

// typeCaches turns the package-level sync.Map variables into vm.TypeCache:
// the caches indexed by reflect.Type (encoding/json's encoderCache...) then
// keep the entries of interpreted types in the type descriptors, which go
// away with their machine, instead of keeping every unloaded plugin alive.
func (rw *rewriter) typeCaches() {
	n := 0
	for _, d := range rw.f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if vs.Type == nil || len(vs.Values) > 0 {
				continue
			}
			nt, ok := rw.info.TypeOf(vs.Type).(*types.Named)
			if !ok || nt.Obj().Pkg() == nil || nt.Obj().Pkg().Path() != "sync" || nt.Obj().Name() != "Map" {
				continue
			}
			vs.Type = rw.vmSel("TypeCache")
			n++
		}
	}
	if n > 0 {
		rw.pi.typeCaches += n
		if !astutil.UsesImport(rw.f, "sync") {
			astutil.DeleteImport(rw.g.fset, rw.f, "sync")
		}
	}
}

// errorsNest bounds the recursion of errors.Is, errors.As and errors.AsType
// (functions is, as and asType). They call themselves for each error of an
// Unwrap() []error, so an error tree with a cycle (an Unwrap() []error that
// returns its receiver, or an errors.Join that contains itself) grows the Go
// stack until gc kills the whole process. A plugin must stop with
// ErrStackOverflow instead: the case of the type switch for
// Unwrap() []error starts with
//
//	defer aotvm.NestAOT().Done()
//
// so each level counts against MaxNest (vm/nest.go). The defer runs when
// the function returns, which the case always does after its loop.
func (rw *rewriter) errorsNest() {
	if rw.pi.p.PkgPath != "errors" {
		return
	}
	for _, d := range rw.f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || fd.Body == nil {
			continue
		}
		switch fd.Name.Name {
		case "is", "as", "asType":
		default:
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok || len(cc.List) != 1 || !isUnwrapAll(cc.List[0]) {
				return true
			}
			call := &ast.CallExpr{Fun: &ast.SelectorExpr{
				X:   &ast.CallExpr{Fun: rw.vmSel("NestAOT")},
				Sel: ast.NewIdent("Done"),
			}}
			cc.Body = append([]ast.Stmt{&ast.DeferStmt{Call: call}}, cc.Body...)
			rw.pi.nestGuards++
			return false
		})
	}
}

// isUnwrapAll reports whether e is the type interface{ Unwrap() []error }.
func isUnwrapAll(e ast.Expr) bool {
	it, ok := e.(*ast.InterfaceType)
	if !ok || it.Methods == nil || len(it.Methods.List) != 1 {
		return false
	}
	m := it.Methods.List[0]
	ft, ok := m.Type.(*ast.FuncType)
	if !ok || len(m.Names) != 1 || m.Names[0].Name != "Unwrap" || ft.Params.NumFields() != 0 ||
		ft.Results.NumFields() != 1 {
		return false
	}
	at, ok := ft.Results.List[0].Type.(*ast.ArrayType)
	if !ok || at.Len != nil {
		return false
	}
	id, ok := at.Elt.(*ast.Ident)
	return ok && id.Name == "error"
}

// jsonPtrKey fixes the pointer cycle check of encoding/json. After 1000
// nested pointers, ptrEncoder.encode keeps the pointers it is inside in a
// map[any]struct{} (ptrSeen), with the key
//
//	ptr := v.Interface()
//
// For an interpreted pointer, the twin's Interface returns a new box at each
// call, so the key never matched: a pointer that points back to itself
// (directly or through other types) grew the Go stack until gc killed the
// whole process, where go run returns an UnsupportedValueError. The key
// becomes
//
//	ptr := aotvm.PtrKey(v)
//
// which is the same for the same pointer and type (vm/twin_value.go).
func (rw *rewriter) jsonPtrKey() {
	if rw.pi.p.PkgPath != "encoding/json" {
		return
	}
	for _, d := range rw.f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || fd.Body == nil || fd.Name.Name != "encode" || len(fd.Recv.List) != 1 {
			continue
		}
		if id, ok := fd.Recv.List[0].Type.(*ast.Ident); !ok || id.Name != "ptrEncoder" {
			continue
		}
		rw.pi.ptrKeyFuncs++
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			if id, ok := as.Lhs[0].(*ast.Ident); !ok || id.Name != "ptr" {
				return true
			}
			call, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) != 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Interface" {
				return true
			}
			as.Rhs[0] = &ast.CallExpr{Fun: rw.vmSel("PtrKey"), Args: []ast.Expr{sel.X}}
			rw.pi.ptrKeys++
			return false
		})
	}
}

// exitFuncs are the functions that end the process, and their
// replacements in package vm (vm/exit.go).
var exitFuncs = map[string]string{
	"os.Exit":     "AOTExit",
	"log.Fatal":   "AOTFatal",
	"log.Fatalf":  "AOTFatalf",
	"log.Fatalln": "AOTFatalln",
}

// httpDefaultFuncs use http.DefaultServeMux, a registry of the process,
// and their replacements in package vm (vm/hostregs.go): they use the
// DefaultServeMux of the machine of the goroutine. The functions with a
// handler argument use that mux for a nil handler.
var httpDefaultFuncs = map[string]string{
	"net/http.Handle":            "AOTHandle",
	"net/http.HandleFunc":        "AOTHandleFunc",
	"net/http.ListenAndServe":    "AOTListenAndServe",
	"net/http.ListenAndServeTLS": "AOTListenAndServeTLS",
	"net/http.Serve":             "AOTServe",
	"net/http.ServeTLS":          "AOTServeTLS",
}

// noHTTPDefaultFuncs serve http.DefaultServeMux for a nil Handler field and
// have no replacement yet: a copy that uses one is refused.
var noHTTPDefaultFuncs = map[string]bool{
	"(*net/http.Server).Serve":             true,
	"(*net/http.Server).ServeTLS":          true,
	"(*net/http.Server).ListenAndServe":    true,
	"(*net/http.Server).ListenAndServeTLS": true,
	"net/http/httptest.NewServer":          true,
	"net/http/httptest.NewTLSServer":       true,
	"net/http/httptest.NewUnstartedServer": true,
	"(*net/http/httptest.Server).Start":    true,
	"(*net/http/httptest.Server).StartTLS": true,
	"net/http/fcgi.Serve":                  true,
	"net/http/cgi.Serve":                   true,
}

// noExitFuncs end the process and have no replacement yet: a copy that
// uses one is refused.
var noExitFuncs = map[string]bool{
	"(*log.Logger).Fatal":   true,
	"(*log.Logger).Fatalf":  true,
	"(*log.Logger).Fatalln": true,
	"syscall.Exit":          true,
}

// processExits makes the copy follow the plugin's process functions. An
// AOT copy is compiled Go: its calls to os.Exit and log.Fatal end the host
// process, and it reads the host's os.Args (flag.Parse with
// flag.ExitOnError and a bad flag killed the host). They become
//
//	aotvm.AOTExit(2)
//	aotvm.AOTFatal(v...)
//	aotvm.AOTArgs()
//
// which use the os.Exit, log.Fatal and os.Args of the machine of the
// goroutine: they panic for a restricted plugin, like the plugin's own
// os.Exit. A function value (f := os.Exit) is replaced the same way.
// Assigning os.Args, or using a function of noExitFuncs, is an error.
//
// It does the same for http.DefaultServeMux, a registry of the process
// (vm/hostregs.go): the functions of httpDefaultFuncs become their
// replacement, and the reads of http.DefaultServeMux become
// aotvm.AOTDefaultServeMux(). The copy of net/rpc registers its HandleHTTP
// routes this way. Assigning http.DefaultServeMux, or using a function of
// noHTTPDefaultFuncs, is an error.
func (rw *rewriter) processExits() error {
	var err error
	astutil.Apply(rw.f, func(c *astutil.Cursor) bool {
		sel, ok := c.Node().(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch obj := rw.info.Uses[sel.Sel].(type) {
		case *types.Func:
			name := obj.FullName()
			if noExitFuncs[name] && err == nil {
				err = fmt.Errorf("%s ends the process: add its replacement to processExits", name)
			}
			if fn, ok := exitFuncs[name]; ok {
				c.Replace(rw.vmSel(fn))
				rw.pi.exits++
			}
			if noHTTPDefaultFuncs[name] && err == nil {
				err = fmt.Errorf("%s uses http.DefaultServeMux for a nil handler: add its replacement to processExits", name)
			}
			if fn, ok := httpDefaultFuncs[name]; ok {
				c.Replace(rw.vmSel(fn))
				rw.pi.httpDefaults++
			}
		case *types.Var:
			if obj.Pkg() != nil && obj.Pkg().Path() == "net/http" && obj.Name() == "DefaultServeMux" {
				if as, ok := c.Parent().(*ast.AssignStmt); ok && c.Name() == "Lhs" && err == nil {
					err = fmt.Errorf("http.DefaultServeMux is assigned at %s", rw.g.fset.Position(as.Pos()))
				}
				c.Replace(&ast.CallExpr{Fun: rw.vmSel("AOTDefaultServeMux")})
				rw.pi.httpDefaults++
				return true
			}
			if obj.Pkg() == nil || obj.Pkg().Path() != "os" || obj.Name() != "Args" {
				return true
			}
			if as, ok := c.Parent().(*ast.AssignStmt); ok && c.Name() == "Lhs" && err == nil {
				err = fmt.Errorf("os.Args is assigned at %s", rw.g.fset.Position(as.Pos()))
			}
			c.Replace(&ast.CallExpr{Fun: rw.vmSel("AOTArgs")})
			rw.pi.exits++
		}
		return true
	}, nil)
	for _, path := range []string{"os", "log", "net/http"} {
		if !astutil.UsesImport(rw.f, path) {
			astutil.DeleteImport(rw.g.fset, rw.f, path)
		}
	}
	return err
}

// jsonV2Cycle fixes the cycle check of encoding/json/v2, the code of
// encoding/json with GOEXPERIMENT=jsonv2 (and in later Go versions, where
// encode.go and ptrEncoder are gone). The check keys on
// typedPointer{v.Type(), v.UnsafePointer(), sliceLen(v)} in visitPointer
// and leavePointer: the twin's UnsafePointer is the identity of the VM
// pointer, map or slice, so pointers are fine. But maps and slices lost
// their identity before they reached the check:
//
//   - marshalValueAny has the type switch case map[string]any and case
//     []any, rewritten on aotvm.Unbox(val), which makes a new native copy at
//     each call. It becomes aotvm.NoUnbox(val): an interpreted map or slice
//     takes the reflect path of the function.
//
//   - that path, and the marshal func of makeInterfaceArshaler, copy the
//     value of the interface into an addressable value:
//
//     v := newAddressableValue(T)
//     v.Set(X)
//
//     For map[string]any or []any, New gives a native cell and Set a new
//     native copy. It becomes
//
//     v := addressableValue{aotvm.AddrCopy(X), true}
//
//     which keeps the VM map or slice (vm/twin_value.go).
//
//   - marshalEncode copies the top-level value the same way when it is not
//     a pointer:
//
//     v2 := reflect.New(v.Type())
//     v2.Elem().Set(v)
//     v = v2
//
//     It becomes v = aotvm.AddrCopy(v).Addr(). The native copy of an
//     interpreted []any or map[string]any also lost the types of its
//     elements: json.Marshal([]any{c}) of a Color c with a MarshalText
//     method encoded c as a struct in default mode (vm/aotroute.go).
//
// So a map or a slice that contains itself got "exceeded max depth" where
// go run returns an UnsupportedValueError. checkJSONCycle makes aotgen stop
// if a target is not found.
func (rw *rewriter) jsonV2Cycle() {
	if rw.pi.p.PkgPath != "encoding/json/v2" {
		return
	}
	for _, d := range rw.f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || fd.Body == nil {
			continue
		}
		switch fd.Name.Name {
		case "visitPointer", "leavePointer":
			if keysOnUnsafePointer(fd.Body) {
				rw.pi.jsonPtrChecks++
			}
		case "marshalValueAny":
			rw.jsonNoUnbox(fd.Body)
			rw.jsonAddrCopy(fd.Body)
		case "makeInterfaceArshaler":
			rw.jsonAddrCopy(fd.Body)
		case "marshalEncode":
			rw.jsonTopCopy(fd.Body)
		}
	}
}

// jsonTopCopy rewrites, in each block of body,
//
//	X := reflect.New(E.Type())
//	X.Elem().Set(E)
//	E = X
//
// to E = aotvm.AddrCopy(E).Addr() (see jsonV2Cycle).
func (rw *rewriter) jsonTopCopy(body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		b, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i := 0; i+2 < len(b.List); i++ {
			as, ok := b.List[i].(*ast.AssignStmt)
			if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				continue
			}
			x, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				continue
			}
			mk, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok || len(mk.Args) != 1 {
				continue
			}
			if sel, ok := mk.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "New" {
				continue
			}
			tc, ok := mk.Args[0].(*ast.CallExpr)
			if !ok || len(tc.Args) != 0 {
				continue
			}
			ts, ok := tc.Fun.(*ast.SelectorExpr)
			if !ok || ts.Sel.Name != "Type" {
				continue
			}
			e := exprString(rw.g.fset, ts.X)
			// X.Elem().Set(E)
			es, ok := b.List[i+1].(*ast.ExprStmt)
			if !ok {
				continue
			}
			set, ok := es.X.(*ast.CallExpr)
			if !ok || len(set.Args) != 1 || exprString(rw.g.fset, set.Args[0]) != e {
				continue
			}
			if exprString(rw.g.fset, set.Fun) != x.Name+".Elem().Set" {
				continue
			}
			// E = X
			back, ok := b.List[i+2].(*ast.AssignStmt)
			if !ok || back.Tok != token.ASSIGN || len(back.Lhs) != 1 || len(back.Rhs) != 1 ||
				exprString(rw.g.fset, back.Lhs[0]) != e || exprString(rw.g.fset, back.Rhs[0]) != x.Name {
				continue
			}
			back.Rhs[0] = &ast.CallExpr{Fun: &ast.SelectorExpr{
				X:   &ast.CallExpr{Fun: rw.vmSel("AddrCopy"), Args: []ast.Expr{ts.X}},
				Sel: ast.NewIdent("Addr"),
			}}
			b.List = append(b.List[:i], b.List[i+2:]...)
			rw.pi.jsonTopCopies++
		}
		return true
	})
}

// keysOnUnsafePointer reports whether body builds a typedPointer composite
// literal with a v.UnsafePointer() element.
func keysOnUnsafePointer(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return !found
		}
		if id, ok := cl.Type.(*ast.Ident); !ok || id.Name != "typedPointer" {
			return true
		}
		for _, e := range cl.Elts {
			if call, ok := e.(*ast.CallExpr); ok && len(call.Args) == 0 {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "UnsafePointer" {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// jsonNoUnbox rewrites switch val := aotvm.Unbox(val).(type) to
// aotvm.NoUnbox (see jsonV2Cycle).
func (rw *rewriter) jsonNoUnbox(body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSwitchStmt)
		if !ok {
			return true
		}
		as, ok := ts.Assign.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			return true
		}
		ta, ok := as.Rhs[0].(*ast.TypeAssertExpr)
		if !ok {
			return true
		}
		call, ok := ta.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Unbox" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != vmName {
			return true
		}
		sel.Sel = ast.NewIdent("NoUnbox")
		rw.pi.jsonNoUnboxes++
		return true
	})
}

// jsonAddrCopy rewrites, in each block of body,
//
//	v := newAddressableValue(reflect.TypeOf(Y)) // or (E.Type())
//	v.Set(reflect.ValueOf(Y))                   // or (E)
//
// to v := addressableValue{aotvm.AddrCopy(X), true} (see jsonV2Cycle).
func (rw *rewriter) jsonAddrCopy(body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		b, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i := 0; i+1 < len(b.List); i++ {
			as, ok := b.List[i].(*ast.AssignStmt)
			if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				continue
			}
			v, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				continue
			}
			mk, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok || len(mk.Args) != 1 {
				continue
			}
			if id, ok := mk.Fun.(*ast.Ident); !ok || id.Name != "newAddressableValue" {
				continue
			}
			es, ok := b.List[i+1].(*ast.ExprStmt)
			if !ok {
				continue
			}
			set, ok := es.X.(*ast.CallExpr)
			if !ok || len(set.Args) != 1 {
				continue
			}
			sel, ok := set.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Set" {
				continue
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != v.Name {
				continue
			}
			if !sameTypeOf(rw.g.fset, mk.Args[0], set.Args[0]) {
				continue
			}
			as.Rhs[0] = &ast.CompositeLit{
				Type: ast.NewIdent("addressableValue"),
				Elts: []ast.Expr{
					&ast.CallExpr{Fun: rw.vmSel("AddrCopy"), Args: []ast.Expr{set.Args[0]}},
					ast.NewIdent("true"),
				},
			}
			b.List = append(b.List[:i+1], b.List[i+2:]...)
			rw.pi.jsonAddrCopies++
		}
		return true
	})
}

// sameTypeOf reports whether typ is the type of the value x: typ is
// reflect.TypeOf(Y) and x is reflect.ValueOf(Y), or typ is E.Type() and x
// is E.
func sameTypeOf(fset *token.FileSet, typ, x ast.Expr) bool {
	tc, ok := typ.(*ast.CallExpr)
	if !ok {
		return false
	}
	ts, ok := tc.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch {
	case ts.Sel.Name == "Type" && len(tc.Args) == 0:
		return exprString(fset, ts.X) == exprString(fset, x)
	case ts.Sel.Name == "TypeOf" && len(tc.Args) == 1:
		xc, ok := x.(*ast.CallExpr)
		if !ok || len(xc.Args) != 1 {
			return false
		}
		xs, ok := xc.Fun.(*ast.SelectorExpr)
		if !ok || xs.Sel.Name != "ValueOf" {
			return false
		}
		return exprString(fset, ts.X) == exprString(fset, xs.X) &&
			exprString(fset, tc.Args[0]) == exprString(fset, xc.Args[0])
	}
	return false
}

func exprString(fset *token.FileSet, e ast.Expr) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, e); err != nil {
		return ""
	}
	return b.String()
}

// checkJSONCycle makes aotgen stop when it cannot protect the cycle check
// of encoding/json (jsonPtrKey for the code of encoding/json, jsonV2Cycle
// for encoding/json/v2): without it, a value that contains itself kills the
// host with a fatal stack overflow or gives a wrong error. A new Go version
// that changes the code must fail here, not produce a copy without the
// protection. usesV2 reports that encoding/json imports encoding/json/v2
// (GOEXPERIMENT=jsonv2, later Go versions); v2Converted that
// encoding/json/v2 is an AOT copy too.
func checkJSONCycle(path string, usesV2, v2Converted bool, pi *pinfo) error {
	switch path {
	case "encoding/json":
		if usesV2 {
			if !v2Converted {
				return fmt.Errorf("encoding/json: uses encoding/json/v2, which is not an AOT copy: its cycle check is not protected (jsonV2Cycle)")
			}
			if pi.ptrKeyFuncs > 0 {
				return fmt.Errorf("encoding/json: has ptrEncoder.encode and uses encoding/json/v2: update jsonPtrKey and jsonV2Cycle")
			}
			return nil
		}
		if pi.ptrKeyFuncs != 1 || pi.ptrKeys != 1 {
			// A new Go version changed ptrEncoder.encode: update jsonPtrKey.
			return fmt.Errorf("encoding/json: %d ptrEncoder.encode found with %d pointer keys rewritten to vm.PtrKey, want 1 and 1: update jsonPtrKey", pi.ptrKeyFuncs, pi.ptrKeys)
		}
	case "encoding/json/v2":
		if pi.jsonPtrChecks != 2 || pi.jsonNoUnboxes != 1 || pi.jsonAddrCopies != 2 || pi.jsonTopCopies != 1 {
			// A new Go version changed the cycle check: update jsonV2Cycle.
			return fmt.Errorf("encoding/json/v2: cycle check: %d of visitPointer and leavePointer keyed on UnsafePointer (want 2), %d Unbox made NoUnbox in marshalValueAny (want 1), %d copies made AddrCopy in marshalValueAny and makeInterfaceArshaler (want 2), %d top-level copies made AddrCopy in marshalEncode (want 1): update jsonV2Cycle",
				pi.jsonPtrChecks, pi.jsonNoUnboxes, pi.jsonAddrCopies, pi.jsonTopCopies)
		}
	}
	return nil
}

func (rw *rewriter) vmSel(name string) ast.Expr {
	rw.needVM = true
	return &ast.SelectorExpr{X: ast.NewIdent(vmName), Sel: ast.NewIdent(name)}
}

// fixFile remaps imports and neutralizes directives that make no sense in
// the copy (linknames, the part of the build constraints that the
// generation evaluated: fixConstraint).
func (rw *rewriter) fixFile() {
	if rw.pi.vetFixes > 0 && !astutil.UsesImport(rw.f, "unsafe") {
		astutil.DeleteImport(rw.g.fset, rw.f, "unsafe") // its only use was rewritten (vetHygiene)
	}
	for _, is := range rw.f.Imports {
		p, _ := strconv.Unquote(is.Path.Value)
		np := rw.g.importPathFor(p)
		if np == p {
			continue
		}
		if is.Name == nil {
			name := ""
			if isReflectPath(p) {
				name = base(p)
			} else if qi := rw.g.pkgs[p]; qi != nil {
				name = qi.p.Name
			}
			if name != "" {
				is.Name = ast.NewIdent(name)
			}
		}
		is.Path.Value = strconv.Quote(np)
	}
	if rw.needVM {
		astutil.AddNamedImport(rw.g.fset, rw.f, vmName, vmPath)
	}
	fixConstraint(rw.f, rw.kept)
	for _, cg := range rw.f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:linkname") {
				c.Text = "// aotgen: removed " + strings.TrimPrefix(c.Text, "//")
				rw.pi.linknames++
			}
		}
	}
}

func (rw *rewriter) print(name string) ([]byte, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by aotgen from %s (%s). DO NOT EDIT.\n\n", rw.pi.p.PkgPath, name)
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&buf, rw.g.fset, rw.f); err != nil {
		return nil, err
	}
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), nil // keep it unformatted: the build reports the error
	}
	return out, nil
}
