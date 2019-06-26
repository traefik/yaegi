package interp

import (
	"path"
	"reflect"
)

// gta performs a global types analysis on the AST, registering types,
// variables and functions symbols at package level, prior to CFG.
// All function bodies are skipped. GTA is necessary to handle out of
// order declarations and multiple source files packages.
func (interp *Interpreter) gta(root *node, rpath string) error {
	scope, _ := interp.initScopePkg(root)
	var err error
	var iotaValue int

	root.Walk(func(n *node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case constDecl:
			iotaValue = 0

		case blockStmt:
			if n != root {
				return false // skip statement block if not the entry point
			}

		case defineStmt:
			var atyp *itype
			if n.nleft+n.nright < len(n.child) {
				if atyp, err = nodeType(interp, scope, n.child[n.nleft]); err != nil {
					return false
				}
			}

			var sbase int
			if n.nright > 0 {
				sbase = len(n.child) - n.nright
			}

			for i := 0; i < n.nleft; i++ {
				dest, src := n.child[i], n.child[sbase+i]
				typ := atyp
				val := reflect.ValueOf(iotaValue)
				if typ == nil {
					if typ, err = nodeType(interp, scope, src); err != nil {
						return false
					}
					val = src.rval
				}
				var index int
				if !typ.incomplete {
					if typ.cat == nilT {
						err = n.cfgErrorf("use of untyped nil")
						return false
					}
					index = scope.add(typ)
				}
				scope.sym[dest.ident] = &symbol{kind: varSym, global: true, index: index, typ: typ, rval: val}
				if n.anc.kind == constDecl {
					iotaValue++
				}
			}
			return false

		case defineXStmt:
			// TODO: handle global DefineX
			//err = n.cfgError("global DefineX not implemented")

		case valueSpec:
			// TODO: handle global ValueSpec
			//err = n.cfgError("global ValueSpec not implemented")

		case funcDecl:
			if n.typ, err = nodeType(interp, scope, n.child[2]); err != nil {
				return false
			}
			if !isMethod(n) {
				scope.sym[n.child[1].ident] = &symbol{kind: funcSym, typ: n.typ, node: n, index: -1}
			}
			if len(n.child[0].child) > 0 {
				// function is a method, add it to the related type
				var rcvrtype *itype
				var typeName string
				n.ident = n.child[1].ident
				rcvr := n.child[0].child[0]
				if len(rcvr.child) < 2 {
					// Receiver var name is skipped in method declaration (fix that in AST ?)
					typeName = rcvr.child[0].ident
				} else {
					typeName = rcvr.child[1].ident
				}
				if typeName == "" {
					// The receiver is a pointer, retrieve typeName from indirection
					typeName = rcvr.lastChild().child[0].ident
					elementType := scope.getType(typeName)
					if elementType == nil {
						// Add type if necessary, so method can be registered
						scope.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, pkgPath: rpath}}
						elementType = scope.sym[typeName].typ
					}
					rcvrtype = &itype{cat: ptrT, val: elementType}
					elementType.method = append(elementType.method, n)
				} else {
					rcvrtype = scope.getType(typeName)
					if rcvrtype == nil {
						// Add type if necessary, so method can be registered
						scope.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, pkgPath: rpath}}
						rcvrtype = scope.sym[typeName].typ
					}
				}
				rcvrtype.method = append(rcvrtype.method, n)
			}
			return false

		case importSpec:
			var name, ipath string
			if len(n.child) == 2 {
				ipath = n.child[1].rval.String()
				name = n.child[0].ident
			} else {
				ipath = n.child[0].rval.String()
				name = path.Base(ipath)
			}
			if interp.binPkg[ipath] != nil {
				if name == "." {
					for n, v := range interp.binPkg[ipath] {
						typ := v.Type()
						if isBinType(v) {
							typ = typ.Elem()
						}
						scope.sym[n] = &symbol{kind: binSym, typ: &itype{cat: valueT, rtype: typ}, rval: v}
					}
				} else {
					scope.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT}, path: ipath}
				}
			} else {
				// TODO: make sure we do not import a src package more than once
				err = interp.importSrcFile(rpath, ipath, name)
				scope.types = interp.universe.types
				scope.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: srcPkgT}, path: ipath}
			}

		case typeSpec:
			typeName := n.child[0].ident
			var typ *itype
			if typ, err = nodeType(interp, scope, n.child[1]); err != nil {
				return false
			}
			if n.child[1].kind == identExpr {
				n.typ = &itype{cat: aliasT, val: typ, name: typeName, pkgPath: rpath}
			} else {
				n.typ = typ
				n.typ.name = typeName
				n.typ.pkgPath = rpath
			}
			// Type may already be declared for a receiver in a method function
			if scope.sym[typeName] == nil {
				scope.sym[typeName] = &symbol{kind: typeSym}
			} else {
				n.typ.method = append(n.typ.method, scope.sym[typeName].typ.method...)
			}
			scope.sym[typeName].typ = n.typ
			return false
		}
		return true
	}, nil)

	if scope != interp.universe {
		scope.pop()
	}
	return err
}
