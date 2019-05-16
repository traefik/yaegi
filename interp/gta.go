package interp

import (
	"path"
)

// Gta performs a global types analysis on the AST, registering types,
// variables and functions symbols at package level, prior to CFG.
// All function bodies are skipped. GTA is necessary to handle out of
// order declarations and multiple source files packages.
func (interp *Interpreter) Gta(root *Node, rpath string) error {
	scope, _ := interp.initScopePkg(root)
	var err error
	var iotaValue int

	root.Walk(func(n *Node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case ConstDecl:
			iotaValue = 0

		case BlockStmt:
			if n != root {
				return false // skip statement block if not the entry point
			}

		case Define:
			var atyp *Type
			if n.nleft+n.nright < len(n.child) {
				atyp, err = nodeType(interp, scope, n.child[n.nleft])
				if err != nil {
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
				var val interface{} = iotaValue
				if typ == nil {
					typ, err = nodeType(interp, scope, src)
					if err != nil {
						return false
					}
					val = src.val
				}
				if typ.cat == NilT {
					err = n.cfgError("use of untyped nil")
					return false
				}
				scope.sym[dest.ident] = &Symbol{kind: Var, global: true, index: scope.add(typ), typ: typ, val: val}
				if n.anc.kind == ConstDecl {
					iotaValue++
				}
			}
			return false

		case DefineX:
			// TODO: handle global DefineX
			err = n.cfgError("global DefineX not implemented")

		case ValueSpec:
			// TODO: handle global ValueSpec
			err = n.cfgError("global ValueSpec not implemented")

		case FuncDecl:
			if n.typ, err = nodeType(interp, scope, n.child[2]); err != nil {
				return false
			}
			if !isMethod(n) {
				scope.sym[n.child[1].ident] = &Symbol{kind: Func, typ: n.typ, node: n, index: -1}
			}
			if len(n.child[0].child) > 0 {
				// function is a method, add it to the related type
				var receiverType *Type
				var typeName string
				n.ident = n.child[1].ident
				receiver := n.child[0].child[0]
				if len(receiver.child) < 2 {
					// Receiver var name is skipped in method declaration (fix that in AST ?)
					typeName = receiver.child[0].ident
				} else {
					typeName = receiver.child[1].ident
				}
				if typeName == "" {
					// The receiver is a pointer, retrieve typeName from indirection
					typeName = receiver.child[1].child[0].ident
					elementType := scope.getType(typeName)
					if elementType == nil {
						// Add type if necessary, so method can be registered
						scope.sym[typeName] = &Symbol{kind: Typ, typ: &Type{name: typeName, pkgPath: rpath}}
						elementType = scope.sym[typeName].typ
					}
					receiverType = &Type{cat: PtrT, val: elementType}
					elementType.method = append(elementType.method, n)
				} else {
					receiverType = scope.getType(typeName)
					if receiverType == nil {
						// Add type if necessary, so method can be registered
						scope.sym[typeName] = &Symbol{kind: Typ, typ: &Type{name: typeName, pkgPath: rpath}}
						receiverType = scope.sym[typeName].typ
					}
				}
				receiverType.method = append(receiverType.method, n)
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
			if interp.binValue[ipath] != nil {
				if name == "." {
					for n, v := range interp.binValue[ipath] {
						typ := v.Type()
						if isBinType(v) {
							typ = typ.Elem()
						}
						scope.sym[n] = &Symbol{kind: Bin, typ: &Type{cat: ValueT, rtype: typ}, val: v}
					}
				} else {
					scope.sym[name] = &Symbol{kind: Package, typ: &Type{cat: BinPkgT}, path: ipath}
				}
			} else {
				// TODO: make sure we do not import a src package more than once
				err = interp.importSrcFile(rpath, ipath, name)
				scope.types = interp.universe.types
				scope.sym[name] = &Symbol{kind: Package, typ: &Type{cat: SrcPkgT}, path: ipath}
			}

		case TypeSpec:
			typeName := n.child[0].ident
			var typ *Type
			typ, err = nodeType(interp, scope, n.child[1])
			if err != nil {
				return false
			}
			if n.child[1].kind == Ident {
				n.typ = &Type{cat: AliasT, val: typ, name: typeName, pkgPath: rpath}
			} else {
				n.typ = typ
				n.typ.name = typeName
				n.typ.pkgPath = rpath
			}
			// Type may already be declared for a receiver in a method function
			if scope.sym[typeName] == nil {
				scope.sym[typeName] = &Symbol{kind: Typ}
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
