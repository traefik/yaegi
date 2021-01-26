package interp

import (
	"path/filepath"
	"reflect"
)

// gta performs a global types analysis on the AST, registering types,
// variables and functions symbols at package level, prior to CFG.
// All function bodies are skipped. GTA is necessary to handle out of
// order declarations and multiple source files packages.
// rpath is the relative path to the directory containing the source for the package.
func (interp *Interpreter) gta(root *node, rpath, importPath string) ([]*node, error) {
	sc := interp.initScopePkg(importPath)
	var err error
	var revisit []*node

	baseName := filepath.Base(interp.fset.Position(root.pos).Filename)

	root.Walk(func(n *node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case constDecl:
			// Early parse of constDecl subtree, to compute all constant
			// values which may be used in further declarations.
			if _, err = interp.cfg(n, importPath); err != nil {
				// No error processing here, to allow recovery in subtree nodes.
				// TODO(marc): check for a non recoverable error and return it for better diagnostic.
				err = nil
			}

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
				val := reflect.ValueOf(sc.iota)
				if n.anc.kind == constDecl {
					if _, err2 := interp.cfg(n, importPath); err2 != nil {
						// Constant value can not be computed yet.
						// Come back when child dependencies are known.
						revisit = append(revisit, n)
						return false
					}
				}
				typ := atyp
				if typ == nil {
					if typ, err = nodeType(interp, sc, src); err != nil {
						return false
					}
					val = src.rval
				}
				if !typ.isComplete() {
					// Come back when type is known.
					revisit = append(revisit, n)
					return false
				}
				if typ.cat == nilT {
					err = n.cfgErrorf("use of untyped nil")
					return false
				}
				if typ.isBinMethod {
					typ = &itype{cat: valueT, rtype: typ.methodCallType(), isBinMethod: true, scope: sc}
				}
				if sc.sym[dest.ident] == nil || sc.sym[dest.ident].typ.incomplete {
					sc.sym[dest.ident] = &symbol{kind: varSym, global: true, index: sc.add(typ), typ: typ, rval: val, node: n}
				}
				if n.anc.kind == constDecl {
					sc.sym[dest.ident].kind = constSym
					if childPos(n) == len(n.anc.child)-1 {
						sc.iota = 0
					} else {
						sc.iota++
					}
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
				if !n.typ.isComplete() {
					// Come back when type is known.
					revisit = append(revisit, n)
					return false
				}
			}
			for _, c := range n.child[:l] {
				asImportName := filepath.Join(c.ident, baseName)
				sym, exists := sc.sym[asImportName]
				if !exists {
					sc.sym[c.ident] = &symbol{index: sc.add(n.typ), kind: varSym, global: true, typ: n.typ, node: n}
					continue
				}
				c.level = globalFrame

				// redeclaration error
				if sym.typ.node != nil && sym.typ.node.anc != nil {
					prevDecl := n.interp.fset.Position(sym.typ.node.anc.pos)
					err = n.cfgErrorf("%s redeclared in this block\n\tprevious declaration at %v", c.ident, prevDecl)
					return false
				}
				err = n.cfgErrorf("%s redeclared in this block", c.ident)
				return false
			}

		case funcDecl:
			if n.typ, err = nodeType(interp, sc, n.child[2]); err != nil {
				return false
			}
			ident := n.child[1].ident
			switch {
			case isMethod(n):
				// Add a method symbol in the receiver type name space
				var rcvrtype *itype
				n.ident = ident
				rcvr := n.child[0].child[0]
				rtn := rcvr.lastChild()
				typeName := rtn.ident
				if typeName == "" {
					// The receiver is a pointer, retrieve typeName from indirection
					typeName = rtn.child[0].ident
					elementType := sc.getType(typeName)
					if elementType == nil {
						// Add type if necessary, so method can be registered
						sc.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, path: importPath, incomplete: true, node: rtn.child[0], scope: sc}}
						elementType = sc.sym[typeName].typ
					}
					rcvrtype = &itype{cat: ptrT, val: elementType, incomplete: elementType.incomplete, node: rtn, scope: sc}
					elementType.method = append(elementType.method, n)
				} else {
					rcvrtype = sc.getType(typeName)
					if rcvrtype == nil {
						// Add type if necessary, so method can be registered
						sc.sym[typeName] = &symbol{kind: typeSym, typ: &itype{name: typeName, path: importPath, incomplete: true, node: rtn, scope: sc}}
						rcvrtype = sc.sym[typeName].typ
					}
				}
				rcvrtype.method = append(rcvrtype.method, n)
				n.child[0].child[0].lastChild().typ = rcvrtype
			case ident == "init":
				// init functions do not get declared as per the Go spec.
			default:
				asImportName := filepath.Join(ident, baseName)
				if _, exists := sc.sym[asImportName]; exists {
					// redeclaration error
					err = n.cfgErrorf("%s redeclared in this block", ident)
					return false
				}
				// Add a function symbol in the package name space except for init
				sc.sym[n.child[1].ident] = &symbol{kind: funcSym, typ: n.typ, node: n, index: -1}
			}
			if !n.typ.isComplete() {
				revisit = append(revisit, n)
			}
			return false

		case importSpec:
			var name, ipath string
			if len(n.child) == 2 {
				ipath = constToString(n.child[1].rval)
				name = n.child[0].ident
			} else {
				ipath = constToString(n.child[0].rval)
			}
			// Try to import a binary package first, or a source package
			var pkgName string
			if interp.binPkg[ipath] != nil {
				switch name {
				case "_": // no import of symbols
				case ".": // import symbols in current scope
					for n, v := range interp.binPkg[ipath] {
						typ := v.Type()
						if isBinType(v) {
							typ = typ.Elem()
						}
						sc.sym[n] = &symbol{kind: binSym, typ: &itype{cat: valueT, rtype: typ, scope: sc}, rval: v}
					}
				default: // import symbols in package namespace
					if name == "" {
						name = identifier.FindString(ipath)
					}
					// imports of a same package are all mapped in the same scope, so we cannot just
					// map them by their names, otherwise we could have collisions from same-name
					// imports in different source files of the same package. Therefore, we suffix
					// the key with the basename of the source file.
					name = filepath.Join(name, baseName)
					if _, exists := sc.sym[name]; !exists {
						sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT, path: ipath, scope: sc}}
						break
					}

					// redeclaration error. Not caught by the parser.
					err = n.cfgErrorf("%s redeclared in this block", name)
					return false
				}
			} else if pkgName, err = interp.importSrc(rpath, ipath, NoTest); err == nil {
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
					if name == "" {
						name = pkgName
					}
					name = filepath.Join(name, baseName)
					if _, exists := sc.sym[name]; !exists {
						sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: srcPkgT, path: ipath, scope: sc}}
						break
					}

					// redeclaration error
					err = n.cfgErrorf("%s redeclared as imported package name", name)
					return false
				}
			} else {
				err = n.cfgErrorf("import %q error: %v", ipath, err)
			}

		case typeSpec:
			typeName := n.child[0].ident
			var typ *itype
			if typ, err = nodeType(interp, sc, n.child[1]); err != nil {
				err = nil
				revisit = append(revisit, n)
				return false
			}

			switch n.child[1].kind {
			case identExpr, selectorExpr:
				n.typ = &itype{cat: aliasT, val: typ, name: typeName, path: importPath, field: typ.field, incomplete: typ.incomplete, scope: sc, node: n.child[0]}
				copy(n.typ.method, typ.method)
			default:
				n.typ = typ
				n.typ.name = typeName
				n.typ.path = importPath
			}

			asImportName := filepath.Join(typeName, baseName)
			if _, exists := sc.sym[asImportName]; exists {
				// redeclaration error
				err = n.cfgErrorf("%s redeclared in this block", typeName)
				return false
			}
			sym, exists := sc.sym[typeName]
			if !exists {
				sc.sym[typeName] = &symbol{kind: typeSym}
			} else {
				if sym.typ != nil && (len(sym.typ.method) > 0) {
					// Type has already been seen as a receiver in a method function
					n.typ.method = append(n.typ.method, sym.typ.method...)
				} else {
					// TODO(mpl): figure out how to detect redeclarations without breaking type aliases.
					// Allow redeclarations for now.
					sc.sym[typeName] = &symbol{kind: typeSym}
				}
			}
			sc.sym[typeName].typ = n.typ
			if !n.typ.isComplete() {
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

// gtaRetry (re)applies gta until all global constants and types are defined.
func (interp *Interpreter) gtaRetry(nodes []*node, importPath string) error {
	revisit := []*node{}
	for {
		for _, n := range nodes {
			list, err := interp.gta(n, importPath, importPath)
			if err != nil {
				return err
			}
			revisit = append(revisit, list...)
		}

		if len(revisit) == 0 || equalNodes(nodes, revisit) {
			break
		}

		nodes = revisit
		revisit = []*node{}
	}

	if len(revisit) > 0 {
		n := revisit[0]
		if n.kind == typeSpec {
			if err := definedType(n.typ); err != nil {
				return err
			}
		}
		return n.cfgErrorf("constant definition loop")
	}
	return nil
}

func definedType(typ *itype) error {
	if !typ.incomplete {
		return nil
	}
	switch typ.cat {
	case interfaceT, structT:
		for _, f := range typ.field {
			if err := definedType(f.typ); err != nil {
				return err
			}
		}
	case funcT:
		for _, t := range typ.arg {
			if err := definedType(t); err != nil {
				return err
			}
		}
		for _, t := range typ.ret {
			if err := definedType(t); err != nil {
				return err
			}
		}
	case mapT:
		if err := definedType(typ.key); err != nil {
			return err
		}
		fallthrough
	case aliasT, arrayT, chanT, chanSendT, chanRecvT, ptrT, variadicT:
		if err := definedType(typ.val); err != nil {
			return err
		}
	case nilT:
		return typ.node.cfgErrorf("undefined: %s", typ.node.ident)
	}
	return nil
}

// equalNodes returns true if two slices of nodes are identical.
func equalNodes(a, b []*node) bool {
	if len(a) != len(b) {
		return false
	}
	for i, n := range a {
		if n != b[i] {
			return false
		}
	}
	return true
}
