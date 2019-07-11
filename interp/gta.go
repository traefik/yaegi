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
	sc, _ := interp.initScopePkg(root)
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
				if atyp, err = nodeType(interp, sc, n.child[n.nleft]); err != nil {
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
					if typ, err = nodeType(interp, sc, src); err != nil {
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
					index = sc.add(typ)
				}
				sc.sym[dest.ident] = &symbol{kind: varSym, global: true, index: index, typ: typ, rval: val}
				if n.anc.kind == constDecl {
					sc.sym[dest.ident].kind = constSym
					iotaValue++
				}
			}
			return false

		case defineXStmt:
			err = compDefineX(sc, n)

		case valueSpec:
			// TODO: handle global ValueSpec
			//err = n.cfgError("global ValueSpec not implemented")

		case funcDecl:
			if n.typ, err = nodeType(interp, sc, n.child[2]); err != nil {
				return false
			}
			if !isMethod(n) {
				sc.sym[n.child[1].ident] = &symbol{kind: funcSym, typ: n.typ, node: n, index: -1}
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
					elementType := sc.getType(typeName)
					if elementType == nil {
						// Add type if necessary, so method can be registered
						sc.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, pkgPath: rpath}}
						elementType = sc.sym[typeName].typ
					}
					rcvrtype = &itype{cat: ptrT, val: elementType}
					elementType.method = append(elementType.method, n)
				} else {
					rcvrtype = sc.getType(typeName)
					if rcvrtype == nil {
						// Add type if necessary, so method can be registered
						sc.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, pkgPath: rpath}}
						rcvrtype = sc.sym[typeName].typ
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
						sc.sym[n] = &symbol{kind: binSym, typ: &itype{cat: valueT, rtype: typ}, rval: v}
					}
				} else {
					sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT}, path: ipath}
				}
			} else {
				// TODO: make sure we do not import a src package more than once
				err = interp.importSrcFile(rpath, ipath, name)
				sc.types = interp.universe.types
				sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: srcPkgT}, path: ipath}
			}

		case typeSpec:
			typeName := n.child[0].ident
			var typ *itype
			if typ, err = nodeType(interp, sc, n.child[1]); err != nil {
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
			if sc.sym[typeName] == nil {
				sc.sym[typeName] = &symbol{kind: typeSym}
			} else {
				n.typ.method = append(n.typ.method, sc.sym[typeName].typ.method...)
			}
			sc.sym[typeName].typ = n.typ
			return false
		}
		return true
	}, nil)

	if sc != interp.universe {
		sc.pop()
	}
	return err
}
