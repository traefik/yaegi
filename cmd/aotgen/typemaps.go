package main

// Package-level maps indexed by reflect.Type become vm.TypeMap (see
// vm/typecache.go): yaml.v3's structMap, the field caches of toml and
// sigs.k8s.io/yaml, encoding/gob's types. A map is rewritten only when every
// use is one TypeMap supports: m[k], v, ok := m[k], m[k] = v, delete(m, k),
// m == nil, m != nil, and m = make(...) or an empty composite literal. A
// TypeMap declared with make starts nil (IsNil): no generated copy tests a
// map declared that way against nil.

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ast/astutil"
)

// typeMapKey reports whether v is a map indexed by reflect.Type.
func (rw *rewriter) typeMapKey(v *types.Var) bool {
	m, ok := v.Type().Underlying().(*types.Map)
	if !ok {
		return false
	}
	n, ok := types.Unalias(m.Key()).(*types.Named)
	return ok && n.Obj().Pkg() != nil && isReflectPath(n.Obj().Pkg().Path()) && n.Obj().Name() == "Type"
}

// objOf returns the variable an identifier or a field selector denotes.
func (rw *rewriter) objOf(e ast.Expr) *types.Var {
	switch e := unparen(e).(type) {
	case *ast.Ident:
		v, _ := rw.info.Uses[e].(*types.Var)
		return v
	case *ast.SelectorExpr:
		v, _ := rw.info.Uses[e.Sel].(*types.Var)
		return v
	}
	return nil
}

// typeMapTargets returns the maps of the package to rewrite, with the map
// type of their declaration: candidates are found in every file, and a
// candidate used in an unsupported way in any file is left alone.
func typeMapTargets(files []*ast.File, info *types.Info, pkg *types.Package) map[*types.Var]*ast.MapType {
	rw := &rewriter{info: info}
	cand := map[*types.Var]*ast.MapType{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for _, name := range vs.Names {
					v, ok := info.Defs[name].(*types.Var)
					if !ok || v.Parent() != pkg.Scope() {
						continue
					}
					if rw.typeMapKey(v) && len(vs.Names) == 1 {
						if mt := declMapType(vs); mt != nil {
							cand[v] = mt
						}
					}
					if st, ok := vs.Type.(*ast.StructType); ok {
						for _, fl := range st.Fields.List {
							for _, fn := range fl.Names {
								if fv, ok := info.Defs[fn].(*types.Var); ok && rw.typeMapKey(fv) && len(fl.Names) == 1 {
									if mt, isMap := fl.Type.(*ast.MapType); isMap {
										cand[fv] = mt
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if len(cand) == 0 {
		return nil
	}
	// Every use must be a supported one; parents are needed to tell.
	bad := map[*types.Var]bool{}
	for _, f := range files {
		astutil.Apply(f, func(c *astutil.Cursor) bool {
			e, isExpr := c.Node().(ast.Expr)
			if !isExpr {
				return true
			}
			v := rw.objOf(e)
			if v == nil || cand[v] == nil {
				return true
			}
			switch p := c.Parent().(type) {
			case *ast.IndexExpr:
				if p.X == e {
					return false
				}
			case *ast.BinaryExpr:
				if (p.Op == token.EQL || p.Op == token.NEQ) && (rw.isNil(p.X) || rw.isNil(p.Y)) {
					return false
				}
			case *ast.AssignStmt:
				if len(p.Lhs) == 1 && p.Lhs[0] == e && p.Tok == token.ASSIGN {
					switch r := unparen(p.Rhs[0]).(type) {
					case *ast.CompositeLit:
						return false
					case *ast.CallExpr:
						if id, isId := r.Fun.(*ast.Ident); isId && id.Name == "make" {
							return false
						}
					}
				}
			case *ast.CallExpr:
				if id, isId := p.Fun.(*ast.Ident); isId && id.Name == "delete" && len(p.Args) == 2 && p.Args[0] == e {
					return false
				}
			case *ast.SelectorExpr:
				return true
			}
			bad[v] = true // any other use (range, len, argument...)
			return false
		}, nil)
	}
	for v := range bad {
		delete(cand, v)
	}
	return cand
}

// typeMaps rewrites the uses and declarations of the package's TypeMap
// targets in the file.
func (rw *rewriter) typeMaps() {
	cand := rw.pi.typeMapTargets
	if len(cand) == 0 {
		return
	}
	ok := map[*types.Var]bool{}
	for v := range cand {
		ok[v] = true
	}
	// Rewrite the uses (assignments before their index expressions), then
	// the declarations.
	astutil.Apply(rw.f, func(c *astutil.Cursor) bool {
		switch n := c.Node().(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) == 1 && n.Tok == token.ASSIGN {
				if ix, isIx := n.Lhs[0].(*ast.IndexExpr); isIx && rw.target(ok, ix.X) {
					c.Replace(&ast.ExprStmt{X: rw.call(ix.X, "Set", ix.Index, n.Rhs[0])})
					return false
				}
				if rw.target(ok, n.Lhs[0]) {
					c.Replace(&ast.ExprStmt{X: rw.call(n.Lhs[0], "Init")})
					return false
				}
			}
			if len(n.Lhs) == 2 && len(n.Rhs) == 1 {
				if ix, isIx := unparen(n.Rhs[0]).(*ast.IndexExpr); isIx && rw.target(ok, ix.X) {
					n.Rhs[0] = rw.call(ix.X, "Lookup", ix.Index)
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) == 2 && len(n.Values) == 1 {
				if ix, isIx := unparen(n.Values[0]).(*ast.IndexExpr); isIx && rw.target(ok, ix.X) {
					n.Values[0] = rw.call(ix.X, "Lookup", ix.Index)
				}
			}
		}
		return true
	}, func(c *astutil.Cursor) bool {
		switch n := c.Node().(type) {
		case *ast.IndexExpr:
			if rw.target(ok, n.X) {
				c.Replace(rw.call(n.X, "Get", n.Index))
			}
		case *ast.BinaryExpr:
			if (n.Op == token.EQL || n.Op == token.NEQ) && (rw.target(ok, n.X) || rw.target(ok, n.Y)) {
				x := n.X
				if !rw.target(ok, x) {
					x = n.Y
				}
				var e ast.Expr = rw.call(x, "IsNil")
				if n.Op == token.NEQ {
					e = &ast.UnaryExpr{Op: token.NOT, X: e}
				}
				c.Replace(e)
			}
		case *ast.CallExpr:
			if id, isId := n.Fun.(*ast.Ident); isId && id.Name == "delete" && len(n.Args) == 2 && rw.target(ok, n.Args[0]) {
				c.Replace(rw.call(n.Args[0], "Delete", n.Args[1]))
			}
		}
		return true
	})
	for _, d := range rw.f.Decls {
		gd, isGen := d.(*ast.GenDecl)
		if !isGen || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for _, name := range vs.Names {
				v, _ := rw.info.Defs[name].(*types.Var)
				if v != nil && ok[v] {
					vs.Type = rw.typeMapType(cand[v])
					vs.Values = nil
					rw.pi.typeMaps++
				}
			}
			if st, isSt := vs.Type.(*ast.StructType); isSt {
				for _, f := range st.Fields.List {
					for _, fn := range f.Names {
						if fv, _ := rw.info.Defs[fn].(*types.Var); fv != nil && ok[fv] {
							f.Type = rw.typeMapType(cand[fv])
							rw.pi.typeMaps++
						}
					}
				}
			}
		}
	}
}

// target reports whether e denotes a map being rewritten.
func (rw *rewriter) target(ok map[*types.Var]bool, e ast.Expr) bool {
	v := rw.objOf(e)
	return v != nil && ok[v]
}

func (rw *rewriter) call(recv ast.Expr, method string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ast.NewIdent(method)}, Args: args}
}

// typeMapType returns the expression vm.TypeMap[V] for a map type.
func (rw *rewriter) typeMapType(mt *ast.MapType) ast.Expr {
	return &ast.IndexExpr{X: rw.vmSel("TypeMap"), Index: mt.Value}
}

// declMapType returns the map type of a variable declaration: its type, or
// the type given to make or to a composite literal.
func declMapType(vs *ast.ValueSpec) *ast.MapType {
	if mt, ok := vs.Type.(*ast.MapType); ok {
		return mt
	}
	if vs.Type != nil || len(vs.Values) != 1 {
		return nil
	}
	switch v := unparen(vs.Values[0]).(type) {
	case *ast.CallExpr:
		if id, ok := v.Fun.(*ast.Ident); ok && id.Name == "make" && len(v.Args) > 0 {
			mt, _ := v.Args[0].(*ast.MapType)
			return mt
		}
	case *ast.CompositeLit:
		if len(v.Elts) > 0 {
			return nil // the rewritten declaration has no initial entries
		}
		mt, _ := v.Type.(*ast.MapType)
		return mt
	}
	return nil
}
