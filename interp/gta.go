package interp

import (
	"reflect"
)

// gta performs a global types analysis on the AST, registering types,
// variables and functions symbols at package level, prior to CFG.
// All function bodies are skipped. GTA is necessary to handle out of
// order declarations and multiple source files packages.
func (interp *Interpreter) gta(root *node, rpath, pkgID string) ([]*node, error) {
	sc := interp.initScopePkg(pkgID)
	var err error
	var iotaValue int
	var revisit []*node

	root.Walk(func(n *node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case constDecl:
			iotaValue = 0
			_, err = interp.cfg(n, pkgID)

		case blockStmt:
			if n != root {
				return false // skip statement block if not the entry point
			}

		case defineStmt:
			var atyp *itype
			if n.nleft+n.nright < len(n.child) {
				// Type is declared explicitly in the assign expression.
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
				val := reflect.ValueOf(iotaValue)
				typ := atyp
				if typ == nil {
					if typ, err = nodeType(interp, sc, src); err != nil {
						return false
					}
					val = src.rval
				}
				if typ.incomplete {
					// Come back when type is known.
					revisit = append(revisit, n)
					return false
				}
				if typ.cat == nilT {
					err = n.cfgErrorf("use of untyped nil")
					return false
				}
				if typ.isBinMethod {
					typ = &itype{cat: valueT, rtype: typ.methodCallType(), isBinMethod: true}
				}
				if sc.sym[dest.ident] == nil {
					sc.sym[dest.ident] = &symbol{kind: varSym, global: true, index: sc.add(typ), typ: typ, rval: val}
				}
				if n.anc.kind == constDecl {
					sc.sym[dest.ident].kind = constSym
					iotaValue++
				}
			}
			return false

		case defineXStmt:
			err = compDefineX(sc, n)

		case valueSpec:
			l := len(n.child) - 1
			if n.typ = n.child[l].typ; n.typ == nil {
				if n.typ, err = nodeType(interp, sc, n.child[l]); err != nil {
					return false
				}
				if n.typ.incomplete {
					// Come back when type is known.
					revisit = append(revisit, n)
					return false
				}
			}
			for _, c := range n.child[:l] {
				sc.sym[c.ident] = &symbol{index: sc.add(n.typ), kind: varSym, global: true, typ: n.typ}
			}

		case funcDecl:
			if n.typ, err = nodeType(interp, sc, n.child[2]); err != nil {
				return false
			}
			if isMethod(n) {
				// Add a method symbol in the receiver type name space
				var rcvrtype *itype
				n.ident = n.child[1].ident
				rcvr := n.child[0].child[0]
				rtn := rcvr.lastChild()
				typeName := rtn.ident
				if typeName == "" {
					// The receiver is a pointer, retrieve typeName from indirection
					typeName = rtn.child[0].ident
					elementType := sc.getType(typeName)
					if elementType == nil {
						// Add type if necessary, so method can be registered
						sc.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, path: rpath, incomplete: true, node: rtn.child[0], scope: sc}}
						elementType = sc.sym[typeName].typ
					}
					rcvrtype = &itype{cat: ptrT, val: elementType, incomplete: elementType.incomplete, node: rtn, scope: sc}
					elementType.method = append(elementType.method, n)
				} else {
					rcvrtype = sc.getType(typeName)
					if rcvrtype == nil {
						// Add type if necessary, so method can be registered
						sc.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, path: rpath, incomplete: true, node: rtn, scope: sc}}
						rcvrtype = sc.sym[typeName].typ
					}
				}
				rcvrtype.method = append(rcvrtype.method, n)
			} else {
				// Add a function symbol in the package name space
				sc.sym[n.child[1].ident] = &symbol{kind: funcSym, typ: n.typ, node: n, index: -1}
			}
			if n.typ.incomplete {
				revisit = append(revisit, n)
			}
			return false

		case importSpec:
			var name, ipath string
			if len(n.child) == 2 {
				ipath = n.child[1].rval.String()
				name = n.child[0].ident
			} else {
				ipath = n.child[0].rval.String()
				name = identifier.FindString(ipath)
			}
			// Try to import a binary package first, or a source package
			if interp.binPkg[ipath] != nil {
				switch name {
				case "_": // no import of symbols
				case ".": // import symbols in current scope
					for n, v := range interp.binPkg[ipath] {
						typ := v.Type()
						if isBinType(v) {
							typ = typ.Elem()
						}
						sc.sym[n] = &symbol{kind: binSym, typ: &itype{cat: valueT, rtype: typ}, rval: v}
					}
				default: // import symbols in package namespace
					sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT, path: ipath}}
				}
			} else if err = interp.importSrc(rpath, ipath); err == nil {
				sc.types = interp.universe.types
				switch name {
				case "_": // no import of symbols
				case ".": // import symbols in current namespace
					for k, v := range interp.srcPkg[ipath] {
						if canExport(k) {
							sc.sym[k] = v
						}
					}
				default: // import symbols in package namespace
					sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: srcPkgT, path: ipath}}
				}
			} else {
				err = n.cfgErrorf("import %q error: %v", ipath, err)
			}

		case typeSpec:
			typeName := n.child[0].ident
			var typ *itype
			if typ, err = nodeType(interp, sc, n.child[1]); err != nil {
				return false
			}
			if n.child[1].kind == identExpr {
				n.typ = &itype{cat: aliasT, val: typ, name: typeName, path: rpath, field: typ.field, incomplete: typ.incomplete}
				copy(n.typ.method, typ.method)
			} else {
				n.typ = typ
				n.typ.name = typeName
				n.typ.path = rpath
			}
			// Type may be already declared for a receiver in a method function
			if sc.sym[typeName] == nil {
				sc.sym[typeName] = &symbol{kind: typeSym}
			} else {
				n.typ.method = append(n.typ.method, sc.sym[typeName].typ.method...)
			}
			sc.sym[typeName].typ = n.typ
			if n.typ.incomplete {
				revisit = append(revisit, n)
			}
			return false
		}
		return true
	}, nil)

	if sc != interp.universe {
		sc.pop()
	}
	return revisit, err
}
