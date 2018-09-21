package interp

import (
	"path"
)

// Gta performs a global types analysis on the AST, registering types,
// variables and functions at package level, prior to CFG. All function
// bodies are skipped.
// GTA is necessary to handle out of order declarations and multiple
// source files packages.
func (interp *Interpreter) Gta(root *Node) {
	var pkgName string
	scope := interp.universe

	root.Walk(func(n *Node) bool {
		switch n.kind {
		case Define:
			if len(n.child) > 1 {
				scope.sym[n.child[0].ident] = &Symbol{
					kind:   Var,
					global: scope.global,
					index:  scope.inc(interp),
					typ:    nodeType(interp, scope, n.child[1]),
				}
			} else {
				scope.sym[n.child[0].ident] = &Symbol{
					kind:   Var,
					global: scope.global,
					index:  scope.inc(interp),
					typ:    nodeType(interp, scope, n.anc.child[0].child[1]),
				}
			}
			return false

		case File:
			pkgName = n.child[0].ident
			if _, ok := interp.scope[pkgName]; !ok {
				interp.scope[pkgName] = scope.push(0)
			}
			scope = interp.scope[pkgName]

		case FuncDecl:
			scope.sym[n.child[1].ident] = &Symbol{
				kind: Func,
				typ:  nodeType(interp, scope, n.child[2]),
				node: n,
			}
			return false

		case ImportSpec:
			var name, ipath string
			if len(n.child) == 2 {
				ipath = n.child[1].val.(string)
				name = n.child[0].ident
			} else {
				ipath = n.child[0].val.(string)
				name = path.Base(ipath)
			}
			if pkg, ok := interp.binValue[ipath]; ok {
				if name == "." {
					for n, s := range pkg {
						scope.sym[n] = &Symbol{typ: &Type{cat: BinT}, val: s}
					}
				} else {
					scope.sym[name] = &Symbol{typ: &Type{cat: BinPkgT}, path: ipath}
				}
			} else {
				// TODO: make sure we do not import a src package more than once
				interp.importSrcFile(ipath)
				scope.sym[name] = &Symbol{typ: &Type{cat: SrcPkgT}, path: ipath}
			}

		case TypeSpec:
			if n.child[1].kind == Ident {
				n.typ = &Type{cat: AliasT, val: nodeType(interp, scope, n.child[1])}
			} else {
				n.typ = nodeType(interp, scope, n.child[1])
			}
			scope.sym[n.child[0].ident] = &Symbol{kind: Typ, typ: n.typ}
			return false

		}
		return true
	}, nil)
}
