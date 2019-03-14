package interp

import (
	"path"
)

// Gta performs a global types analysis on the AST, registering types,
// variables and functions symbols at package level, prior to CFG.
// All function bodies are skipped. GTA is necessary to handle out of
// order declarations and multiple source files packages.
func (interp *Interpreter) Gta(root *Node, rpath string) error {
	scope := interp.universe
	var err error
	var pkgName string
	var iotaValue int

	if root.kind != File {
		// Set default package namespace for incremental parse
		pkgName = "_"
		if _, ok := interp.scope[pkgName]; !ok {
			interp.scope[pkgName] = scope.pushBloc()
		}
		scope = interp.scope[pkgName]
	}

	root.Walk(func(n *Node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case ConstDecl:
			iotaValue = 0

		case Define:
			var typ *Type
			if len(n.child) > 1 {
				typ, err = nodeType(interp, scope, n.child[1])
			} else {
				typ, err = nodeType(interp, scope, n.anc.child[0].child[1])
			}
			if err != nil {
				return false
			}
			if typ.cat == NilT {
				err = n.cfgError("use of untyped nil")
				return false
			}
			var val interface{} = iotaValue
			if len(n.child) > 1 {
				val = n.child[1].val
			}
			scope.sym[n.child[0].ident] = &Symbol{kind: Var, global: true, index: scope.add(typ), typ: typ, val: val}
			if n.anc.kind == ConstDecl {
				iotaValue++
			}
			return false

		// TODO: add DefineX, ValueSpec

		case File:
			pkgName = n.child[0].ident
			if _, ok := interp.scope[pkgName]; !ok {
				interp.scope[pkgName] = scope.pushBloc()
			}
			scope = interp.scope[pkgName]

		case FuncDecl:
			if n.typ, err = nodeType(interp, scope, n.child[2]); err != nil {
				return false
			}
			scope.sym[n.child[1].ident] = &Symbol{kind: Func, typ: n.typ, node: n, index: -1}
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
						if isBinType(v) {
							scope.sym[n] = &Symbol{kind: Bin, typ: &Type{cat: ValueT, rtype: v.Type().Elem()}, val: v}
						} else {
							scope.sym[n] = &Symbol{kind: Bin, typ: &Type{cat: ValueT, rtype: v.Type()}, val: v}
						}
					}
					//for n, t := range interp.binType[ipath] {
					//	scope.sym[n] = &Symbol{kind: Bin, typ: &Type{cat: ValueT, rtype: t}}
					//}
				} else {
					scope.sym[name] = &Symbol{typ: &Type{cat: BinPkgT}, path: ipath}
				}
			} else {
				// TODO: make sure we do not import a src package more than once
				err = interp.importSrcFile(rpath, ipath, name)
				scope.types = interp.universe.types
				scope.sym[name] = &Symbol{typ: &Type{cat: SrcPkgT}, path: ipath}
			}

		case TypeSpec:
			typeName := n.child[0].ident
			if n.child[1].kind == Ident {
				var typ *Type
				typ, err = nodeType(interp, scope, n.child[1])
				n.typ = &Type{cat: AliasT, val: typ, name: typeName, pkgPath: rpath}
			} else {
				n.typ, err = nodeType(interp, scope, n.child[1])
				n.typ.name = typeName
				n.typ.pkgPath = rpath
			}
			if err != nil {
				return false
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
