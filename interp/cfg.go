package interp

import (
	"fmt"
	"reflect"
	"unicode"
)

// A CfgError represents an error during CFG build stage
type CfgError error

// Cfg generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables. A list of nodes of init functions is returned.
// Following this pass, the CFG is ready to run
func (interp *Interpreter) Cfg(root *Node) ([]*Node, error) {
	scope := interp.universe
	var loop, loopRestart *Node
	var initNodes []*Node
	var iotaValue int
	var pkgName string
	var err error

	if root.kind != File {
		// Set default package namespace for incremental parse
		pkgName = "_"
		if _, ok := interp.scope[pkgName]; !ok {
			interp.scope[pkgName] = scope.pushBloc()
		}
		scope = interp.scope[pkgName]
	}

	root.Walk(func(n *Node) bool {
		// Pre-order processing
		if err != nil {
			return false
		}
		switch n.kind {
		case AssignStmt, Define:
			if l := len(n.child); n.anc.kind == ConstDecl && l == 1 {
				// Implicit iota assignment. TODO: replicate previous explicit assignment
				n.child = append(n.child, &Node{anc: n, interp: interp, kind: Ident, ident: "iota"})
			}

		case BlockStmt:
			if n.anc != nil && n.anc.kind == RangeStmt {
				// For range block: ensure that array or map type is propagated to iterators
				// prior to process block. We cannot perform this at RangeStmt pre-order because
				// type of array like value is not yet known. This could be fixed in ast structure
				// by setting array/map node as 1st child of ForRangeStmt instead of 3rd child of
				// RangeStmt. The following workaround is less elegant but ok.
				if t := scope.rangeChanType(n.anc); t != nil {
					// range over channel
					e := n.anc.child[0]
					index := scope.add(t.val)
					scope.sym[e.ident] = &Symbol{index: index, kind: Var, typ: t.val}
					e.typ = t.val
					e.findex = index
					n.anc.gen = rangeChan
				} else {
					// range over array or map
					k, v := n.anc.child[0], n.anc.child[1]
					var ktyp, vtyp *Type

					switch n.anc.child[2].typ.cat {
					case ValueT:
						typ := n.anc.child[2].typ.rtype
						switch typ.Kind() {
						case reflect.Map:
							n.anc.gen = rangeMap
							ktyp = &Type{cat: ValueT, rtype: typ.Key()}
							vtyp = &Type{cat: ValueT, rtype: typ.Elem()}
						case reflect.Array, reflect.Slice:
							ktyp = scope.getType("int")
							vtyp = &Type{cat: ValueT, rtype: typ.Elem()}
						}
					case MapT:
						n.anc.gen = rangeMap
						ktyp = n.anc.child[2].typ.key
						vtyp = n.anc.child[2].typ.val
					case ArrayT:
						ktyp = scope.getType("int")
						vtyp = n.anc.child[2].typ.val
					}

					kindex := scope.add(ktyp)
					scope.sym[k.ident] = &Symbol{index: kindex, kind: Var, typ: ktyp}
					k.typ = ktyp
					k.findex = kindex

					vindex := scope.add(vtyp)
					scope.sym[v.ident] = &Symbol{index: vindex, kind: Var, typ: vtyp}
					v.typ = vtyp
					v.findex = vindex
				}
			}
			scope = scope.pushBloc()

		case Break, Continue, Goto:
			if len(n.child) > 0 {
				// Handle labeled statements
				label := n.child[0].ident
				if sym, _, ok := scope.lookup(label); ok {
					if sym.kind != Label {
						err = n.child[0].cfgError("label %s not defined", label)
						break
					}
					sym.from = append(sym.from, n)
					n.sym = sym
				} else {
					n.sym = &Symbol{kind: Label, from: []*Node{n}, index: -1}
					scope.sym[label] = n.sym
				}
			}

		case LabeledStmt:
			label := n.child[0].ident
			if sym, _, ok := scope.lookup(label); ok {
				if sym.kind != Label {
					err = n.child[0].cfgError("label %s not defined", label)
					break
				}
				sym.node = n
				n.sym = sym
			} else {
				n.sym = &Symbol{kind: Label, node: n, index: -1}
				scope.sym[label] = n.sym
			}

		case CaseClause:
			scope = scope.pushBloc()
			if sn := n.anc.anc; sn.kind == TypeSwitch && sn.child[1].action == Assign {
				// Type switch clause with a var defined in switch guard
				var typ *Type
				if len(n.child) == 2 {
					// 1 type in clause: define the var with this type in the case clause scope
					switch sym, _, ok := scope.lookup(n.child[0].ident); {
					case ok && sym.kind == Typ:
						typ = sym.typ
					case n.child[0].ident == "nil":
						typ = scope.getType("interface{}")
					default:
						err = n.cfgError("%s is not a type", n.child[0].ident)
						return false
					}
				} else {
					// define the var with the type in the switch guard expression
					typ = sn.child[1].child[1].child[0].typ
				}
				node := n.lastChild().child[0]
				index := scope.add(typ)
				scope.sym[node.ident] = &Symbol{index: index, kind: Var, typ: typ}
				node.findex = index
				node.typ = typ
			}

		case CommClause:
			scope = scope.pushBloc()
			if n.child[0].action == Assign {
				ch := n.child[0].child[1].child[0]
				if sym, _, ok := scope.lookup(ch.ident); ok {
					assigned := n.child[0].child[0]
					index := scope.add(sym.typ.val)
					scope.sym[assigned.ident] = &Symbol{index: index, kind: Var, typ: sym.typ.val}
					assigned.findex = index
					assigned.typ = sym.typ.val
				}
			}

		case CompositeLitExpr:
			if n.child[0].isType(scope) {
				// Get type from 1st child
				n.typ, err = nodeType(interp, scope, n.child[0])
			} else {
				// Get type from ancestor (implicit type)
				if n.anc.kind == KeyValueExpr && n == n.anc.child[0] {
					n.typ = n.anc.typ.key
				} else if n.anc.typ != nil {
					n.typ = n.anc.typ.val
				}
				n.typ.untyped = true
			}
			// Propagate type to children, to handle implicit types
			for _, c := range n.child {
				c.typ = n.typ
			}

		case File:
			pkgName = n.child[0].ident
			if _, ok := interp.scope[pkgName]; !ok {
				interp.scope[pkgName] = scope.pushBloc()
			}
			scope = interp.scope[pkgName]
			n.findex = -1

		case For0, ForRangeStmt:
			loop, loopRestart = n, n.child[0]
			scope = scope.pushBloc()

		case For1, For2, For3, For3a, For4:
			loop, loopRestart = n, n.lastChild()
			scope = scope.pushBloc()

		case FuncLit:
			n.typ = nil // to force nodeType to recompute the type
			n.typ, err = nodeType(interp, scope, n)
			n.findex = scope.add(n.typ)
			fallthrough

		case FuncDecl:
			n.val = n
			// Add a frame indirection level as we enter in a func
			scope = scope.pushFunc()
			scope.def = n
			if len(n.child[2].child) == 2 {
				// Allocate frame space for return values, define output symbols
				for _, c := range n.child[2].child[1].child {
					var typ *Type
					typ, err = nodeType(interp, scope, c.lastChild())
					if len(c.child) > 1 {
						for _, cc := range c.child[:len(c.child)-1] {
							scope.sym[cc.ident] = &Symbol{index: scope.add(typ), kind: Var, typ: typ}
						}
					} else {
						scope.add(typ)
					}
				}
			}
			if len(n.child[0].child) > 0 {
				// define receiver symbol
				var typ *Type
				recvName := n.child[0].child[0].child[0].ident
				typ, err = nodeType(interp, scope, n.child[0].child[0].lastChild())
				scope.sym[recvName] = &Symbol{index: scope.add(typ), kind: Var, typ: typ}
			}
			for _, c := range n.child[2].child[0].child {
				// define input parameter symbols
				var typ *Type
				typ, err = nodeType(interp, scope, c.lastChild())
				if typ.variadic {
					typ = &Type{cat: ArrayT, val: typ}
				}
				for _, cc := range c.child[:len(c.child)-1] {
					scope.sym[cc.ident] = &Symbol{index: scope.add(typ), kind: Var, typ: typ}
				}
			}
			if n.child[1].ident == "init" {
				initNodes = append(initNodes, n)
			}

		case If0, If1, If2, If3:
			scope = scope.pushBloc()

		case Switch, SwitchIf, TypeSwitch:
			// Make sure default clause is in last position
			c := n.lastChild().child
			if i, l := getDefault(n), len(c)-1; i >= 0 && i != l {
				c[i], c[l] = c[l], c[i]
			}
			scope = scope.pushBloc()
			loop = n

		case ImportSpec, TypeSpec:
			// processing already done in GTA pass
			return false

		case ArrayType, BasicLit, ChanType, FuncType, MapType, StructType:
			n.typ, err = nodeType(interp, scope, n)
			return false
		}
		return true
	}, func(n *Node) {
		// Post-order processing
		if err != nil {
			return
		}
		switch n.kind {
		case Address:
			wireChild(n)
			n.typ = &Type{cat: PtrT, val: n.child[0].typ}
			n.findex = scope.add(n.typ)

		case AssignStmt, Define:
			if n.anc.kind == TypeSwitch && n.anc.child[1] == n {
				// type switch guard assignment: assign dest to concrete value of src
				n.gen = nop
				break
			}
			if n.anc.kind == CommClause {
				n.gen = nop
				break
			}
			dest, src := n.child[0], n.lastChild()
			var sym *Symbol
			var level int
			if n.kind == Define {
				if src.typ.cat == NilT {
					err = src.cfgError("use of untyped nil")
					break
				}
				switch {
				case len(n.child) == 3:
					// type is provided in var declaration
					dest.typ, err = nodeType(interp, scope, n.child[1])
				case isRegularCall(src) || isBinCall(src):
					dest.typ = getReturnedType(src.child[0])
				default:
					dest.typ = src.typ
				}
				if scope.global {
					// Do not overload existings symbols (defined in GTA) in global scope
					sym, _, _ = scope.lookup(dest.ident)
				} else {
					sym = &Symbol{index: scope.add(dest.typ), kind: Var, global: scope.global}
					scope.sym[dest.ident] = sym
				}
				dest.val = src.val
				dest.recv = src.recv
				dest.findex = sym.index
				if src.kind == BasicLit {
					sym.val = src.val
				}
			} else {
				sym, level, _ = scope.lookup(dest.ident)
			}
			wireChild(n)
			switch t0, t1 := dest.typ, src.typ; n.action {
			case AddAssign:
				if !(isNumber(t0) && isNumber(t1) || isString(t0) && isString(t1)) || isInt(t0) && isFloat(t1) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			case SubAssign, MulAssign, QuoAssign:
				if !(isNumber(t0) && isNumber(t1)) || isInt(t0) && isFloat(t1) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			case RemAssign, AndAssign, OrAssign, XorAssign, AndNotAssign:
				if !(isInt(t0) && isInt(t1)) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			case ShlAssign, ShrAssign:
				if !(isInt(t0) && isUint(t1)) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			default:
				// Detect invalid float truncate
				if isInt(dest.typ) && isFloat(src.typ) {
					err = src.cfgError("invalid float truncate")
					return
				}
			}
			n.findex = dest.findex
			n.val = dest.val
			n.rval = dest.rval
			// Propagate type
			// TODO: Check that existing destination type matches source type
			switch {
			case src.action == Recv:
				// Assign by reading from a receiving channel
				n.gen = nop
				src.findex = dest.findex // Set recv address to LHS
				dest.typ = src.typ.val
			case src.action == CompositeLit:
				n.gen = nop
				src.findex = dest.findex
				src.level = level
			case src.kind == BasicLit:
				// TODO: perform constant folding and propagation here
				if dest.typ.cat == InterfaceT {
					src.val = reflect.ValueOf(src.val)
				} else {
					// Convert literal value to destination type
					src.val = reflect.ValueOf(src.val).Convert(dest.typ.TypeOf())
					src.typ = dest.typ
				}
			}
			n.typ = dest.typ
			if sym != nil {
				sym.typ = n.typ
				sym.recv = src.recv
			}
			n.level = level
			// If LHS is an indirection, get reference instead of value, to allow setting
			if dest.action == GetIndex {
				if dest.child[0].typ.cat == MapT {
					n.gen = assignMap
					dest.gen = nop // skip getIndexMap
				}
			}
			if n.anc.kind == ConstDecl {
				iotaValue++
			}

		case IncDecStmt:
			wireChild(n)
			n.findex = n.child[0].findex
			n.level = n.child[0].level
			n.typ = n.child[0].typ
			if sym, level, ok := scope.lookup(n.child[0].ident); ok {
				sym.typ = n.typ
				n.level = level
			}

		case AssignXStmt:
			wireChild(n)
			l := len(n.child) - 1
			switch n.child[l].kind {
			case IndexExpr:
				n.child[l].gen = getIndexMap2
				n.gen = nop
			case TypeAssertExpr:
				n.child[l].gen = typeAssert2
				n.gen = nop
			case UnaryExpr:
				if n.child[l].action == Recv {
					n.child[l].gen = recv2
					n.gen = nop
				}
			}

		case DefineX:
			wireChild(n)
			l := len(n.child) - 1
			var types []*Type
			switch n.child[l].kind {
			case CallExpr:
				if funtype := n.child[l].child[0].typ; funtype.cat == ValueT {
					// Handle functions imported from runtime
					for i := 0; i < funtype.rtype.NumOut(); i++ {
						types = append(types, &Type{cat: ValueT, rtype: funtype.rtype.Out(i)})
					}
				} else {
					types = funtype.ret
				}
				if l > len(types) {
					n.gen = assignX2
				}

			case IndexExpr:
				types = append(types, n.child[l].child[0].typ.val, scope.getType("bool"))
				n.child[l].gen = getIndexMap2
				n.gen = nop

			case TypeAssertExpr:
				types = append(types, n.child[l].child[1].typ, scope.getType("bool"))
				n.child[l].gen = typeAssert2
				n.gen = nop

			case UnaryExpr:
				if n.child[l].action == Recv {
					types = append(types, n.child[l].child[0].typ.val, scope.getType("bool"))
					n.child[l].gen = recv2
					n.gen = nop
				}

			default:
				err = n.cfgError("unsupported assign expression")
				return
			}
			for i, t := range types {
				index := scope.add(t)
				scope.sym[n.child[i].ident] = &Symbol{index: index, kind: Var, typ: t}
				n.child[i].typ = t
				n.child[i].findex = index
			}

		case BinaryExpr:
			wireChild(n)
			nilSym := interp.universe.sym["nil"]
			t0, t1 := n.child[0].typ, n.child[1].typ
			if !t0.untyped && !t1.untyped && t0.id() != t1.id() {
				err = n.cfgError("mismatched types %s and %s", t0.id(), t1.id())
				break
			}
			switch n.action {
			case Add:
				if !(isNumber(t0) && isNumber(t1) || isString(t0) && isString(t1)) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			case Sub, Mul, Quo:
				if !(isNumber(t0) && isNumber(t1)) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			case Rem, And, Or, Xor, AndNot:
				if !(isInt(t0) && isInt(t1)) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			case Shl, Shr:
				if !(isInt(t0) && isUint(t1)) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
			case Equal, NotEqual:
				if isNumber(t0) && !isNumber(t1) || isString(t0) && !isString(t1) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
				n.typ = scope.getType("bool")
				if n.child[0].sym == nilSym || n.child[1].sym == nilSym {
					if n.action == Equal {
						n.gen = isNil
					} else {
						n.gen = isNotNil
					}
				}
			case Greater, GreaterEqual, Lower, LowerEqual:
				if isNumber(t0) && !isNumber(t1) || isString(t0) && !isString(t1) {
					err = n.cfgError("illegal operand types for '%v' operator", n.action)
				}
				n.typ = scope.getType("bool")
			}
			// TODO: Possible optimisation: if type is bool and not in assignment or call, then skip result store
			if err == nil {
				if n.typ == nil {
					n.typ, err = nodeType(interp, scope, n)
				}
				n.findex = scope.add(n.typ)
			}

		case IndexExpr:
			wireChild(n)
			n.typ = n.child[0].typ.val
			n.findex = scope.add(n.typ)
			n.recv = &Receiver{node: n}
			if n.child[0].typ.cat == MapT {
				n.gen = getIndexMap
			} else if n.child[0].typ.cat == ArrayT {
				n.gen = getIndexArray
			}

		case BlockStmt:
			wireChild(n)
			if len(n.child) > 0 {
				n.findex = n.lastChild().findex
				n.val = n.lastChild().val
				n.sym = n.lastChild().sym
				n.typ = n.lastChild().typ
			}
			scope = scope.pop()

		case ConstDecl:
			iotaValue = 0
			wireChild(n)

		case VarDecl:
			wireChild(n)

		case DeclStmt, ExprStmt, SendStmt:
			wireChild(n)
			n.findex = n.lastChild().findex
			n.val = n.lastChild().val
			n.sym = n.lastChild().sym
			n.typ = n.lastChild().typ

		case Break:
			if len(n.child) > 0 {
				gotoLabel(n.sym)
			} else {
				n.tnext = loop
			}

		case Continue:
			if len(n.child) > 0 {
				gotoLabel(n.sym)
			} else {
				n.tnext = loopRestart
			}

		case Goto:
			gotoLabel(n.sym)

		case LabeledStmt:
			wireChild(n)
			n.start = n.child[1].start
			gotoLabel(n.sym)

		case CallExpr:
			wireChild(n)
			switch {
			case isBuiltinCall(n):
				n.gen = n.child[0].sym.builtin
				n.child[0].typ = &Type{cat: BuiltinT}
				switch n.child[0].ident {
				case "append":
					if n.typ = scope.getType(n.child[1].ident); n.typ == nil {
						n.typ, err = nodeType(interp, scope, n.child[1])
					}
				case "cap", "copy", "len":
					n.typ = scope.getType("int")
				case "make":
					if n.typ = scope.getType(n.child[1].ident); n.typ == nil {
						n.typ, err = nodeType(interp, scope, n.child[1])
					}
					n.child[1].val = n.typ
					n.child[1].kind = BasicLit
				case "new":
					n.typ, err = nodeType(interp, scope, n.child[1])
					n.typ = &Type{cat: PtrT, val: n.typ}
				case "recover":
					n.typ = scope.getType("interface{}")
				}
				if n.typ != nil {
					n.findex = scope.add(n.typ)
				}
			case n.child[0].isType(scope):
				// Type conversion expression
				if isInt(n.child[0].typ) && n.child[1].kind == BasicLit && isFloat(n.child[1].typ) {
					err = n.cfgError("truncated to integer")
				}
				n.gen = convert
				n.typ = n.child[0].typ
				n.findex = scope.add(n.typ)
			case isBinCall(n):
				n.gen = callBin
				if typ := n.child[0].typ.rtype; typ.NumOut() > 0 {
					n.typ = &Type{cat: ValueT, rtype: typ.Out(0)}
					n.findex = scope.add(n.typ)
					for i := 1; i < typ.NumOut(); i++ {
						scope.add(&Type{cat: ValueT, rtype: typ.Out(i)})
					}
				}
			default:
				if n.child[0].action == GetFunc {
					// allocate frame entry for anonymous function
					scope.add(n.child[0].typ)
				}
				if typ := n.child[0].typ; len(typ.ret) > 0 {
					n.typ = typ.ret[0]
					n.findex = scope.add(n.typ)
					for _, t := range typ.ret[1:] {
						scope.add(t)
					}
				} else {
					n.findex = -1
				}
			}

		case CaseBody:
			wireChild(n)
			if typeSwichAssign(n) && len(n.child) > 1 {
				n.start = n.child[1].start
			} else {
				n.start = n.child[0].start
			}

		case CaseClause:
			scope = scope.pop()

		case CommClause:
			wireChild(n)
			if len(n.child) > 1 {
				n.start = n.child[1].start // Skip chan operation, performed by select
			} else {
				n.start = n.child[0].start // default clause
			}
			n.lastChild().tnext = n.anc.anc // exit node is SelectStmt
			scope = scope.pop()

		case CompositeLitExpr:
			wireChild(n)
			if n.anc.action != Assign {
				n.findex = scope.add(n.typ)
			}
			// TODO: Check that composite literal expr matches corresponding type
			switch n.typ.cat {
			case ArrayT:
				n.gen = arrayLit
			case MapT:
				n.gen = mapLit
			case StructT:
				if n.lastChild().kind == KeyValueExpr {
					n.gen = compositeSparse
				} else {
					n.gen = compositeLit
				}
			case ValueT:
				n.gen = compositeBin
			}

		case Fallthrough:
			if n.anc.kind != CaseBody {
				err = n.cfgError("fallthrough statement out of place")
			}

		case File:
			wireChild(n)
			scope = scope.pop()

		case For0: // for {}
			body := n.child[0]
			n.start = body.start
			body.tnext = n.start
			loop, loopRestart = nil, nil
			scope = scope.pop()

		case For1: // for cond {}
			cond, body := n.child[0], n.child[1]
			n.start = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.pop()

		case For2: // for init; cond; {}
			init, cond, body := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.pop()

		case For3: // for ; cond; post {}
			cond, post, body := n.child[0], n.child[1], n.child[2]
			n.start = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = post.start
			post.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.pop()

		case For3a: // for int; ; post {}
			init, post, body := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			init.tnext = body.start
			body.tnext = post.start
			post.tnext = body.start
			loop, loopRestart = nil, nil
			scope = scope.pop()

		case For4: // for init; cond; post {}
			init, cond, post, body := n.child[0], n.child[1], n.child[2], n.child[3]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = post.start
			post.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.pop()

		case ForRangeStmt:
			loop, loopRestart = nil, nil
			n.start = n.child[0].start
			n.child[0].fnext = n
			scope = scope.pop()

		case FuncDecl:
			n.types = scope.types
			scope = scope.pop()
			funcName := n.child[1].ident
			n.start = n.child[3].start
			interp.scope[pkgName].sym[funcName].index = -1 // to force value to n.val
			interp.scope[pkgName].sym[funcName].typ = n.typ
			interp.scope[pkgName].sym[funcName].kind = Func
			interp.scope[pkgName].sym[funcName].node = n

		case FuncLit:
			n.types = scope.types
			scope = scope.pop()

		case GoStmt:
			wireChild(n)

		case Ident:
			if isKey(n) || isNewDefine(n) {
				break
			} else if sym, level, ok := scope.lookup(n.ident); ok {
				// Found symbol, populate node info
				n.typ, n.findex, n.level = sym.typ, sym.index, level
				if n.findex < 0 {
					n.val = sym.node
				} else {
					n.sym = sym
					switch {
					case sym.kind == Const && sym.val != nil:
						n.val = sym.val
						n.kind = BasicLit
					case n.ident == "iota":
						n.val = iotaValue
						n.kind = BasicLit
					case n.ident == "nil":
						n.kind = BasicLit
						n.val = nil
					case sym.kind == Bin:
						if sym.val == nil {
							n.kind = Rtype
						} else {
							n.kind = Rvalue
						}
						n.typ = sym.typ
						n.rval = sym.val.(reflect.Value)
					case sym.kind == Bltn:
						if n.anc.kind != CallExpr {
							err = n.cfgError("use of builtin %s not in function call", n.ident)
						}
					}
				}
				if n.sym != nil {
					n.recv = n.sym.recv
				}
			} else {
				err = n.cfgError("undefined: %s", n.ident)
			}

		case If0: // if cond {}
			cond, tbody := n.child[0], n.child[1]
			n.start = cond.start
			cond.tnext = tbody.start
			cond.fnext = n
			tbody.tnext = n
			scope = scope.pop()

		case If1: // if cond {} else {}
			cond, tbody, fbody := n.child[0], n.child[1], n.child[2]
			n.start = cond.start
			cond.tnext = tbody.start
			cond.fnext = fbody.start
			tbody.tnext = n
			fbody.tnext = n
			scope = scope.pop()

		case If2: // if init; cond {}
			init, cond, tbody := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			tbody.tnext = n
			init.tnext = cond.start
			cond.tnext = tbody.start
			cond.fnext = n
			scope = scope.pop()

		case If3: // if init; cond {} else {}
			init, cond, tbody, fbody := n.child[0], n.child[1], n.child[2], n.child[3]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = tbody.start
			cond.fnext = fbody.start
			tbody.tnext = n
			fbody.tnext = n
			scope = scope.pop()

		case KeyValueExpr:
			wireChild(n)

		case LandExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n.child[1].start
			n.child[0].fnext = n
			n.child[1].tnext = n
			n.typ = n.child[0].typ
			n.findex = scope.add(n.typ)

		case LorExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n
			n.child[0].fnext = n.child[1].start
			n.child[1].tnext = n
			n.typ = n.child[0].typ
			n.findex = scope.add(n.typ)

		case ParenExpr:
			wireChild(n)
			n.findex = n.lastChild().findex
			n.typ = n.lastChild().typ

		case RangeStmt:
			if scope.rangeChanType(n) != nil {
				n.start = n.child[1]       // Get chan
				n.child[1].tnext = n       // then go to range function
				n.tnext = n.child[2].start // then go to range body
				n.child[2].tnext = n       // then body go to range function (loop)
				n.child[0].gen = empty
			} else {
				n.start = n.child[2]                // Get array or map object
				n.child[2].tnext = n.child[0].start // then go to iterator init
				n.child[0].tnext = n                // then go to range function
				n.tnext = n.child[3].start          // then go to range body
				n.child[3].tnext = n                // then body go to range function (loop)
				n.child[0].gen = empty              // init filled later by generator
			}

		case ReturnStmt:
			wireChild(n)
			n.tnext = nil
			for i, c := range n.child {
				if c.typ.cat == NilT {
					// nil: Set node value to zero of return type
					f := scope.def
					var typ *Type
					typ, err = nodeType(interp, scope, f.child[2].child[1].child[i].lastChild())
					if err != nil {
						break
					}
					if c.val, err = typ.zero(); err != nil {
						break
					}
				}
			}

		case SelectorExpr:
			wireChild(n)
			n.typ = n.child[0].typ
			n.recv = n.child[0].recv
			if n.typ == nil {
				err = n.cfgError("undefined type")
				break
			}
			if n.typ.cat == ValueT {
				// Handle object defined in runtime, try to find field or method
				// Search for method first, as it applies both to types T and *T
				// Search for field must then be performed on type T only (not *T)
				switch method, ok := n.typ.rtype.MethodByName(n.child[1].ident); {
				case ok:
					n.val = method.Index
					n.gen = getIndexBinMethod
					n.typ = &Type{cat: ValueT, rtype: method.Type}
					n.recv = &Receiver{node: n.child[0]}
				case n.typ.rtype.Kind() == reflect.Ptr:
					if field, ok := n.typ.rtype.Elem().FieldByName(n.child[1].ident); ok {
						n.typ = &Type{cat: ValueT, rtype: field.Type}
						n.val = field.Index
						n.gen = getPtrIndexSeq
					} else {
						err = n.cfgError("undefined field or method: %s", n.child[1].ident)
					}
				case n.typ.rtype.Kind() == reflect.Struct:
					if field, ok := n.typ.rtype.FieldByName(n.child[1].ident); ok {
						n.typ = &Type{cat: ValueT, rtype: field.Type}
						n.val = field.Index
						n.gen = getIndexSeq
					} else {
						err = n.cfgError("undefined field or method: %s", n.child[1].ident)
					}
				default:
					err = n.cfgError("undefined field or method: %s", n.child[1].ident)
				}
			} else if n.typ.cat == PtrT && n.typ.val.cat == ValueT {
				// Handle pointer on object defined in runtime
				if field, ok := n.typ.val.rtype.FieldByName(n.child[1].ident); ok {
					n.typ = &Type{cat: ValueT, rtype: field.Type}
					n.val = field.Index
					n.gen = getPtrIndexSeq
				} else if method, ok := n.typ.val.rtype.MethodByName(n.child[1].ident); ok {
					n.val = method.Index
					n.typ = &Type{cat: ValueT, rtype: method.Type}
					n.recv = &Receiver{node: n.child[0]}
					n.gen = getIndexBinMethod
				} else if method, ok := reflect.PtrTo(n.typ.val.rtype).MethodByName(n.child[1].ident); ok {
					n.val = method.Index
					n.gen = getIndexBinMethod
					n.typ = &Type{cat: ValueT, rtype: method.Type}
					n.recv = &Receiver{node: n.child[0]}
				} else {
					err = n.cfgError("undefined selector: %s", n.child[1].ident)
				}
			} else if n.typ.cat == BinPkgT {
				// Resolve binary package symbol: a type or a value
				name := n.child[1].ident
				pkg := n.child[0].sym.path
				if s, ok := interp.binValue[pkg][name]; ok {
					if isBinType(s) {
						n.kind = Rtype
						n.typ = &Type{cat: ValueT, rtype: s.Type().Elem()}
					} else {
						n.kind = Rvalue
						n.typ = &Type{cat: ValueT, rtype: s.Type()}
						n.rval = s
					}
					n.gen = nop
				} else {
					err = n.cfgError("package %s \"%s\" has no symbol %s", n.child[0].ident, pkg, name)
				}
			} else if n.typ.cat == SrcPkgT {
				pkg, name := n.child[0].ident, n.child[1].ident
				// Resolve source package symbol
				if sym, ok := interp.scope[pkg].sym[name]; ok {
					n.findex = sym.index
					n.val = sym.node
					n.gen = nop
					n.typ = sym.typ
					n.sym = sym
				} else {
					err = n.cfgError("undefined selector: %s", n.child[1].ident)
				}
			} else if m, lind := n.typ.lookupMethod(n.child[1].ident); m != nil {
				// Handle method
				n.gen = getMethod
				n.val = m
				n.typ = m.typ
				n.recv = &Receiver{node: n.child[0], index: lind}
			} else if ti := n.typ.lookupField(n.child[1].ident); len(ti) > 0 {
				// Handle struct field
				n.val = ti
				switch n.typ.cat {
				case InterfaceT:
					n.typ = n.typ.fieldSeq(ti)
					n.gen = getMethodByName
					n.action = Method
				case PtrT:
					n.typ = n.typ.fieldSeq(ti)
					n.gen = getPtrIndexSeq
				default:
					n.gen = getIndexSeq
					n.typ = n.typ.fieldSeq(ti)
					if n.typ.cat == FuncT {
						// function in a struct field is always wrapped in reflect.Value
						rtype := n.typ.TypeOf()
						n.typ = &Type{cat: ValueT, rtype: rtype}
					}
				}
			} else {
				err = n.cfgError("undefined selector: %s", n.child[1].ident)
			}
			if n.findex != -1 {
				n.findex = scope.add(n.typ)
			}

		case SelectStmt:
			wireChild(n)
			// Move action to block statement, so select node can be an exit point
			n.child[0].gen = _select
			n.start = n.child[0]

		case StarExpr:
			switch {
			case n.anc.kind == Define && len(n.anc.child) == 3 && n.anc.child[1] == n:
				// pointer type expression in a var definition
				n.gen = nop
			case n.anc.kind == ValueSpec && n.anc.lastChild() == n:
				// pointer type expression in a value spec
				n.gen = nop
			case n.anc.kind == Field:
				// pointer type expression in a field expression (arg or struct field)
				n.gen = nop
			default:
				// dereference expression
				wireChild(n)
				n.typ = n.child[0].typ.val
				n.findex = scope.add(n.typ)
			}

		case TypeSwitch:
			// Check that cases expressions are all different
			usedCase := map[string]bool{}
			for _, c := range n.lastChild().child {
				for _, t := range c.child[:len(c.child)-1] {
					tid := t.typ.id()
					if usedCase[tid] {
						err = c.cfgError("duplicate case %s in type switch", t.ident)
						return
					}
					usedCase[tid] = true
				}
			}
			fallthrough

		case Switch:
			sbn := n.lastChild() // switch block node
			clauses := sbn.child
			l := len(clauses)
			// Chain case clauses
			for i, c := range clauses[:l-1] {
				c.fnext = clauses[i+1] // chain to next clause
				body := c.lastChild()
				c.tnext = body.start
				if len(body.child) > 0 && body.lastChild().kind == Fallthrough {
					if n.kind == TypeSwitch {
						err = body.lastChild().cfgError("cannot fallthrough in type switch")
					}
					body.tnext = clauses[i+1].lastChild().start
				} else {
					body.tnext = n
				}
			}
			c := clauses[l-1]
			c.tnext = c.lastChild().start
			if n.child[0].action == Assign &&
				(n.child[0].child[0].kind != TypeAssertExpr || len(n.child[0].child[0].child) > 1) {
				// switch init statement is defined
				n.start = n.child[0].start
				n.child[0].tnext = sbn.start
			} else {
				n.start = sbn.start
			}
			scope = scope.pop()
			loop = nil

		case SwitchIf: // like an if-else chain
			sbn := n.lastChild() // switch block node
			clauses := sbn.child
			l := len(clauses)
			// Wire case clauses in reverse order so the next start node is already resolved when used.
			for i := l - 1; i >= 0; i-- {
				c := clauses[i]
				c.gen = nop
				body := c.lastChild()
				if len(c.child) > 1 {
					cond := c.child[0]
					cond.tnext = body.start
					if i == l-1 {
						cond.fnext = n
					} else {
						cond.fnext = clauses[i+1].start
					}
					c.start = cond.start
				} else {
					c.start = body.start
				}
				// If last case body statement is a fallthrough, then jump to next case body
				if i < l-1 && len(body.child) > 0 && body.lastChild().kind == Fallthrough {
					body.tnext = clauses[i+1].lastChild().start
				}
			}
			sbn.start = clauses[0].start
			if n.child[0].action == Assign {
				// switch init statement is defined
				n.start = n.child[0].start
				n.child[0].tnext = sbn.start
			} else {
				n.start = sbn.start
			}
			scope = scope.pop()
			loop = nil

		case TypeAssertExpr:
			if len(n.child) > 1 {
				if n.child[1].typ == nil {
					n.child[1].typ = scope.getType(n.child[1].ident)
				}
				if n.anc.action != AssignX {
					n.typ = n.child[1].typ
					n.findex = scope.add(n.typ)
				}
			} else {
				n.gen = nop
			}

		case SliceExpr:
			wireChild(n)
			if ctyp := n.child[0].typ; ctyp.size != 0 {
				// Create a slice type from an array type
				n.typ = &Type{}
				*n.typ = *ctyp
				n.typ.size = 0
				n.typ.rtype = nil
			} else {
				n.typ = ctyp
			}
			n.findex = scope.add(n.typ)

		case UnaryExpr:
			wireChild(n)
			n.typ = n.child[0].typ
			// TODO: Optimisation: avoid allocation if boolean branch op (i.e. '!' in an 'if' expr)
			n.findex = scope.add(n.typ)

		case ValueSpec:
			l := len(n.child) - 1
			if n.typ = n.child[l].typ; n.typ == nil {
				n.typ, err = nodeType(interp, scope, n.child[l])
				if err != nil {
					return
				}
			}
			for _, c := range n.child[:l] {
				index := scope.add(n.typ)
				scope.sym[c.ident] = &Symbol{index: index, kind: Var, typ: n.typ}
				c.typ = n.typ
				c.findex = index
			}
		}
	})

	if scope != interp.universe {
		scope.pop()
	}
	return initNodes, err
}

func (n *Node) cfgError(format string, a ...interface{}) CfgError {
	a = append([]interface{}{n.fset.Position(n.pos)}, a...)
	return CfgError(fmt.Errorf("%s: "+format, a...))
}

func genRun(node *Node) error {
	var err CfgError

	node.Walk(func(n *Node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case FuncType:
			if len(n.anc.child) == 4 {
				// function body entry point
				setExec(n.anc.child[3].start)
			}
			// continue in function body as there may be inner function definitions
		case ConstDecl, VarDecl:
			setExec(n.start)
			return false
		}
		return true
	}, nil)

	return err
}

// Find default case clause index of a switch statement, if any
func getDefault(n *Node) int {
	for i, c := range n.lastChild().child {
		if len(c.child) == 1 {
			return i
		}
	}
	return -1
}

func isBinType(v reflect.Value) bool { return v.IsValid() && v.Kind() == reflect.Ptr && v.IsNil() }

// isType returns true if node refers to a type definition, false otherwise
func (n *Node) isType(scope *Scope) bool {
	switch n.kind {
	case ArrayType, ChanType, FuncType, MapType, StructType, Rtype:
		return true
	case SelectorExpr:
		pkg, name := n.child[0].ident, n.child[1].ident
		if sym, _, ok := scope.lookup(pkg); ok {
			if p, ok := n.interp.binValue[sym.path]; ok && isBinType(p[name]) {
				return true // Imported binary type
			}
			if p, ok := n.interp.scope[pkg]; ok && p.sym[name] != nil && p.sym[name].kind == Typ {
				return true // Imported source type
			}
		}
	case Ident:
		return scope.getType(n.ident) != nil
	}
	return false
}

// wireChild wires AST nodes for CFG in subtree
func wireChild(n *Node) {
	// Set start node, in subtree (propagated to ancestors by post-order processing)
	for _, child := range n.child {
		switch child.kind {
		case ArrayType, ChanType, FuncDecl, ImportDecl, MapType, BasicLit, Ident, TypeDecl:
			continue
		default:
			n.start = child.start
		}
		break
	}

	// Chain sequential operations inside a block (next is right sibling)
	for i := 1; i < len(n.child); i++ {
		switch n.child[i].kind {
		case FuncDecl:
			n.child[i-1].tnext = n.child[i]
		default:
			switch n.child[i-1].kind {
			case Break, Continue, Goto, ReturnStmt:
				// tnext is already computed, no change
			default:
				n.child[i-1].tnext = n.child[i].start
			}
		}
	}

	// Chain subtree next to self
	for i := len(n.child) - 1; i >= 0; i-- {
		switch n.child[i].kind {
		case ArrayType, ChanType, ImportDecl, MapType, FuncDecl, BasicLit, Ident, TypeDecl:
			continue
		case Break, Continue, Goto, ReturnStmt:
			// tnext is already computed, no change
		default:
			n.child[i].tnext = n
		}
		break
	}
}

// last returns the last child of a node
func (n *Node) lastChild() *Node { return n.child[len(n.child)-1] }

func isKey(n *Node) bool {
	return n.anc.kind == File ||
		(n.anc.kind == SelectorExpr && n.anc.child[0] != n) ||
		(n.anc.kind == KeyValueExpr && n.anc.child[0] == n)
}

func isNewDefine(n *Node) bool {
	if n.ident == "_" {
		return true
	}
	if n.anc.kind == Define && n.anc.child[0] == n {
		return true
	}
	if n.anc.kind == DefineX && n.anc.lastChild() != n {
		return true
	}
	if n.anc.kind == RangeStmt && (n.anc.child[0] == n || n.anc.child[1] == n) {
		return true
	}
	if n.anc.kind == ValueSpec && n.anc.lastChild() != n {
		return true
	}
	return false
}

func isBuiltinCall(n *Node) bool {
	return n.kind == CallExpr && n.child[0].sym != nil && n.child[0].sym.kind == Bltn
}

func isBinCall(n *Node) bool {
	return n.kind == CallExpr && n.child[0].typ.cat == ValueT && n.child[0].typ.rtype.Kind() == reflect.Func
}

func isRegularCall(n *Node) bool {
	return n.kind == CallExpr && n.child[0].typ.cat == FuncT
}

func variadicPos(n *Node) int {
	if len(n.child[0].typ.arg) == 0 {
		return -1
	}
	last := len(n.child[0].typ.arg) - 1
	if n.child[0].typ.arg[last].variadic {
		return last
	}
	return -1
}

func canExport(name string) bool {
	if r := []rune(name); len(r) > 0 && unicode.IsUpper(r[0]) {
		return true
	}
	return false
}

func getExec(n *Node) Builtin {
	if n == nil {
		return nil
	}
	if n.exec == nil {
		setExec(n)
	}
	return n.exec
}

// setExec recursively sets the node exec builtin function by walking the CFG
// from the entry point (first node to exec).
func setExec(n *Node) {
	if n.exec != nil {
		return
	}
	seen := map[*Node]bool{}
	var set func(n *Node)

	set = func(n *Node) {
		if n == nil || n.exec != nil {
			return
		}
		seen[n] = true
		if n.tnext != nil && n.tnext.exec == nil {
			if seen[n.tnext] {
				m := n.tnext
				n.tnext.exec = func(f *Frame) Builtin { return m.exec(f) }
			} else {
				set(n.tnext)
			}
		}
		if n.fnext != nil && n.fnext.exec == nil {
			if seen[n.fnext] {
				m := n.fnext
				n.fnext.exec = func(f *Frame) Builtin { return m.exec(f) }
			} else {
				set(n.fnext)
			}
		}
		n.gen(n)
	}

	set(n)
}

func getReturnedType(n *Node) *Type {
	switch n.typ.cat {
	case BuiltinT:
		return n.anc.typ
	case ValueT:
		if n.typ.rtype.NumOut() > 0 {
			return &Type{cat: ValueT, rtype: n.typ.rtype.Out(0)}
		}
		return &Type{cat: ValueT, rtype: n.typ.rtype}
	}
	return n.typ.ret[0]
}

func typeSwichAssign(n *Node) bool {
	ts := n.anc.anc.anc
	return ts.kind == TypeSwitch && ts.child[1].action == Assign
}

func gotoLabel(s *Symbol) {
	if s.node == nil {
		return
	}
	for _, c := range s.from {
		c.tnext = s.node.start
	}
}
