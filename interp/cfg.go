package interp

import (
	"fmt"
	"log"
	"path"
	"reflect"
	"unicode"
)

// A cfgError represents an error during CFG build stage
type cfgError error

var constOp = map[action]func(*node){
	aAdd:    addConst,
	aSub:    subConst,
	aMul:    mulConst,
	aQuo:    quoConst,
	aRem:    remConst,
	aAnd:    andConst,
	aOr:     orConst,
	aShl:    shlConst,
	aShr:    shrConst,
	aAndNot: andNotConst,
	aXor:    xorConst,
}

// cfg generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables. A list of nodes of init functions is returned.
// Following this pass, the CFG is ready to run
func (interp *Interpreter) cfg(root *node) ([]*node, error) {
	sc, pkgName := interp.initScopePkg(root)
	var loop, loopRestart *node
	var initNodes []*node
	var iotaValue int
	var err error

	root.Walk(func(n *node) bool {
		// Pre-order processing
		if err != nil {
			return false
		}
		switch n.kind {
		case blockStmt:
			if n.anc != nil && n.anc.kind == rangeStmt {
				// For range block: ensure that array or map type is propagated to iterators
				// prior to process block. We cannot perform this at RangeStmt pre-order because
				// type of array like value is not yet known. This could be fixed in ast structure
				// by setting array/map node as 1st child of ForRangeStmt instead of 3rd child of
				// RangeStmt. The following workaround is less elegant but ok.
				if t := sc.rangeChanType(n.anc); t != nil {
					// range over channel
					e := n.anc.child[0]
					index := sc.add(t.val)
					sc.sym[e.ident] = &symbol{index: index, kind: varSym, typ: t.val}
					e.typ = t.val
					e.findex = index
					n.anc.gen = rangeChan
				} else {
					// range over array or map
					var ktyp, vtyp *itype
					var k, v, o *node
					if len(n.anc.child) == 4 {
						k, v, o = n.anc.child[0], n.anc.child[1], n.anc.child[2]
					} else {
						k, o = n.anc.child[0], n.anc.child[1]
					}

					switch o.typ.cat {
					case valueT:
						typ := o.typ.rtype
						switch typ.Kind() {
						case reflect.Map:
							n.anc.gen = rangeMap
							ktyp = &itype{cat: valueT, rtype: typ.Key()}
							vtyp = &itype{cat: valueT, rtype: typ.Elem()}
						case reflect.String:
							ktyp = sc.getType("int")
							vtyp = sc.getType("byte")
						case reflect.Array, reflect.Slice:
							ktyp = sc.getType("int")
							vtyp = &itype{cat: valueT, rtype: typ.Elem()}
						}
					case mapT:
						n.anc.gen = rangeMap
						ktyp = o.typ.key
						vtyp = o.typ.val
					case stringT:
						ktyp = sc.getType("int")
						vtyp = sc.getType("byte")
					case arrayT:
						ktyp = sc.getType("int")
						vtyp = o.typ.val
					}

					kindex := sc.add(ktyp)
					sc.sym[k.ident] = &symbol{index: kindex, kind: varSym, typ: ktyp}
					k.typ = ktyp
					k.findex = kindex

					if v != nil {
						vindex := sc.add(vtyp)
						sc.sym[v.ident] = &symbol{index: vindex, kind: varSym, typ: vtyp}
						v.typ = vtyp
						v.findex = vindex
					}
				}
			}
			n.findex = -1
			n.val = nil
			sc = sc.pushBloc()

		case breakStmt, continueStmt, gotoStmt:
			if len(n.child) > 0 {
				// Handle labeled statements
				label := n.child[0].ident
				if sym, _, ok := sc.lookup(label); ok {
					if sym.kind != labelSym {
						err = n.child[0].cfgErrorf("label %s not defined", label)
						break
					}
					sym.from = append(sym.from, n)
					n.sym = sym
				} else {
					n.sym = &symbol{kind: labelSym, from: []*node{n}, index: -1}
					sc.sym[label] = n.sym
				}
			}

		case labeledStmt:
			label := n.child[0].ident
			if sym, _, ok := sc.lookup(label); ok {
				if sym.kind != labelSym {
					err = n.child[0].cfgErrorf("label %s not defined", label)
					break
				}
				sym.node = n
				n.sym = sym
			} else {
				n.sym = &symbol{kind: labelSym, node: n, index: -1}
				sc.sym[label] = n.sym
			}

		case caseClause:
			sc = sc.pushBloc()
			if sn := n.anc.anc; sn.kind == typeSwitch && sn.child[1].action == aAssign {
				// Type switch clause with a var defined in switch guard
				var typ *itype
				if len(n.child) == 2 {
					// 1 type in clause: define the var with this type in the case clause scope
					switch sym, _, ok := sc.lookup(n.child[0].ident); {
					case ok && sym.kind == typeSym:
						typ = sym.typ
					case n.child[0].ident == "nil":
						typ = sc.getType("interface{}")
					default:
						err = n.cfgErrorf("%s is not a type", n.child[0].ident)
						return false
					}
				} else {
					// define the var with the type in the switch guard expression
					typ = sn.child[1].child[1].child[0].typ
				}
				nod := n.lastChild().child[0]
				index := sc.add(typ)
				sc.sym[nod.ident] = &symbol{index: index, kind: varSym, typ: typ}
				nod.findex = index
				nod.typ = typ
			}

		case commClause:
			sc = sc.pushBloc()
			if n.child[0].action == aAssign {
				ch := n.child[0].child[1].child[0]
				if sym, _, ok := sc.lookup(ch.ident); ok {
					assigned := n.child[0].child[0]
					index := sc.add(sym.typ.val)
					sc.sym[assigned.ident] = &symbol{index: index, kind: varSym, typ: sym.typ.val}
					assigned.findex = index
					assigned.typ = sym.typ.val
				}
			}

		case compositeLitExpr:
			if n.child[0].isType(sc) {
				// Get type from 1st child
				if n.typ, err = nodeType(interp, sc, n.child[0]); err != nil {
					return false
				}
			} else {
				// Get type from ancestor (implicit type)
				if n.anc.kind == keyValueExpr && n == n.anc.child[0] {
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

		case forStmt0, forRangeStmt:
			loop, loopRestart = n, n.child[0]
			sc = sc.pushBloc()

		case forStmt1, forStmt2, forStmt3, forStmt3a, forStmt4:
			loop, loopRestart = n, n.lastChild()
			sc = sc.pushBloc()

		case funcLit:
			n.typ = nil // to force nodeType to recompute the type
			if n.typ, err = nodeType(interp, sc, n); err != nil {
				return false
			}
			n.findex = sc.add(n.typ)
			fallthrough

		case funcDecl:
			n.val = n
			// Add a frame indirection level as we enter in a func
			sc = sc.pushFunc()
			sc.def = n
			if len(n.child[2].child) == 2 {
				// Allocate frame space for return values, define output symbols
				for _, c := range n.child[2].child[1].child {
					var typ *itype
					if typ, err = nodeType(interp, sc, c.lastChild()); err != nil {
						return false
					}
					if len(c.child) > 1 {
						for _, cc := range c.child[:len(c.child)-1] {
							sc.sym[cc.ident] = &symbol{index: sc.add(typ), kind: varSym, typ: typ}
						}
					} else {
						sc.add(typ)
					}
				}
			}
			if len(n.child[0].child) > 0 {
				// define receiver symbol
				var typ *itype
				recvName := n.child[0].child[0].child[0].ident
				recvTypeNode := n.child[0].child[0].lastChild()
				if typ, err = nodeType(interp, sc, recvTypeNode); err != nil {
					return false
				}
				recvTypeNode.typ = typ
				sc.sym[recvName] = &symbol{index: sc.add(typ), kind: varSym, typ: typ}
			}
			for _, c := range n.child[2].child[0].child {
				// define input parameter symbols
				var typ *itype
				if typ, err = nodeType(interp, sc, c.lastChild()); err != nil {
					return false
				}
				if typ.variadic {
					typ = &itype{cat: arrayT, val: typ}
				}
				for _, cc := range c.child[:len(c.child)-1] {
					sc.sym[cc.ident] = &symbol{index: sc.add(typ), kind: varSym, typ: typ}
				}
			}
			if n.child[1].ident == "init" && len(n.child[0].child) == 0 {
				initNodes = append(initNodes, n)
			}

		case ifStmt0, ifStmt1, ifStmt2, ifStmt3:
			sc = sc.pushBloc()

		case switchStmt, switchIfStmt, typeSwitch:
			// Make sure default clause is in last position
			c := n.lastChild().child
			if i, l := getDefault(n), len(c)-1; i >= 0 && i != l {
				c[i], c[l] = c[l], c[i]
			}
			sc = sc.pushBloc()
			loop = n

		case importSpec:
			var name, ipath string
			if len(n.child) == 2 {
				ipath = n.child[1].rval.String()
				name = n.child[0].ident
			} else {
				ipath = n.child[0].rval.String()
				name = path.Base(ipath)
			}
			if interp.binValue[ipath] != nil && name != "." {
				sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT}, path: ipath}
			} else {
				sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: srcPkgT}, path: ipath}
			}
			return false

		case typeSpec:
			// processing already done in GTA pass
			return false

		case arrayType, basicLit, chanType, funcType, mapType, structType:
			n.typ, err = nodeType(interp, sc, n)
			return false
		}
		return true
	}, func(n *node) {
		// Post-order processing
		if err != nil {
			return
		}
		switch n.kind {
		case addressExpr:
			wireChild(n)
			n.typ = &itype{cat: ptrT, val: n.child[0].typ}
			n.findex = sc.add(n.typ)

		case assignStmt, defineStmt:
			if n.anc.kind == typeSwitch && n.anc.child[1] == n {
				// type switch guard assignment: assign dest to concrete value of src
				n.gen = nop
				break
			}
			if n.anc.kind == commClause {
				n.gen = nop
				break
			}
			var atyp *itype
			if n.nleft+n.nright < len(n.child) {
				if atyp, err = nodeType(interp, sc, n.child[n.nleft]); err != nil {
					return
				}
			}

			var sbase int
			if n.nright > 0 {
				sbase = len(n.child) - n.nright
			}

			wireChild(n)
			for i := 0; i < n.nleft; i++ {
				dest, src := n.child[i], n.child[sbase+i]
				var sym *symbol
				var level int
				if n.kind == defineStmt {
					if src.typ != nil && src.typ.cat == nilT {
						err = src.cfgErrorf("use of untyped nil")
						break
					}
					if atyp != nil {
						dest.typ = atyp
					} else {
						dest.typ = src.typ
					}
					if sc.global {
						// Do not overload existings symbols (defined in GTA) in global scope
						sym, _, _ = sc.lookup(dest.ident)
					} else {
						sym = &symbol{index: sc.add(dest.typ), kind: varSym, typ: dest.typ}
						sc.sym[dest.ident] = sym
					}
					dest.val = src.val
					dest.recv = src.recv
					dest.findex = sym.index
					if src.kind == basicLit {
						sym.rval = src.rval
					}
				} else {
					sym, level, _ = sc.lookup(dest.ident)
				}
				switch t0, t1 := dest.typ.TypeOf(), src.typ.TypeOf(); n.action {
				case aAddAssign:
					if !(isNumber(t0) && isNumber(t1) || isString(t0) && isString(t1)) || isInt(t0) && isFloat(t1) {
						err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
					}
				case aSubAssign, aMulAssign, aQuoAssign:
					if !(isNumber(t0) && isNumber(t1)) || isInt(t0) && isFloat(t1) {
						err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
					}
				case aRemAssign, aAndAssign, aOrAssign, aXorAssign, aAndNotAssign:
					if !(isInt(t0) && isInt(t1)) {
						err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
					}
				case aShlAssign, aShrAssign:
					if !(isInt(t0) && isUint(t1)) {
						err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
					}
				default:
					// Detect invalid float truncate
					if isInt(t0) && isFloat(t1) {
						err = src.cfgErrorf("invalid float truncate")
						return
					}
				}
				n.findex = dest.findex
				n.val = dest.val
				n.rval = dest.rval
				// Propagate type
				// TODO: Check that existing destination type matches source type
				switch {
				case n.action == aAssign && src.action == aCall:
					n.gen = nop
					src.level = level
					src.findex = dest.findex
				case n.action == aAssign && src.action == aRecv:
					// Assign by reading from a receiving channel
					n.gen = nop
					src.findex = dest.findex // Set recv address to LHS
					dest.typ = src.typ.val
				case n.action == aAssign && src.action == aCompositeLit:
					n.gen = nop
					src.findex = dest.findex
					src.level = level
				case src.kind == basicLit:
					// TODO: perform constant folding and propagation here
					switch {
					case dest.typ.cat == interfaceT:
						// value set in genValue
					case !src.rval.IsValid():
						// Assign to nil
						src.rval = reflect.New(dest.typ.TypeOf()).Elem()
					default:
						// Convert literal value to destination type
						src.rval = src.rval.Convert(dest.typ.TypeOf())
						src.typ = dest.typ
					}
				}
				n.typ = dest.typ
				if sym != nil {
					sym.typ = n.typ
					sym.recv = src.recv
				}
				n.level = level
				if isMapEntry(dest) {
					dest.gen = nop // skip getIndexMap
				}
			}
			if n.anc.kind == constDecl {
				iotaValue++
			}

		case incDecStmt:
			wireChild(n)
			n.findex = n.child[0].findex
			n.level = n.child[0].level
			n.typ = n.child[0].typ
			if sym, level, ok := sc.lookup(n.child[0].ident); ok {
				sym.typ = n.typ
				n.level = level
			}

		case assignXStmt:
			wireChild(n)
			l := len(n.child) - 1
			switch n.child[l].kind {
			case callExpr:
				n.gen = nop
			case indexExpr:
				n.child[l].gen = getIndexMap2
				n.gen = nop
			case typeAssertExpr:
				n.child[l].gen = typeAssert2
				n.gen = nop
			case unaryExpr:
				if n.child[l].action == aRecv {
					n.child[l].gen = recv2
					n.gen = nop
				}
			}

		case defineXStmt:
			wireChild(n)
			l := len(n.child) - 1
			var types []*itype
			switch n.child[l].kind {
			case callExpr:
				if funtype := n.child[l].child[0].typ; funtype.cat == valueT {
					// Handle functions imported from runtime
					for i := 0; i < funtype.rtype.NumOut(); i++ {
						types = append(types, &itype{cat: valueT, rtype: funtype.rtype.Out(i)})
					}
				} else {
					types = funtype.ret
				}
				n.gen = nop

			case indexExpr:
				types = append(types, n.child[l].child[0].typ.val, sc.getType("bool"))
				n.child[l].gen = getIndexMap2
				n.gen = nop

			case typeAssertExpr:
				types = append(types, n.child[l].child[1].typ, sc.getType("bool"))
				n.child[l].gen = typeAssert2
				n.gen = nop

			case unaryExpr:
				if n.child[l].action == aRecv {
					types = append(types, n.child[l].child[0].typ.val, sc.getType("bool"))
					n.child[l].gen = recv2
					n.gen = nop
				}

			default:
				err = n.cfgErrorf("unsupported assign expression")
				return
			}
			for i, t := range types {
				index := sc.add(t)
				sc.sym[n.child[i].ident] = &symbol{index: index, kind: varSym, typ: t}
				n.child[i].typ = t
				n.child[i].findex = index
			}

		case binaryExpr:
			wireChild(n)
			nilSym := interp.universe.sym["nil"]
			c0, c1 := n.child[0], n.child[1]
			t0, t1 := c0.typ.TypeOf(), c1.typ.TypeOf()
			if !c0.typ.untyped && !c1.typ.untyped && c0.typ.id() != c1.typ.id() {
				err = n.cfgErrorf("mismatched types %s and %s", c0.typ.id(), c1.typ.id())
				break
			}
			switch n.action {
			case aAdd:
				if !(isNumber(t0) && isNumber(t1) || isString(t0) && isString(t1)) {
					err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
				}
			case aSub, aMul, aQuo:
				if !(isNumber(t0) && isNumber(t1)) {
					err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
				}
			case aRem, aAnd, aOr, aXor, aAndNot:
				if !(isInt(t0) && isInt(t1)) {
					err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
				}
			case aShl, aShr:
				if !(isInt(t0) && isUint(t1)) {
					err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
				}
				n.typ = c0.typ
			case aEqual, aNotEqual:
				if isNumber(t0) && !isNumber(t1) || isString(t0) && !isString(t1) {
					err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
				}
				n.typ = sc.getType("bool")
				if n.child[0].sym == nilSym || n.child[1].sym == nilSym {
					if n.action == aEqual {
						n.gen = isNil
					} else {
						n.gen = isNotNil
					}
				}
			case aGreater, aGreaterEqual, aLower, aLowerEqual:
				if isNumber(t0) && !isNumber(t1) || isString(t0) && !isString(t1) {
					err = n.cfgErrorf("illegal operand types for '%v' operator", n.action)
				}
				n.typ = sc.getType("bool")
			}
			if err != nil {
				break
			}
			if c0.rval.IsValid() && c1.rval.IsValid() && constOp[n.action] != nil {
				if n.typ == nil {
					if n.typ, err = nodeType(interp, sc, n); err != nil {
						return
					}
				}
				n.typ.TypeOf() // init reflect type
				constOp[n.action](n)
			}
			switch {
			//case n.typ != nil && n.typ.cat == BoolT && isAncBranch(n):
			//	n.findex = -1
			case n.rval.IsValid():
				n.gen = nop
				n.findex = -1
			case n.anc.kind == assignStmt && n.anc.action == aAssign:
				dest := n.anc.child[childPos(n)-n.anc.nright]
				n.typ = dest.typ
				n.findex = dest.findex
			case n.anc.kind == returnStmt:
				pos := childPos(n)
				n.typ = sc.def.typ.ret[pos]
				n.findex = pos
			default:
				if n.typ == nil {
					if n.typ, err = nodeType(interp, sc, n); err != nil {
						return
					}
				}
				n.findex = sc.add(n.typ)
			}

		case indexExpr:
			wireChild(n)
			t := n.child[0].typ
			switch t.cat {
			case valueT:
				n.typ = &itype{cat: valueT, rtype: t.rtype.Elem()}
			case stringT:
				n.typ = sc.getType("byte")
			default:
				n.typ = t.val
			}
			n.findex = sc.add(n.typ)
			n.recv = &receiver{node: n}
			switch k := t.TypeOf().Kind(); k {
			case reflect.Map:
				n.gen = getIndexMap
			case reflect.Array, reflect.Slice, reflect.String:
				n.gen = getIndexArray
			default:
				err = n.cfgErrorf("type is not an array, slice, string or map: %v", t.id())
			}

		case blockStmt:
			wireChild(n)
			if len(n.child) > 0 {
				l := n.lastChild()
				n.findex = l.findex
				n.val = l.val
				n.sym = l.sym
				n.typ = l.typ
				n.rval = l.rval
			}
			sc = sc.pop()

		case constDecl:
			iotaValue = 0
			wireChild(n)

		case varDecl:
			wireChild(n)

		case declStmt, exprStmt, sendStmt:
			wireChild(n)
			l := n.lastChild()
			n.findex = l.findex
			n.val = l.val
			n.sym = l.sym
			n.typ = l.typ
			n.rval = l.rval

		case breakStmt:
			if len(n.child) > 0 {
				gotoLabel(n.sym)
			} else {
				n.tnext = loop
			}

		case continueStmt:
			if len(n.child) > 0 {
				gotoLabel(n.sym)
			} else {
				n.tnext = loopRestart
			}

		case gotoStmt:
			gotoLabel(n.sym)

		case labeledStmt:
			wireChild(n)
			n.start = n.child[1].start
			gotoLabel(n.sym)

		case callExpr:
			wireChild(n)
			switch {
			case isBuiltinCall(n):
				n.gen = n.child[0].sym.builtin
				n.child[0].typ = &itype{cat: builtinT}
				switch n.child[0].ident {
				case "append":
					c1, c2 := n.child[1], n.child[2]
					if n.typ = sc.getType(c1.ident); n.typ == nil {
						if n.typ, err = nodeType(interp, sc, c1); err != nil {
							return
						}
					}
					if len(n.child) == 3 {
						if c2.typ.cat == arrayT && c2.typ.val.id() == n.typ.val.id() ||
							isByteArray(c1.typ.TypeOf()) && isString(c2.typ.TypeOf()) {
							n.gen = appendSlice
						}
					}
				case "cap", "copy", "len":
					n.typ = sc.getType("int")
				case "complex":
					c0, c1 := n.child[1], n.child[2]
					switch t0, t1 := c0.typ.TypeOf(), c1.typ.TypeOf(); {
					case isFloat32(t0) && isFloat32(t1):
						n.typ = sc.getType("complex64")
					case isFloat64(t0) && isFloat64(t1):
						n.typ = sc.getType("complex128")
					case c0.typ.untyped && isNumber(t0) && c1.typ.untyped && isNumber(t1):
						n.typ = &itype{cat: valueT, rtype: complexType}
					case c0.typ.untyped && isFloat32(t1) || c1.typ.untyped && isFloat32(t0):
						n.typ = sc.getType("complex64")
					case c0.typ.untyped && isFloat64(t1) || c1.typ.untyped && isFloat64(t0):
						n.typ = sc.getType("complex128")
					default:
						err = n.cfgErrorf("invalid types %s and %s", t0.Kind(), t1.Kind())
					}
				case "real", "imag":
					switch k := n.child[1].typ.TypeOf().Kind(); {
					case k == reflect.Complex64:
						n.typ = sc.getType("float32")
					case k == reflect.Complex128:
						n.typ = sc.getType("float64")
					case n.child[1].typ.untyped && isNumber(n.child[1].typ.TypeOf()):
						n.typ = &itype{cat: valueT, rtype: floatType}
					default:
						err = n.cfgErrorf("invalid complex type %s", k)
					}
				case "make":
					if n.typ = sc.getType(n.child[1].ident); n.typ == nil {
						if n.typ, err = nodeType(interp, sc, n.child[1]); err != nil {
							return
						}
					}
					n.child[1].val = n.typ
					n.child[1].kind = basicLit
				case "new":
					if n.typ, err = nodeType(interp, sc, n.child[1]); err != nil {
						return
					}
					n.typ = &itype{cat: ptrT, val: n.typ}
				case "recover":
					n.typ = sc.getType("interface{}")
				}
				if n.typ != nil {
					n.findex = sc.add(n.typ)
				} else {
					n.findex = -1
					n.val = nil
				}
			case n.child[0].isType(sc):
				// Type conversion expression
				if isInt(n.child[0].typ.TypeOf()) && n.child[1].kind == basicLit && isFloat(n.child[1].typ.TypeOf()) {
					err = n.cfgErrorf("truncated to integer")
				}
				if isInterface(n.child[0].typ) {
					// Convert to interface: just check that all required methods are defined by concrete type.
					c0, c1 := n.child[0], n.child[1]
					if !c1.typ.implements(c0.typ) {
						err = n.cfgErrorf("type %v does not implement interface %v", c1.typ.id(), c0.typ.id())
					}
					// Pass value as is
					n.gen = nop
					n.typ = n.child[1].typ
					n.findex = n.child[1].findex
					n.val = n.child[1].val
					n.rval = n.child[1].rval
				} else {
					n.gen = convert
					n.typ = n.child[0].typ
					n.findex = sc.add(n.typ)
				}
			case isBinCall(n):
				n.gen = callBin
				if typ := n.child[0].typ.rtype; typ.NumOut() > 0 {
					n.typ = &itype{cat: valueT, rtype: typ.Out(0)}
					n.findex = sc.add(n.typ)
					for i := 1; i < typ.NumOut(); i++ {
						sc.add(&itype{cat: valueT, rtype: typ.Out(i)})
					}
				}
			default:
				if n.child[0].action == aGetFunc {
					// allocate frame entry for anonymous function
					sc.add(n.child[0].typ)
				}
				if typ := n.child[0].typ; len(typ.ret) > 0 {
					n.typ = typ.ret[0]
					n.findex = sc.add(n.typ)
					for _, t := range typ.ret[1:] {
						sc.add(t)
					}
				} else {
					n.findex = -1
				}
			}

		case caseBody:
			wireChild(n)
			if typeSwichAssign(n) && len(n.child) > 1 {
				n.start = n.child[1].start
			} else {
				n.start = n.child[0].start
			}

		case caseClause:
			sc = sc.pop()

		case commClause:
			wireChild(n)
			if len(n.child) > 1 {
				n.start = n.child[1].start // Skip chan operation, performed by select
			} else {
				n.start = n.child[0].start // default clause
			}
			n.lastChild().tnext = n.anc.anc // exit node is SelectStmt
			sc = sc.pop()

		case compositeLitExpr:
			wireChild(n)
			if n.anc.action != aAssign {
				n.findex = sc.add(n.typ)
			}
			// TODO: Check that composite literal expr matches corresponding type
			n.gen = compositeGenerator(n)

		case fallthroughtStmt:
			if n.anc.kind != caseBody {
				err = n.cfgErrorf("fallthrough statement out of place")
			}

		case fileStmt:
			wireChild(n)
			sc = sc.pop()
			n.findex = -1

		case forStmt0: // for {}
			body := n.child[0]
			n.start = body.start
			body.tnext = n.start
			loop, loopRestart = nil, nil
			sc = sc.pop()

		case forStmt1: // for cond {}
			cond, body := n.child[0], n.child[1]
			n.start = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = cond.start
			loop, loopRestart = nil, nil
			sc = sc.pop()

		case forStmt2: // for init; cond; {}
			init, cond, body := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = cond.start
			loop, loopRestart = nil, nil
			sc = sc.pop()

		case forStmt3: // for ; cond; post {}
			cond, post, body := n.child[0], n.child[1], n.child[2]
			n.start = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = post.start
			post.tnext = cond.start
			loop, loopRestart = nil, nil
			sc = sc.pop()

		case forStmt3a: // for int; ; post {}
			init, post, body := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			init.tnext = body.start
			body.tnext = post.start
			post.tnext = body.start
			loop, loopRestart = nil, nil
			sc = sc.pop()

		case forStmt4: // for init; cond; post {}
			init, cond, post, body := n.child[0], n.child[1], n.child[2], n.child[3]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = post.start
			post.tnext = cond.start
			loop, loopRestart = nil, nil
			sc = sc.pop()

		case forRangeStmt:
			loop, loopRestart = nil, nil
			n.start = n.child[0].start
			n.child[0].fnext = n
			sc = sc.pop()

		case funcDecl:
			n.start = n.child[3].start
			n.types = sc.types
			sc = sc.pop()
			funcName := n.child[1].ident
			if !isMethod(n) {
				interp.scopes[pkgName].sym[funcName].index = -1 // to force value to n.val
				interp.scopes[pkgName].sym[funcName].typ = n.typ
				interp.scopes[pkgName].sym[funcName].kind = funcSym
				interp.scopes[pkgName].sym[funcName].node = n
			}

		case funcLit:
			n.types = sc.types
			sc = sc.pop()

		case goStmt:
			wireChild(n)

		case identExpr:
			if isKey(n) || isNewDefine(n, sc) {
				break
			} else if sym, level, ok := sc.lookup(n.ident); ok {
				// Found symbol, populate node info
				n.typ, n.findex, n.level = sym.typ, sym.index, level
				if n.findex < 0 {
					n.val = sym.node
				} else {
					n.sym = sym
					switch {
					case sym.kind == constSym && sym.rval.IsValid():
						n.rval = sym.rval
						n.kind = basicLit
					case n.ident == "iota":
						n.rval = reflect.ValueOf(iotaValue)
						n.kind = basicLit
					case n.ident == "nil":
						n.kind = basicLit
					case sym.kind == binSym:
						if sym.rval.IsValid() {
							n.kind = rvalueExpr
						} else {
							n.kind = rtypeExpr
						}
						n.typ = sym.typ
						n.rval = sym.rval
					case sym.kind == bltnSym:
						if n.anc.kind != callExpr {
							err = n.cfgErrorf("use of builtin %s not in function call", n.ident)
						}
					}
					if sym.kind == varSym && sym.typ != nil && sym.typ.TypeOf().Kind() == reflect.Bool {
						switch n.anc.kind {
						case ifStmt0, ifStmt1, ifStmt2, ifStmt3, forStmt1, forStmt2, forStmt3, forStmt4:
							n.gen = branch
						}
					}
				}
				if n.sym != nil {
					n.recv = n.sym.recv
				}
			} else {
				err = n.cfgErrorf("undefined: %s", n.ident)
			}

		case ifStmt0: // if cond {}
			cond, tbody := n.child[0], n.child[1]
			n.start = cond.start
			cond.tnext = tbody.start
			cond.fnext = n
			tbody.tnext = n
			sc = sc.pop()

		case ifStmt1: // if cond {} else {}
			cond, tbody, fbody := n.child[0], n.child[1], n.child[2]
			n.start = cond.start
			cond.tnext = tbody.start
			cond.fnext = fbody.start
			tbody.tnext = n
			fbody.tnext = n
			sc = sc.pop()

		case ifStmt2: // if init; cond {}
			init, cond, tbody := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			tbody.tnext = n
			init.tnext = cond.start
			cond.tnext = tbody.start
			cond.fnext = n
			sc = sc.pop()

		case ifStmt3: // if init; cond {} else {}
			init, cond, tbody, fbody := n.child[0], n.child[1], n.child[2], n.child[3]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = tbody.start
			cond.fnext = fbody.start
			tbody.tnext = n
			fbody.tnext = n
			sc = sc.pop()

		case keyValueExpr:
			wireChild(n)

		case landExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n.child[1].start
			n.child[0].fnext = n
			n.child[1].tnext = n
			n.typ = n.child[0].typ
			n.findex = sc.add(n.typ)

		case lorExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n
			n.child[0].fnext = n.child[1].start
			n.child[1].tnext = n
			n.typ = n.child[0].typ
			n.findex = sc.add(n.typ)

		case parenExpr:
			wireChild(n)
			c := n.lastChild()
			n.findex = c.findex
			n.typ = c.typ
			n.rval = c.rval

		case rangeStmt:
			if sc.rangeChanType(n) != nil {
				n.start = n.child[1]       // Get chan
				n.child[1].tnext = n       // then go to range function
				n.tnext = n.child[2].start // then go to range body
				n.child[2].tnext = n       // then body go to range function (loop)
				n.child[0].gen = empty
			} else {
				var k, o, body *node
				if len(n.child) == 4 {
					k, o, body = n.child[0], n.child[2], n.child[3]
				} else {
					k, o, body = n.child[0], n.child[1], n.child[2]
				}
				n.start = o          // Get array or map object
				o.tnext = k.start    // then go to iterator init
				k.tnext = n          // then go to range function
				n.tnext = body.start // then go to range body
				body.tnext = n       // then body go to range function (loop)
				k.gen = empty        // init filled later by generator
			}

		case returnStmt:
			wireChild(n)
			n.tnext = nil
			n.val = sc.def
			for i, c := range n.child {
				if c.typ.cat == nilT {
					// nil: Set node value to zero of return type
					f := sc.def
					var typ *itype
					if typ, err = nodeType(interp, sc, f.child[2].child[1].child[i].lastChild()); err != nil {
						return
					}
					c.rval = reflect.New(typ.TypeOf()).Elem()
				}
			}

		case selectorExpr:
			wireChild(n)
			n.typ = n.child[0].typ
			n.recv = n.child[0].recv
			if n.typ == nil {
				err = n.cfgErrorf("undefined type")
				break
			}
			if n.typ.cat == valueT || n.typ.cat == errorT {
				// Handle object defined in runtime, try to find field or method
				// Search for method first, as it applies both to types T and *T
				// Search for field must then be performed on type T only (not *T)
				switch method, ok := n.typ.rtype.MethodByName(n.child[1].ident); {
				case ok:
					n.val = method.Index
					n.gen = getIndexBinMethod
					n.recv = &receiver{node: n.child[0]}
					n.typ = &itype{cat: valueT, rtype: method.Type}
				case n.typ.rtype.Kind() == reflect.Ptr:
					if field, ok := n.typ.rtype.Elem().FieldByName(n.child[1].ident); ok {
						n.typ = &itype{cat: valueT, rtype: field.Type}
						n.val = field.Index
						n.gen = getPtrIndexSeq
					} else {
						err = n.cfgErrorf("undefined field or method: %s", n.child[1].ident)
					}
				case n.typ.rtype.Kind() == reflect.Struct:
					if field, ok := n.typ.rtype.FieldByName(n.child[1].ident); ok {
						n.typ = &itype{cat: valueT, rtype: field.Type}
						n.val = field.Index
						n.gen = getIndexSeq
					} else {
						// method lookup failed on type, now lookup on pointer to type
						pt := reflect.PtrTo(n.typ.rtype)
						if m2, ok2 := pt.MethodByName(n.child[1].ident); ok2 {
							n.val = m2.Index
							n.gen = getIndexBinPtrMethod
							n.typ = &itype{cat: valueT, rtype: m2.Type}
							n.recv = &receiver{node: n.child[0]}
						} else {
							err = n.cfgErrorf("undefined field or method: %s", n.child[1].ident)
						}
					}
				default:
					err = n.cfgErrorf("undefined field or method: %s", n.child[1].ident)
				}
			} else if n.typ.cat == ptrT && (n.typ.val.cat == valueT || n.typ.val.cat == errorT) {
				// Handle pointer on object defined in runtime
				if field, ok := n.typ.val.rtype.FieldByName(n.child[1].ident); ok {
					n.typ = &itype{cat: valueT, rtype: field.Type}
					n.val = field.Index
					n.gen = getPtrIndexSeq
				} else if method, ok := n.typ.val.rtype.MethodByName(n.child[1].ident); ok {
					n.val = method.Index
					n.typ = &itype{cat: valueT, rtype: method.Type}
					n.recv = &receiver{node: n.child[0]}
					n.gen = getIndexBinMethod
				} else if method, ok := reflect.PtrTo(n.typ.val.rtype).MethodByName(n.child[1].ident); ok {
					n.val = method.Index
					n.gen = getIndexBinMethod
					n.typ = &itype{cat: valueT, rtype: method.Type}
					n.recv = &receiver{node: n.child[0]}
				} else {
					err = n.cfgErrorf("undefined selector: %s", n.child[1].ident)
				}
			} else if n.typ.cat == binPkgT {
				// Resolve binary package symbol: a type or a value
				name := n.child[1].ident
				pkg := n.child[0].sym.path
				if s, ok := interp.binValue[pkg][name]; ok {
					if isBinType(s) {
						n.kind = rtypeExpr
						n.typ = &itype{cat: valueT, rtype: s.Type().Elem()}
					} else {
						n.kind = rvalueExpr
						n.typ = &itype{cat: valueT, rtype: s.Type()}
						n.rval = s
					}
					n.gen = nop
				} else {
					err = n.cfgErrorf("package %s \"%s\" has no symbol %s", n.child[0].ident, pkg, name)
				}
			} else if n.typ.cat == srcPkgT {
				pkg, name := n.child[0].ident, n.child[1].ident
				// Resolve source package symbol
				if sym, ok := interp.scopes[pkg].sym[name]; ok {
					n.findex = sym.index
					n.val = sym.node
					n.gen = nop
					n.typ = sym.typ
					n.sym = sym
				} else {
					err = n.cfgErrorf("undefined selector: %s", n.child[1].ident)
				}
			} else if m, lind := n.typ.lookupMethod(n.child[1].ident); m != nil {
				if n.child[0].isType(sc) {
					// Handle method as a function with receiver in 1st argument
					n.val = m
					n.findex = -1
					n.gen = nop
					n.typ = &itype{}
					*n.typ = *m.typ
					n.typ.arg = append([]*itype{n.child[0].typ}, m.typ.arg...)
				} else {
					// Handle method with receiver
					n.gen = getMethod
					n.val = m
					n.typ = m.typ
					n.recv = &receiver{node: n.child[0], index: lind}
				}
			} else if m, lind, ok := n.typ.lookupBinMethod(n.child[1].ident); ok {
				n.gen = getIndexSeqMethod
				n.val = append([]int{m.Index}, lind...)
				n.typ = &itype{cat: valueT, rtype: m.Type}
			} else if ti := n.typ.lookupField(n.child[1].ident); len(ti) > 0 {
				// Handle struct field
				n.val = ti
				switch n.typ.cat {
				case interfaceT:
					n.typ = n.typ.fieldSeq(ti)
					n.gen = getMethodByName
					n.action = aMethod
				case ptrT:
					n.typ = n.typ.fieldSeq(ti)
					n.gen = getPtrIndexSeq
					if n.typ.cat == funcT {
						// function in a struct field is always wrapped in reflect.Value
						rtype := n.typ.TypeOf()
						n.typ = &itype{cat: valueT, rtype: rtype}
					}
				default:
					n.gen = getIndexSeq
					n.typ = n.typ.fieldSeq(ti)
					if n.typ.cat == funcT {
						// function in a struct field is always wrapped in reflect.Value
						rtype := n.typ.TypeOf()
						n.typ = &itype{cat: valueT, rtype: rtype}
					}
				}
			} else if s, lind, ok := n.typ.lookupBinField(n.child[1].ident); ok {
				// Handle an embedded binary field into a struct field
				n.gen = getIndexSeqField
				lind = append(lind, s.Index...)
				n.val = lind
				n.typ = &itype{cat: valueT, rtype: s.Type}
			} else {
				err = n.cfgErrorf("undefined selector: %s", n.child[1].ident)
			}
			if err == nil && n.findex != -1 {
				n.findex = sc.add(n.typ)
			}

		case selectStmt:
			wireChild(n)
			// Move action to block statement, so select node can be an exit point
			n.child[0].gen = _select
			n.start = n.child[0]

		case starExpr:
			switch {
			case n.anc.kind == defineStmt && len(n.anc.child) == 3 && n.anc.child[1] == n:
				// pointer type expression in a var definition
				n.gen = nop
			case n.anc.kind == valueSpec && n.anc.lastChild() == n:
				// pointer type expression in a value spec
				n.gen = nop
			case n.anc.kind == fieldExpr:
				// pointer type expression in a field expression (arg or struct field)
				n.gen = nop
			case n.child[0].isType(sc):
				// pointer type expression
				n.gen = nop
				n.typ = &itype{cat: ptrT, val: n.child[0].typ}
			default:
				// dereference expression
				wireChild(n)
				n.typ = n.child[0].typ.val
				n.findex = sc.add(n.typ)
			}

		case typeSwitch:
			// Check that cases expressions are all different
			usedCase := map[string]bool{}
			for _, c := range n.lastChild().child {
				for _, t := range c.child[:len(c.child)-1] {
					tid := t.typ.id()
					if usedCase[tid] {
						err = c.cfgErrorf("duplicate case %s in type switch", t.ident)
						return
					}
					usedCase[tid] = true
				}
			}
			fallthrough

		case switchStmt:
			sbn := n.lastChild() // switch block node
			clauses := sbn.child
			l := len(clauses)
			// Chain case clauses
			for i, c := range clauses[:l-1] {
				c.fnext = clauses[i+1] // chain to next clause
				body := c.lastChild()
				c.tnext = body.start
				if len(body.child) > 0 && body.lastChild().kind == fallthroughtStmt {
					if n.kind == typeSwitch {
						err = body.lastChild().cfgErrorf("cannot fallthrough in type switch")
					}
					body.tnext = clauses[i+1].lastChild().start
				} else {
					body.tnext = n
				}
			}
			c := clauses[l-1]
			c.tnext = c.lastChild().start
			if n.child[0].action == aAssign &&
				(n.child[0].child[0].kind != typeAssertExpr || len(n.child[0].child[0].child) > 1) {
				// switch init statement is defined
				n.start = n.child[0].start
				n.child[0].tnext = sbn.start
			} else {
				n.start = sbn.start
			}
			sc = sc.pop()
			loop = nil

		case switchIfStmt: // like an if-else chain
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
				if i < l-1 && len(body.child) > 0 && body.lastChild().kind == fallthroughtStmt {
					body.tnext = clauses[i+1].lastChild().start
				}
			}
			sbn.start = clauses[0].start
			if n.child[0].action == aAssign {
				// switch init statement is defined
				n.start = n.child[0].start
				n.child[0].tnext = sbn.start
			} else {
				n.start = sbn.start
			}
			sc = sc.pop()
			loop = nil

		case typeAssertExpr:
			if len(n.child) > 1 {
				wireChild(n)
				if n.child[1].typ == nil {
					n.child[1].typ = sc.getType(n.child[1].ident)
				}
				if n.anc.action != aAssignX {
					n.typ = n.child[1].typ
					n.findex = sc.add(n.typ)
				}
			} else {
				n.gen = nop
			}

		case sliceExpr:
			wireChild(n)
			if ctyp := n.child[0].typ; ctyp.size != 0 {
				// Create a slice type from an array type
				n.typ = &itype{}
				*n.typ = *ctyp
				n.typ.size = 0
				n.typ.rtype = nil
			} else {
				n.typ = ctyp
			}
			n.findex = sc.add(n.typ)

		case unaryExpr:
			wireChild(n)
			n.typ = n.child[0].typ
			// TODO: Optimisation: avoid allocation if boolean branch op (i.e. '!' in an 'if' expr)
			n.findex = sc.add(n.typ)

		case valueSpec:
			n.gen = reset
			l := len(n.child) - 1
			if n.typ = n.child[l].typ; n.typ == nil {
				if n.typ, err = nodeType(interp, sc, n.child[l]); err != nil {
					return
				}
			}
			for _, c := range n.child[:l] {
				index := sc.add(n.typ)
				sc.sym[c.ident] = &symbol{index: index, kind: varSym, typ: n.typ}
				c.typ = n.typ
				c.findex = index
			}
		}
	})

	if sc != interp.universe {
		sc.pop()
	}
	return initNodes, err
}

// used for allocation optimization, temporarily disabled
//func isAncBranch(n *node) bool {
//	switch n.anc.kind {
//	case If0, If1, If2, If3:
//		return true
//	}
//	return false
//}

func childPos(n *node) int {
	for i, c := range n.anc.child {
		if n == c {
			return i
		}
	}
	return -1
}

func (n *node) cfgErrorf(format string, a ...interface{}) cfgError {
	a = append([]interface{}{n.interp.fset.Position(n.pos)}, a...)
	return cfgError(fmt.Errorf("%s: "+format, a...))
}

func genRun(nod *node) error {
	var err cfgError

	nod.Walk(func(n *node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case funcType:
			if len(n.anc.child) == 4 {
				// function body entry point
				setExec(n.anc.child[3].start)
			}
			// continue in function body as there may be inner function definitions
		case constDecl, varDecl:
			setExec(n.start)
			return false
		}
		return true
	}, nil)

	return err
}

// Find default case clause index of a switch statement, if any
func getDefault(n *node) int {
	for i, c := range n.lastChild().child {
		if len(c.child) == 1 {
			return i
		}
	}
	return -1
}

func isBinType(v reflect.Value) bool { return v.IsValid() && v.Kind() == reflect.Ptr && v.IsNil() }

// isType returns true if node refers to a type definition, false otherwise
func (n *node) isType(sc *scope) bool {
	switch n.kind {
	case arrayType, chanType, funcType, mapType, structType, rtypeExpr:
		return true
	case parenExpr, starExpr:
		if len(n.child) == 1 {
			return n.child[0].isType(sc)
		}
	case selectorExpr:
		pkg, name := n.child[0].ident, n.child[1].ident
		if sym, _, ok := sc.lookup(pkg); ok {
			if p, ok := n.interp.binValue[sym.path]; ok && isBinType(p[name]) {
				return true // Imported binary type
			}
			if p, ok := n.interp.scopes[pkg]; ok && p.sym[name] != nil && p.sym[name].kind == typeSym {
				return true // Imported source type
			}
		}
	case identExpr:
		return sc.getType(n.ident) != nil
	}
	return false
}

// wireChild wires AST nodes for CFG in subtree
func wireChild(n *node) {
	// Set start node, in subtree (propagated to ancestors by post-order processing)
	for _, child := range n.child {
		switch child.kind {
		case arrayType, chanType, funcDecl, importDecl, mapType, basicLit, identExpr, typeDecl:
			continue
		default:
			n.start = child.start
		}
		break
	}

	// Chain sequential operations inside a block (next is right sibling)
	for i := 1; i < len(n.child); i++ {
		switch n.child[i].kind {
		case funcDecl:
			n.child[i-1].tnext = n.child[i]
		default:
			switch n.child[i-1].kind {
			case breakStmt, continueStmt, gotoStmt, returnStmt:
				// tnext is already computed, no change
			default:
				n.child[i-1].tnext = n.child[i].start
			}
		}
	}

	// Chain subtree next to self
	for i := len(n.child) - 1; i >= 0; i-- {
		switch n.child[i].kind {
		case arrayType, chanType, importDecl, mapType, funcDecl, basicLit, identExpr, typeDecl:
			continue
		case breakStmt, continueStmt, gotoStmt, returnStmt:
			// tnext is already computed, no change
		default:
			n.child[i].tnext = n
		}
		break
	}
}

// last returns the last child of a node
func (n *node) lastChild() *node { return n.child[len(n.child)-1] }

func isKey(n *node) bool {
	return n.anc.kind == fileStmt ||
		(n.anc.kind == selectorExpr && n.anc.child[0] != n) ||
		(n.anc.kind == funcDecl && isMethod(n.anc)) ||
		(n.anc.kind == keyValueExpr && isStruct(n.anc.typ) && n.anc.child[0] == n)
}

// isNewDefine returns true if node refers to a new definition
func isNewDefine(n *node, sc *scope) bool {
	if n.ident == "_" {
		return true
	}
	if (n.anc.kind == defineXStmt || n.anc.kind == defineStmt || n.anc.kind == valueSpec) && childPos(n) < n.anc.nleft {
		return true
	}
	if n.anc.kind == rangeStmt {
		if n.anc.child[0] == n {
			return true // array or map key, or chan element
		}
		if sc.rangeChanType(n.anc) == nil && n.anc.child[1] == n && len(n.anc.child) == 4 {
			return true // array or map value
		}
		return false // array, map or channel are always pre-defined in range expression
	}
	return false
}

func isMethod(n *node) bool {
	return len(n.child[0].child) > 0 // receiver defined
}

func isMapEntry(n *node) bool {
	return n.action == aGetIndex && n.child[0].typ.cat == mapT
}

func isBuiltinCall(n *node) bool {
	return n.kind == callExpr && n.child[0].sym != nil && n.child[0].sym.kind == bltnSym
}

func isBinCall(n *node) bool {
	return n.kind == callExpr && n.child[0].typ.cat == valueT && n.child[0].typ.rtype.Kind() == reflect.Func
}

func isRegularCall(n *node) bool {
	return n.kind == callExpr && n.child[0].typ.cat == funcT
}

func variadicPos(n *node) int {
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

func getExec(n *node) bltn {
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
func setExec(n *node) {
	if n.exec != nil {
		return
	}
	seen := map[*node]bool{}
	var set func(n *node)

	set = func(n *node) {
		if n == nil || n.exec != nil {
			return
		}
		seen[n] = true
		if n.tnext != nil && n.tnext.exec == nil {
			if seen[n.tnext] {
				m := n.tnext
				n.tnext.exec = func(f *frame) bltn { return m.exec(f) }
			} else {
				set(n.tnext)
			}
		}
		if n.fnext != nil && n.fnext.exec == nil {
			if seen[n.fnext] {
				m := n.fnext
				n.fnext.exec = func(f *frame) bltn { return m.exec(f) }
			} else {
				set(n.fnext)
			}
		}
		n.gen(n)
	}

	set(n)
}

func typeSwichAssign(n *node) bool {
	ts := n.anc.anc.anc
	return ts.kind == typeSwitch && ts.child[1].action == aAssign
}

func gotoLabel(s *symbol) {
	if s.node == nil {
		return
	}
	for _, c := range s.from {
		c.tnext = s.node.start
	}
}

func compositeGenerator(n *node) (gen bltnGenerator) {
	switch n.typ.cat {
	case aliasT:
		n.typ = n.typ.val
		gen = compositeGenerator(n)
	case arrayT:
		gen = arrayLit
	case mapT:
		gen = mapLit
	case structT:
		if n.lastChild().kind == keyValueExpr {
			gen = compositeSparse
		} else {
			gen = compositeLit
		}
	case valueT:
		switch k := n.typ.rtype.Kind(); k {
		case reflect.Struct:
			gen = compositeBinStruct
		case reflect.Map:
			gen = compositeBinMap
		default:
			log.Panic(n.cfgErrorf("compositeGenerator not implemented for type kind: %s", k))
		}
	}
	return
}
