package interp

import (
	"fmt"
	"go/constant"
	"log"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
)

// A cfgError represents an error during CFG build stage.
type cfgError struct {
	*node
	error
}

func (c *cfgError) Error() string { return c.error.Error() }

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
	aNot:    notConst,
	aBitNot: bitNotConst,
	aNeg:    negConst,
	aPos:    posConst,
}

var constBltn = map[string]func(*node){
	bltnComplex: complexConst,
	bltnImag:    imagConst,
	bltnReal:    realConst,
}

const nilIdent = "nil"

// cfg generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables. A list of nodes of init functions is returned.
// Following this pass, the CFG is ready to run.
func (interp *Interpreter) cfg(root *node, importPath, pkgName string) ([]*node, error) {
	sc := interp.initScopePkg(importPath, pkgName)
	check := typecheck{scope: sc}
	var initNodes []*node
	var err error

	baseName := filepath.Base(interp.fset.Position(root.pos).Filename)

	root.Walk(func(n *node) bool {
		// Pre-order processing
		if err != nil {
			return false
		}
		switch n.kind {
		case binaryExpr, unaryExpr, parenExpr:
			if isBoolAction(n) {
				break
			}
			// Gather assigned type if set, to give context for type propagation at post-order.
			switch n.anc.kind {
			case assignStmt, defineStmt:
				a := n.anc
				i := childPos(n) - a.nright
				if i < 0 {
					break
				}
				if len(a.child) > a.nright+a.nleft {
					i--
				}
				dest := a.child[i]
				if dest.typ == nil {
					break
				}
				if dest.typ.incomplete {
					err = n.cfgErrorf("invalid type declaration")
					return false
				}
				if !isInterface(dest.typ) {
					// Interface type are not propagated, and will be resolved at post-order.
					n.typ = dest.typ
				}
			case binaryExpr, unaryExpr, parenExpr:
				n.typ = n.anc.typ
			}

		case defineStmt:
			// Determine type of variables initialized at declaration, so it can be propagated.
			if n.nleft+n.nright == len(n.child) {
				// No type was specified on the left hand side, it will resolved at post-order.
				break
			}
			n.typ, err = nodeType(interp, sc, n.child[n.nleft])
			if err != nil {
				break
			}
			for i := 0; i < n.nleft; i++ {
				n.child[i].typ = n.typ
			}

		case blockStmt:
			if n.anc != nil && n.anc.kind == rangeStmt {
				// For range block: ensure that array or map type is propagated to iterators
				// prior to process block. We cannot perform this at RangeStmt pre-order because
				// type of array like value is not yet known. This could be fixed in ast structure
				// by setting array/map node as 1st child of ForRangeStmt instead of 3rd child of
				// RangeStmt. The following workaround is less elegant but ok.
				c := n.anc.child[1]
				if c != nil && c.typ != nil && isSendChan(c.typ) {
					err = c.cfgErrorf("invalid operation: range %s receive from send-only channel", c.ident)
					return false
				}

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
							ityp := valueTOf(reflect.TypeOf((*reflect.MapIter)(nil)))
							sc.add(ityp)
							ktyp = valueTOf(typ.Key())
							vtyp = valueTOf(typ.Elem())
						case reflect.String:
							sc.add(sc.getType("int")) // Add a dummy type to store array shallow copy for range
							sc.add(sc.getType("int")) // Add a dummy type to store index for range
							ktyp = sc.getType("int")
							vtyp = sc.getType("rune")
						case reflect.Array, reflect.Slice:
							sc.add(sc.getType("int")) // Add a dummy type to store array shallow copy for range
							ktyp = sc.getType("int")
							vtyp = valueTOf(typ.Elem())
						}
					case mapT:
						n.anc.gen = rangeMap
						ityp := valueTOf(reflect.TypeOf((*reflect.MapIter)(nil)))
						sc.add(ityp)
						ktyp = o.typ.key
						vtyp = o.typ.val
					case ptrT:
						ktyp = sc.getType("int")
						vtyp = o.typ.val
						if vtyp.cat == valueT {
							vtyp = valueTOf(vtyp.rtype.Elem())
						} else {
							vtyp = vtyp.val
						}
					case stringT:
						sc.add(sc.getType("int")) // Add a dummy type to store array shallow copy for range
						sc.add(sc.getType("int")) // Add a dummy type to store index for range
						ktyp = sc.getType("int")
						vtyp = sc.getType("rune")
					case arrayT, sliceT, variadicT:
						sc.add(sc.getType("int")) // Add a dummy type to store array shallow copy for range
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
				// Handle labeled statements.
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
			// TODO(marc): labels must be stored outside of symbols to avoid collisions
			// Used labels are searched in current and sub scopes, not upper ones.
			if sym, ok := sc.lookdown(label); ok {
				sym.node = n
				n.sym = sym
			} else {
				n.sym = &symbol{kind: labelSym, node: n, index: -1}
			}
			sc.sym[label] = n.sym

		case caseClause:
			sc = sc.pushBloc()
			if sn := n.anc.anc; sn.kind == typeSwitch && sn.child[1].action == aAssign {
				// Type switch clause with a var defined in switch guard.
				var typ *itype
				if len(n.child) == 2 {
					// 1 type in clause: define the var with this type in the case clause scope.
					switch {
					case n.child[0].ident == nilIdent:
						typ = sc.getType("interface{}")
					case !n.child[0].isType(sc):
						err = n.cfgErrorf("%s is not a type", n.child[0].ident)
					default:
						typ, err = nodeType(interp, sc, n.child[0])
					}
				} else {
					// Define the var with the type in the switch guard expression.
					typ = sn.child[1].child[1].child[0].typ
				}
				if err != nil {
					return false
				}
				nod := n.lastChild().child[0]
				index := sc.add(typ)
				sc.sym[nod.ident] = &symbol{index: index, kind: varSym, typ: typ}
				nod.findex = index
				nod.typ = typ
			}

		case commClauseDefault:
			sc = sc.pushBloc()

		case commClause:
			sc = sc.pushBloc()
			if len(n.child) > 0 && n.child[0].action == aAssign {
				ch := n.child[0].child[1].child[0]
				var typ *itype
				if typ, err = nodeType(interp, sc, ch); err != nil {
					return false
				}
				if !isChan(typ) {
					err = n.cfgErrorf("invalid operation: receive from non-chan type")
					return false
				}
				elem := chanElement(typ)
				assigned := n.child[0].child[0]
				index := sc.add(elem)
				sc.sym[assigned.ident] = &symbol{index: index, kind: varSym, typ: elem}
				assigned.findex = index
				assigned.typ = elem
			}

		case compositeLitExpr:
			if len(n.child) > 0 && n.child[0].isType(sc) {
				// Get type from 1st child
				if n.typ, err = nodeType(interp, sc, n.child[0]); err != nil {
					return false
				}
				// Indicate that the first child is the type
				n.nleft = 1
			} else {
				// Get type from ancestor (implicit type)
				if n.anc.kind == keyValueExpr && n == n.anc.child[0] {
					n.typ = n.anc.typ.key
				} else if atyp := n.anc.typ; atyp != nil {
					if atyp.cat == valueT && hasElem(atyp.rtype) {
						n.typ = valueTOf(atyp.rtype.Elem())
					} else {
						n.typ = atyp.val
					}
				}
				if n.typ == nil {
					err = n.cfgErrorf("undefined type")
					return false
				}
			}

			child := n.child
			if n.nleft > 0 {
				n.child[0].typ = n.typ
				child = n.child[1:]
			}
			// Propagate type to children, to handle implicit types
			for _, c := range child {
				switch c.kind {
				case binaryExpr, unaryExpr, compositeLitExpr:
					// Do not attempt to propagate composite type to operator expressions,
					// it breaks constant folding.
				case keyValueExpr, typeAssertExpr, indexExpr:
					c.typ = n.typ
				default:
					if c.ident == nilIdent {
						c.typ = sc.getType(nilIdent)
						continue
					}
					if c.typ, err = nodeType(interp, sc, c); err != nil {
						return false
					}
				}
			}

		case forStmt0, forStmt1, forStmt2, forStmt3, forStmt4, forStmt5, forStmt6, forStmt7, forRangeStmt:
			sc = sc.pushBloc()
			sc.loop, sc.loopRestart = n, n.lastChild()

		case funcLit:
			n.typ = nil // to force nodeType to recompute the type
			if n.typ, err = nodeType(interp, sc, n); err != nil {
				return false
			}
			n.findex = sc.add(n.typ)
			fallthrough

		case funcDecl:
			n.val = n
			// Compute function type before entering local scope to avoid
			// possible collisions with function argument names.
			n.child[2].typ, err = nodeType(interp, sc, n.child[2])
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
				fr := n.child[0].child[0]
				recvTypeNode := fr.lastChild()
				if typ, err = nodeType(interp, sc, recvTypeNode); err != nil {
					return false
				}
				recvTypeNode.typ = typ
				n.child[2].typ.recv = typ
				n.typ.recv = typ
				index := sc.add(typ)
				if len(fr.child) > 1 {
					sc.sym[fr.child[0].ident] = &symbol{index: index, kind: varSym, typ: typ}
				}
			}
			for _, c := range n.child[2].child[0].child {
				// define input parameter symbols
				var typ *itype
				if typ, err = nodeType(interp, sc, c.lastChild()); err != nil {
					return false
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
			// Make sure default clause is in last position.
			c := n.lastChild().child
			if i, l := getDefault(n), len(c)-1; i >= 0 && i != l {
				c[i], c[l] = c[l], c[i]
			}
			sc = sc.pushBloc()
			sc.loop = n

		case importSpec:
			// already all done in gta
			return false

		case typeSpec:
			// processing already done in GTA pass for global types, only parses inlined types
			if sc.def == nil {
				return false
			}
			typeName := n.child[0].ident
			var typ *itype
			if typ, err = nodeType(interp, sc, n.child[1]); err != nil {
				return false
			}
			if typ.incomplete {
				err = n.cfgErrorf("invalid type declaration")
				return false
			}

			switch n.child[1].kind {
			case identExpr, selectorExpr:
				n.typ = namedOf(typ, pkgName, typeName)
			default:
				n.typ = typ
				n.typ.name = typeName
			}
			sc.sym[typeName] = &symbol{kind: typeSym, typ: n.typ}
			return false

		case constDecl:
			// Early parse of constDecl subtrees, to compute all constant
			// values which may be used in further declarations.
			if !sc.global {
				for _, c := range n.child {
					if _, err = interp.cfg(c, importPath, pkgName); err != nil {
						// No error processing here, to allow recovery in subtree nodes.
						err = nil
					}
				}
			}

		case arrayType, basicLit, chanType, chanTypeRecv, chanTypeSend, funcType, interfaceType, mapType, structType:
			n.typ, err = nodeType(interp, sc, n)
			return false
		}
		return true
	}, func(n *node) {
		// Post-order processing
		if err != nil {
			return
		}

		defer func() {
			if r := recover(); r != nil {
				// Display the exact location in input source which triggered the panic
				panic(n.cfgErrorf("CFG post-order panic: %v", r))
			}
		}()

		switch n.kind {
		case addressExpr:
			wireChild(n)

			err = check.addressExpr(n)
			if err != nil {
				break
			}

			n.typ = ptrOf(n.child[0].typ)
			n.findex = sc.add(n.typ)

		case assignStmt, defineStmt:
			if n.anc.kind == typeSwitch && n.anc.child[1] == n {
				// type switch guard assignment: assign dest to concrete value of src
				n.gen = nop
				break
			}

			var atyp *itype
			if n.nleft+n.nright < len(n.child) {
				if atyp, err = nodeType(interp, sc, n.child[n.nleft]); err != nil {
					break
				}
			}

			var sbase int
			if n.nright > 0 {
				sbase = len(n.child) - n.nright
			}

			wireChild(n)
			for i := 0; i < n.nleft; i++ {
				dest, src := n.child[i], n.child[sbase+i]
				updateSym := false
				var sym *symbol
				var level int
				if n.kind == defineStmt || (n.kind == assignStmt && dest.ident == "_") {
					if atyp != nil {
						dest.typ = atyp
					} else {
						if src.typ, err = nodeType(interp, sc, src); err != nil {
							return
						}
						if src.typ.isBinMethod {
							dest.typ = valueTOf(src.typ.methodCallType())
						} else {
							// In a new definition, propagate the source type to the destination
							// type. If the source is an untyped constant, make sure that the
							// type matches a default type.
							dest.typ = sc.fixType(src.typ)
						}
					}
					if dest.typ.incomplete {
						return
					}
					if sc.global {
						// Do not overload existing symbols (defined in GTA) in global scope
						sym, _, _ = sc.lookup(dest.ident)
					}
					if sym == nil {
						sym = &symbol{index: sc.add(dest.typ), kind: varSym, typ: dest.typ}
						sc.sym[dest.ident] = sym
					}
					dest.val = src.val
					dest.recv = src.recv
					dest.findex = sym.index
					updateSym = true
				} else {
					sym, level, _ = sc.lookup(dest.ident)
				}

				err = check.assignExpr(n, dest, src)
				if err != nil {
					break
				}

				if updateSym {
					sym.typ = dest.typ
					sym.rval = src.rval
					// As we are updating the sym type, we need to update the sc.type
					// when the sym has an index.
					if sym.index >= 0 {
						sc.types[sym.index] = sym.typ.frameType()
					}
				}
				n.findex = dest.findex
				n.level = dest.level

				// In the following, we attempt to optimize by skipping the assign
				// operation and setting the source location directly to the destination
				// location in the frame.
				//
				switch {
				case n.action != aAssign:
					// Do not skip assign operation if it is combined with another operator.
				case src.rval.IsValid():
					// Do not skip assign operation if setting from a constant value.
				case isMapEntry(dest):
					// Setting a map entry requires an additional step, do not optimize.
					// As we only write, skip the default useless getIndexMap dest action.
					dest.gen = nop
				case isFuncField(dest):
					// Setting a struct field of function type requires an extra step. Do not optimize.
				case isCall(src) && !isInterfaceSrc(dest.typ) && !isRecursiveField(dest) && n.kind != defineStmt:
					// Call action may perform the assignment directly.
					if dest.typ.id() != src.typ.id() {
						// Skip optimitization if returned type doesn't match assigned one.
						break
					}
					n.gen = nop
					src.level = level
					src.findex = dest.findex
					if src.typ.untyped && !dest.typ.untyped {
						src.typ = dest.typ
					}
				case src.action == aRecv:
					// Assign by reading from a receiving channel.
					n.gen = nop
					src.findex = dest.findex // Set recv address to LHS.
					dest.typ = src.typ
				case src.action == aCompositeLit:
					if dest.typ.cat == valueT && dest.typ.rtype.Kind() == reflect.Interface {
						// Skip optimisation for assigned interface.
						break
					}
					if dest.action == aGetIndex {
						// Skip optimization, as it does not work when assigning to a struct field.
						break
					}
					n.gen = nop
					src.findex = dest.findex
					src.level = level
				case len(n.child) < 4 && isArithmeticAction(src):
					// Optimize single assignments from some arithmetic operations.
					src.typ = dest.typ
					src.findex = dest.findex
					src.level = level
					n.gen = nop
				case src.kind == basicLit:
					// Assign to nil.
					src.rval = reflect.New(dest.typ.TypeOf()).Elem()
				case n.nright == 0:
					n.gen = reset
				}

				n.typ = dest.typ
				if sym != nil {
					sym.typ = n.typ
					sym.recv = src.recv
				}

				n.level = level

				if n.anc.kind == constDecl {
					n.gen = nop
					n.findex = notInFrame
					if sym, _, ok := sc.lookup(dest.ident); ok {
						sym.kind = constSym
					}
					if childPos(n) == len(n.anc.child)-1 {
						sc.iota = 0
					} else {
						sc.iota++
					}
				}
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
			switch lc := n.child[l]; lc.kind {
			case callExpr:
				if n.child[l-1].isType(sc) {
					l--
				}
				if r := lc.child[0].typ.numOut(); r != l {
					err = n.cfgErrorf("assignment mismatch: %d variables but %s returns %d values", l, lc.child[0].name(), r)
				}
				n.gen = nop
			case indexExpr:
				lc.gen = getIndexMap2
				n.gen = nop
			case typeAssertExpr:
				if n.child[0].ident == "_" {
					lc.gen = typeAssertStatus
				} else {
					lc.gen = typeAssertLong
				}
				n.gen = nop
			case unaryExpr:
				if lc.action == aRecv {
					lc.gen = recv2
					n.gen = nop
				}
			}

		case defineXStmt:
			wireChild(n)
			if sc.def == nil {
				// In global scope, type definition already handled by GTA.
				break
			}
			err = compDefineX(sc, n)

		case binaryExpr:
			wireChild(n)
			nilSym := interp.universe.sym[nilIdent]
			c0, c1 := n.child[0], n.child[1]

			err = check.binaryExpr(n)
			if err != nil {
				break
			}

			switch n.action {
			case aRem:
				n.typ = c0.typ
			case aShl, aShr:
				if c0.typ.untyped {
					break
				}
				n.typ = c0.typ
			case aEqual, aNotEqual:
				n.typ = sc.getType("bool")
				if c0.sym == nilSym || c1.sym == nilSym {
					if n.action == aEqual {
						n.gen = isNil
					} else {
						n.gen = isNotNil
					}
				}
			case aGreater, aGreaterEqual, aLower, aLowerEqual:
				n.typ = sc.getType("bool")
			}
			if err != nil {
				break
			}
			if n.typ == nil {
				if n.typ, err = nodeType(interp, sc, n); err != nil {
					break
				}
			}
			if c0.rval.IsValid() && c1.rval.IsValid() && (!isInterface(n.typ)) && constOp[n.action] != nil {
				n.typ.TypeOf()       // Force compute of reflection type.
				constOp[n.action](n) // Compute a constant result now rather than during exec.
			}
			switch {
			case n.rval.IsValid():
				// This operation involved constants, and the result is already computed
				// by constOp and available in n.rval. Nothing else to do at execution.
				n.gen = nop
				n.findex = notInFrame
			case n.anc.kind == assignStmt && n.anc.action == aAssign && n.anc.nleft == 1:
				// To avoid a copy in frame, if the result is to be assigned, store it directly
				// at the frame location of destination.
				dest := n.anc.child[childPos(n)-n.anc.nright]
				n.typ = dest.typ
				n.findex = dest.findex
				n.level = dest.level
			case n.anc.kind == returnStmt:
				// To avoid a copy in frame, if the result is to be returned, store it directly
				// at the frame location reserved for output arguments.
				pos := childPos(n)
				n.typ = sc.def.typ.ret[pos]
				n.findex = pos
			default:
				// Allocate a new location in frame, and store the result here.
				n.findex = sc.add(n.typ)
			}

		case indexExpr:
			wireChild(n)
			t := n.child[0].typ
			switch t.cat {
			case aliasT:
				if isString(t.val.TypeOf()) {
					n.typ = sc.getType("byte")
					break
				}
				fallthrough
			case ptrT:
				n.typ = t.val
				if t.val.cat == valueT {
					n.typ = valueTOf(t.val.rtype.Elem())
				} else {
					n.typ = t.val.val
				}
			case stringT:
				n.typ = sc.getType("byte")
			case valueT:
				if t.rtype.Kind() == reflect.String {
					n.typ = sc.getType("byte")
				} else {
					n.typ = valueTOf(t.rtype.Elem())
				}
			default:
				n.typ = t.val
			}
			n.findex = sc.add(n.typ)
			typ := t.TypeOf()
			if typ.Kind() == reflect.Map {
				err = check.assignment(n.child[1], t.key, "map index")
				n.gen = getIndexMap
				break
			}

			l := -1
			switch k := typ.Kind(); k {
			case reflect.Array:
				l = typ.Len()
				fallthrough
			case reflect.Slice, reflect.String:
				n.gen = getIndexArray
			case reflect.Ptr:
				if typ2 := typ.Elem(); typ2.Kind() == reflect.Array {
					l = typ2.Len()
					n.gen = getIndexArray
				} else {
					err = n.cfgErrorf("type %v does not support indexing", typ)
				}
			default:
				err = n.cfgErrorf("type is not an array, slice, string or map: %v", t.id())
			}

			err = check.index(n.child[1], l)

		case blockStmt:
			wireChild(n)
			if len(n.child) > 0 {
				l := n.lastChild()
				n.findex = l.findex
				n.level = l.level
				n.val = l.val
				n.sym = l.sym
				n.typ = l.typ
				n.rval = l.rval
			}
			sc = sc.pop()

		case constDecl:
			wireChild(n)

		case varDecl:
			// Global varDecl do not need to be wired as this
			// will be handled after cfg.
			if n.anc.kind == fileStmt {
				break
			}
			wireChild(n)

		case declStmt, exprStmt, sendStmt:
			wireChild(n)
			l := n.lastChild()
			n.findex = l.findex
			n.level = l.level
			n.val = l.val
			n.sym = l.sym
			n.typ = l.typ
			n.rval = l.rval

		case breakStmt:
			if len(n.child) > 0 {
				gotoLabel(n.sym)
			} else {
				n.tnext = sc.loop
			}

		case continueStmt:
			if len(n.child) > 0 {
				gotoLabel(n.sym)
			} else {
				n.tnext = sc.loopRestart
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
			case isBuiltinCall(n, sc):
				c0 := n.child[0]
				bname := c0.ident
				err = check.builtin(bname, n, n.child[1:], n.action == aCallSlice)
				if err != nil {
					break
				}

				n.gen = c0.sym.builtin
				c0.typ = &itype{cat: builtinT}
				if n.typ, err = nodeType(interp, sc, n); err != nil {
					return
				}
				switch {
				case n.typ.cat == builtinT:
					n.findex = notInFrame
					n.val = nil
				case n.anc.kind == returnStmt:
					// Store result directly to frame output location, to avoid a frame copy.
					n.findex = 0
				case bname == "cap" && isInConstOrTypeDecl(n):
					switch n.child[1].typ.TypeOf().Kind() {
					case reflect.Array, reflect.Chan:
						capConst(n)
					default:
						err = n.cfgErrorf("cap argument is not an array or channel")
					}
					n.findex = notInFrame
					n.gen = nop
				case bname == "len" && isInConstOrTypeDecl(n):
					switch n.child[1].typ.TypeOf().Kind() {
					case reflect.Array, reflect.Chan, reflect.String:
						lenConst(n)
					default:
						err = n.cfgErrorf("len argument is not an array, channel or string")
					}
					n.findex = notInFrame
					n.gen = nop
				default:
					n.findex = sc.add(n.typ)
				}
				if op, ok := constBltn[bname]; ok && n.anc.action != aAssign {
					op(n) // pre-compute non-assigned constant :
				}
			case n.child[0].isType(sc):
				// Type conversion expression
				c0, c1 := n.child[0], n.child[1]
				switch len(n.child) {
				case 1:
					err = n.cfgErrorf("missing argument in conversion to %s", c0.typ.id())
				case 2:
					err = check.conversion(c1, c0.typ)
				default:
					err = n.cfgErrorf("too many arguments in conversion to %s", c0.typ.id())
				}
				if err != nil {
					break
				}

				n.action = aConvert
				switch {
				case isInterface(c0.typ) && !c1.isNil():
					// Convert to interface: just check that all required methods are defined by concrete type.
					if !c1.typ.implements(c0.typ) {
						err = n.cfgErrorf("type %v does not implement interface %v", c1.typ.id(), c0.typ.id())
					}
					// Convert type to interface while keeping a reference to the original concrete type.
					// besides type, the node value remains preserved.
					n.gen = nop
					t := *c0.typ
					n.typ = &t
					n.typ.val = c1.typ
					n.findex = c1.findex
					n.level = c1.level
					n.val = c1.val
					n.rval = c1.rval
				case c1.rval.IsValid() && isConstType(c0.typ):
					n.gen = nop
					n.findex = notInFrame
					n.typ = c0.typ
					if c, ok := c1.rval.Interface().(constant.Value); ok {
						i, _ := constant.Int64Val(constant.ToInt(c))
						n.rval = reflect.ValueOf(i).Convert(c0.typ.rtype)
					} else {
						n.rval = c1.rval.Convert(c0.typ.rtype)
					}
				default:
					n.gen = convert
					n.typ = c0.typ
					n.findex = sc.add(n.typ)
				}
			case isBinCall(n):
				err = check.arguments(n, n.child[1:], n.child[0], n.action == aCallSlice)
				if err != nil {
					break
				}

				n.gen = callBin
				typ := n.child[0].typ.rtype
				if typ.NumOut() > 0 {
					if funcType := n.child[0].typ.val; funcType != nil {
						// Use the original unwrapped function type, to allow future field and
						// methods resolutions, otherwise impossible on the opaque bin type.
						n.typ = funcType.ret[0]
						n.findex = sc.add(n.typ)
						for i := 1; i < len(funcType.ret); i++ {
							sc.add(funcType.ret[i])
						}
					} else {
						n.typ = valueTOf(typ.Out(0))
						if n.anc.kind == returnStmt {
							n.findex = childPos(n)
						} else {
							n.findex = sc.add(n.typ)
							for i := 1; i < typ.NumOut(); i++ {
								sc.add(valueTOf(typ.Out(i)))
							}
						}
					}
				}
			case isOffsetof(n):
				if len(n.child) != 2 || n.child[1].kind != selectorExpr || !isStruct(n.child[1].child[0].typ) {
					err = n.cfgErrorf("Offsetof argument: invalid expression")
					break
				}
				c1 := n.child[1]
				field, ok := c1.child[0].typ.rtype.FieldByName(c1.child[1].ident)
				if !ok {
					err = n.cfgErrorf("struct does not contain field: %s", c1.child[1].ident)
					break
				}
				n.typ = valueTOf(reflect.TypeOf(field.Offset))
				n.rval = reflect.ValueOf(field.Offset)
				n.gen = nop
			default:
				err = check.arguments(n, n.child[1:], n.child[0], n.action == aCallSlice)
				if err != nil {
					break
				}

				if n.child[0].action == aGetFunc {
					// Allocate a frame entry to store the anonymous function definition.
					sc.add(n.child[0].typ)
				}
				if typ := n.child[0].typ; len(typ.ret) > 0 {
					n.typ = typ.ret[0]
					if n.anc.kind == returnStmt && n.typ.id() == sc.def.typ.ret[0].id() {
						// Store the result directly to the return value area of frame.
						// It can be done only if no type conversion at return is involved.
						n.findex = childPos(n)
					} else {
						n.findex = sc.add(n.typ)
						for _, t := range typ.ret[1:] {
							sc.add(t)
						}
					}
				} else {
					n.findex = notInFrame
				}
			}

		case caseBody:
			wireChild(n)
			switch {
			case typeSwichAssign(n) && len(n.child) > 1:
				n.start = n.child[1].start
			case len(n.child) == 0:
				// Empty case body: jump to switch node (exit node).
				n.start = n.anc.anc.anc
			default:
				n.start = n.child[0].start
			}

		case caseClause:
			sc = sc.pop()

		case commClauseDefault:
			wireChild(n)
			sc = sc.pop()
			if len(n.child) == 0 {
				return
			}
			n.start = n.child[0].start
			n.lastChild().tnext = n.anc.anc // exit node is selectStmt

		case commClause:
			wireChild(n)
			sc = sc.pop()
			if len(n.child) == 0 {
				return
			}
			if len(n.child) > 1 {
				n.start = n.child[1].start // Skip chan operation, performed by select
			}
			n.lastChild().tnext = n.anc.anc // exit node is selectStmt

		case compositeLitExpr:
			wireChild(n)

			child := n.child
			if n.nleft > 0 {
				child = child[1:]
			}

			switch n.typ.cat {
			case arrayT, sliceT:
				err = check.arrayLitExpr(child, n.typ)
			case mapT:
				err = check.mapLitExpr(child, n.typ.key, n.typ.val)
			case structT:
				err = check.structLitExpr(child, n.typ)
			case valueT:
				rtype := n.typ.rtype
				switch rtype.Kind() {
				case reflect.Struct:
					err = check.structBinLitExpr(child, rtype)
				case reflect.Map:
					ktyp := valueTOf(rtype.Key())
					vtyp := valueTOf(rtype.Elem())
					err = check.mapLitExpr(child, ktyp, vtyp)
				}
			}
			if err != nil {
				break
			}

			n.findex = sc.add(n.typ)
			// TODO: Check that composite literal expr matches corresponding type
			n.gen = compositeGenerator(n, n.typ, nil)

		case fallthroughtStmt:
			if n.anc.kind != caseBody {
				err = n.cfgErrorf("fallthrough statement out of place")
			}

		case fileStmt:
			wireChild(n, varDecl)
			sc = sc.pop()
			n.findex = notInFrame

		case forStmt0: // for {}
			body := n.child[0]
			n.start = body.start
			body.tnext = n.start
			sc = sc.pop()

		case forStmt1: // for init; ; {}
			init, body := n.child[0], n.child[1]
			n.start = init.start
			init.tnext = body.start
			body.tnext = n.start
			sc = sc.pop()

		case forStmt2: // for cond {}
			cond, body := n.child[0], n.child[1]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as for condition")
			}
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test.
				if cond.rval.Bool() {
					n.start = body.start
					body.tnext = body.start
				}
			} else {
				n.start = cond.start
				cond.tnext = body.start
				body.tnext = cond.start
			}
			setFNext(cond, n)
			sc = sc.pop()

		case forStmt3: // for init; cond; {}
			init, cond, body := n.child[0], n.child[1], n.child[2]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as for condition")
			}
			n.start = init.start
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test.
				if cond.rval.Bool() {
					init.tnext = body.start
					body.tnext = body.start
				} else {
					init.tnext = n
				}
			} else {
				init.tnext = cond.start
				body.tnext = cond.start
			}
			cond.tnext = body.start
			setFNext(cond, n)
			sc = sc.pop()

		case forStmt4: // for ; ; post {}
			post, body := n.child[0], n.child[1]
			n.start = body.start
			post.tnext = body.start
			body.tnext = post.start
			sc = sc.pop()

		case forStmt5: // for ; cond; post {}
			cond, post, body := n.child[0], n.child[1], n.child[2]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as for condition")
			}
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test.
				if cond.rval.Bool() {
					n.start = body.start
					post.tnext = body.start
				}
			} else {
				n.start = cond.start
				post.tnext = cond.start
			}
			cond.tnext = body.start
			setFNext(cond, n)
			body.tnext = post.start
			sc = sc.pop()

		case forStmt6: // for init; ; post {}
			init, post, body := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			init.tnext = body.start
			body.tnext = post.start
			post.tnext = body.start
			sc = sc.pop()

		case forStmt7: // for init; cond; post {}
			init, cond, post, body := n.child[0], n.child[1], n.child[2], n.child[3]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as for condition")
			}
			n.start = init.start
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test.
				if cond.rval.Bool() {
					init.tnext = body.start
					post.tnext = body.start
				} else {
					init.tnext = n
				}
			} else {
				init.tnext = cond.start
				post.tnext = cond.start
			}
			cond.tnext = body.start
			setFNext(cond, n)
			body.tnext = post.start
			sc = sc.pop()

		case forRangeStmt:
			n.start = n.child[0].start
			setFNext(n.child[0], n)
			sc = sc.pop()

		case funcDecl:
			n.start = n.child[3].start
			n.types, n.scope = sc.types, sc
			sc = sc.pop()
			funcName := n.child[1].ident
			if sym := sc.sym[funcName]; !isMethod(n) && sym != nil {
				sym.index = -1 // to force value to n.val
				sym.typ = n.typ
				sym.kind = funcSym
				sym.node = n
			}

		case funcLit:
			n.types, n.scope = sc.types, sc
			sc = sc.pop()
			err = genRun(n)

		case deferStmt, goStmt:
			wireChild(n)

		case identExpr:
			if isKey(n) || isNewDefine(n, sc) {
				break
			}
			if n.anc.kind == funcDecl && n.anc.child[1] == n {
				// Dont process a function name identExpr.
				break
			}

			sym, level, found := sc.lookup(n.ident)
			if !found {
				// retry with the filename, in case ident is a package name.
				sym, level, found = sc.lookup(filepath.Join(n.ident, baseName))
				if !found {
					err = n.cfgErrorf("undefined: %s", n.ident)
					break
				}
			}
			// Found symbol, populate node info
			n.sym, n.typ, n.findex, n.level = sym, sym.typ, sym.index, level
			if n.findex < 0 {
				n.val = sym.node
			} else {
				switch {
				case sym.kind == constSym && sym.rval.IsValid():
					n.rval = sym.rval
					n.kind = basicLit
				case n.ident == "iota":
					n.rval = reflect.ValueOf(constant.MakeInt64(int64(sc.iota)))
					n.kind = basicLit
				case n.ident == nilIdent:
					n.kind = basicLit
				case sym.kind == binSym:
					n.typ = sym.typ
					n.rval = sym.rval
				case sym.kind == bltnSym:
					if n.anc.kind != callExpr {
						err = n.cfgErrorf("use of builtin %s not in function call", n.ident)
					}
				}
			}
			if n.sym != nil {
				n.recv = n.sym.recv
			}

		case ifStmt0: // if cond {}
			cond, tbody := n.child[0], n.child[1]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as if condition")
			}
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test.
				if cond.rval.Bool() {
					n.start = tbody.start
				}
			} else {
				n.start = cond.start
				cond.tnext = tbody.start
			}
			setFNext(cond, n)
			tbody.tnext = n
			sc = sc.pop()

		case ifStmt1: // if cond {} else {}
			cond, tbody, fbody := n.child[0], n.child[1], n.child[2]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as if condition")
			}
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test and the useless branch.
				if cond.rval.Bool() {
					n.start = tbody.start
				} else {
					n.start = fbody.start
				}
			} else {
				n.start = cond.start
				cond.tnext = tbody.start
				setFNext(cond, fbody.start)
			}
			tbody.tnext = n
			fbody.tnext = n
			sc = sc.pop()

		case ifStmt2: // if init; cond {}
			init, cond, tbody := n.child[0], n.child[1], n.child[2]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as if condition")
			}
			n.start = init.start
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test.
				if cond.rval.Bool() {
					init.tnext = tbody.start
				} else {
					init.tnext = n
				}
			} else {
				init.tnext = cond.start
				cond.tnext = tbody.start
			}
			tbody.tnext = n
			setFNext(cond, n)
			sc = sc.pop()

		case ifStmt3: // if init; cond {} else {}
			init, cond, tbody, fbody := n.child[0], n.child[1], n.child[2], n.child[3]
			if !isBool(cond.typ) {
				err = cond.cfgErrorf("non-bool used as if condition")
			}
			n.start = init.start
			if cond.rval.IsValid() {
				// Condition is known at compile time, bypass test.
				if cond.rval.Bool() {
					init.tnext = tbody.start
				} else {
					init.tnext = fbody.start
				}
			} else {
				init.tnext = cond.start
				cond.tnext = tbody.start
				setFNext(cond, fbody.start)
			}
			tbody.tnext = n
			fbody.tnext = n
			sc = sc.pop()

		case keyValueExpr:
			wireChild(n)

		case landExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n.child[1].start
			setFNext(n.child[0], n)
			n.child[1].tnext = n
			n.typ = n.child[0].typ
			n.findex = sc.add(n.typ)
			if n.start.action == aNop {
				n.start.gen = branch
			}

		case lorExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n
			setFNext(n.child[0], n.child[1].start)
			n.child[1].tnext = n
			n.typ = n.child[0].typ
			n.findex = sc.add(n.typ)
			if n.start.action == aNop {
				n.start.gen = branch
			}

		case parenExpr:
			wireChild(n)
			c := n.lastChild()
			n.findex = c.findex
			n.level = c.level
			n.typ = c.typ
			n.rval = c.rval

		case rangeStmt:
			if sc.rangeChanType(n) != nil {
				n.start = n.child[1].start // Get chan
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
				n.start = o.start    // Get array or map object
				o.tnext = k.start    // then go to iterator init
				k.tnext = n          // then go to range function
				n.tnext = body.start // then go to range body
				body.tnext = n       // then body go to range function (loop)
				k.gen = empty        // init filled later by generator
			}

		case returnStmt:
			if len(n.child) > sc.def.typ.numOut() {
				err = n.cfgErrorf("too many arguments to return")
				break
			}
			returnSig := sc.def.child[2]
			if mustReturnValue(returnSig) {
				nret := len(n.child)
				if nret == 1 && isCall(n.child[0]) {
					nret = n.child[0].child[0].typ.numOut()
				}
				if nret < sc.def.typ.numOut() {
					err = n.cfgErrorf("not enough arguments to return")
					break
				}
			}
			wireChild(n)
			n.tnext = nil
			n.val = sc.def
			for i, c := range n.child {
				var typ *itype
				typ, err = nodeType(interp, sc.upperLevel(), returnSig.child[1].fieldType(i))
				if err != nil {
					return
				}
				// TODO(mpl): move any of that code to typecheck?
				c.typ.node = c
				if !c.typ.assignableTo(typ) {
					err = c.cfgErrorf("cannot use %v (type %v) as type %v in return argument", c.ident, c.typ.cat, typ.cat)
					return
				}
				if c.typ.cat == nilT {
					// nil: Set node value to zero of return type
					if typ.cat == funcT {
						// Wrap the typed nil value in a node, as per other interpreter functions
						c.rval = reflect.ValueOf(&node{kind: basicLit, rval: reflect.New(typ.TypeOf()).Elem()})
					} else {
						c.rval = reflect.New(typ.TypeOf()).Elem()
					}
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
			switch {
			case n.typ.cat == binPkgT:
				// Resolve binary package symbol: a type or a value
				name := n.child[1].ident
				pkg := n.child[0].sym.typ.path
				if s, ok := interp.binPkg[pkg][name]; ok {
					if isBinType(s) {
						n.typ = valueTOf(s.Type().Elem())
					} else {
						n.typ = valueTOf(fixPossibleConstType(s.Type()), withUntyped(isValueUntyped(s)))
						n.rval = s
					}
					n.action = aGetSym
					n.gen = nop
				} else {
					err = n.cfgErrorf("package %s \"%s\" has no symbol %s", n.child[0].ident, pkg, name)
				}
			case n.typ.cat == srcPkgT:
				pkg, name := n.child[0].sym.typ.path, n.child[1].ident
				// Resolve source package symbol
				if sym, ok := interp.srcPkg[pkg][name]; ok {
					n.findex = sym.index
					if sym.global {
						n.level = globalFrame
					}
					n.val = sym.node
					n.gen = nop
					n.action = aGetSym
					n.typ = sym.typ
					n.sym = sym
					n.recv = sym.recv
					n.rval = sym.rval
				} else {
					err = n.cfgErrorf("undefined selector: %s.%s", pkg, name)
				}
			case isStruct(n.typ) || isInterfaceSrc(n.typ):
				// Find a matching field.
				if ti := n.typ.lookupField(n.child[1].ident); len(ti) > 0 {
					if isStruct(n.typ) {
						// If a method of the same name exists, use it if it is shallower than the struct field.
						// if method's depth is the same as field's, this is an error.
						d := n.typ.methodDepth(n.child[1].ident)
						if d >= 0 && d < len(ti) {
							goto tryMethods
						}
						if d == len(ti) {
							err = n.cfgErrorf("ambiguous selector: %s", n.child[1].ident)
							break
						}
					}
					n.val = ti
					switch {
					case isInterfaceSrc(n.typ):
						n.typ = n.typ.fieldSeq(ti)
						n.gen = getMethodByName
						n.action = aMethod
					case n.typ.cat == ptrT:
						n.typ = n.typ.fieldSeq(ti)
						n.gen = getPtrIndexSeq
						if n.typ.cat == funcT {
							// Function in a struct field is always wrapped in reflect.Value.
							n.typ = wrapperValueTOf(n.typ.TypeOf(), n.typ)
						}
					default:
						n.gen = getIndexSeq
						n.typ = n.typ.fieldSeq(ti)
						if n.typ.cat == funcT {
							// Function in a struct field is always wrapped in reflect.Value.
							n.typ = wrapperValueTOf(n.typ.TypeOf(), n.typ)
						}
					}
					break
				}
				if s, lind, ok := n.typ.lookupBinField(n.child[1].ident); ok {
					// Handle an embedded binary field into a struct field.
					n.gen = getIndexSeqField
					lind = append(lind, s.Index...)
					if isStruct(n.typ) {
						// If a method of the same name exists, use it if it is shallower than the struct field.
						// if method's depth is the same as field's, this is an error.
						d := n.typ.methodDepth(n.child[1].ident)
						if d >= 0 && d < len(lind) {
							goto tryMethods
						}
						if d == len(lind) {
							err = n.cfgErrorf("ambiguous selector: %s", n.child[1].ident)
							break
						}
					}
					n.val = lind
					n.typ = valueTOf(s.Type)
					break
				}
				// No field (embedded or not) matched. Try to match a method.
			tryMethods:
				fallthrough
			default:
				// Find a matching method.
				// TODO (marc): simplify the following if/elseif blocks.
				if n.typ.cat == valueT || n.typ.cat == errorT {
					switch method, ok := n.typ.rtype.MethodByName(n.child[1].ident); {
					case ok:
						hasRecvType := n.typ.rtype.Kind() != reflect.Interface
						n.val = method.Index
						n.gen = getIndexBinMethod
						n.action = aGetMethod
						n.recv = &receiver{node: n.child[0]}
						n.typ = valueTOf(method.Type, isBinMethod())
						if hasRecvType {
							n.typ.recv = n.typ
						}
					case n.typ.rtype.Kind() == reflect.Ptr:
						if field, ok := n.typ.rtype.Elem().FieldByName(n.child[1].ident); ok {
							n.typ = valueTOf(field.Type)
							n.val = field.Index
							n.gen = getPtrIndexSeq
							break
						}
						err = n.cfgErrorf("undefined field or method: %s", n.child[1].ident)
					case n.typ.rtype.Kind() == reflect.Struct:
						if field, ok := n.typ.rtype.FieldByName(n.child[1].ident); ok {
							n.typ = valueTOf(field.Type)
							n.val = field.Index
							n.gen = getIndexSeq
							break
						}
						fallthrough
					default:
						// method lookup failed on type, now lookup on pointer to type
						pt := reflect.PtrTo(n.typ.rtype)
						if m2, ok2 := pt.MethodByName(n.child[1].ident); ok2 {
							n.val = m2.Index
							n.gen = getIndexBinPtrMethod
							n.typ = valueTOf(m2.Type, isBinMethod(), withRecv(valueTOf(pt)))
							n.recv = &receiver{node: n.child[0]}
							n.action = aGetMethod
							break
						}
						err = n.cfgErrorf("undefined field or method: %s", n.child[1].ident)
					}
				} else if n.typ.cat == ptrT && (n.typ.val.cat == valueT || n.typ.val.cat == errorT) {
					// Handle pointer on object defined in runtime
					if method, ok := n.typ.val.rtype.MethodByName(n.child[1].ident); ok {
						n.val = method.Index
						n.typ = valueTOf(method.Type, isBinMethod(), withRecv(n.typ))
						n.recv = &receiver{node: n.child[0]}
						n.gen = getIndexBinElemMethod
						n.action = aGetMethod
					} else if method, ok := reflect.PtrTo(n.typ.val.rtype).MethodByName(n.child[1].ident); ok {
						n.val = method.Index
						n.gen = getIndexBinMethod
						n.typ = valueTOf(method.Type, withRecv(valueTOf(reflect.PtrTo(n.typ.val.rtype), isBinMethod())))
						n.recv = &receiver{node: n.child[0]}
						n.action = aGetMethod
					} else if field, ok := n.typ.val.rtype.FieldByName(n.child[1].ident); ok {
						n.typ = valueTOf(field.Type)
						n.val = field.Index
						n.gen = getPtrIndexSeq
					} else {
						err = n.cfgErrorf("undefined selector: %s", n.child[1].ident)
					}
				} else if m, lind := n.typ.lookupMethod(n.child[1].ident); m != nil {
					n.action = aGetMethod
					if n.child[0].isType(sc) {
						// Handle method as a function with receiver in 1st argument
						n.val = m
						n.findex = notInFrame
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
				} else if m, lind, isPtr, ok := n.typ.lookupBinMethod(n.child[1].ident); ok {
					n.action = aGetMethod
					switch {
					case isPtr && n.typ.fieldSeq(lind).cat != ptrT:
						n.gen = getIndexSeqPtrMethod
					case isInterfaceSrc(n.typ):
						n.gen = getMethodByName
					default:
						n.gen = getIndexSeqMethod
					}
					n.recv = &receiver{node: n.child[0], index: lind}
					n.val = append([]int{m.Index}, lind...)
					n.typ = valueTOf(m.Type, isBinMethod(), withRecv(n.child[0].typ))
				} else {
					err = n.cfgErrorf("undefined selector: %s", n.child[1].ident)
				}
			}
			if err == nil && n.findex != -1 {
				n.findex = sc.add(n.typ)
			}

		case selectStmt:
			wireChild(n)
			// Move action to block statement, so select node can be an exit point.
			n.child[0].gen = _select
			// Chain channel init actions in commClauses prior to invoke select.
			var cur *node
			for _, c := range n.child[0].child {
				var an, pn *node // channel init action nodes
				if len(c.child) > 0 {
					switch c0 := c.child[0]; {
					case c0.kind == exprStmt && len(c0.child) == 1 && c0.child[0].action == aRecv:
						an = c0.child[0].child[0]
						pn = an
					case c0.action == aAssign:
						an = c0.lastChild().child[0]
						pn = an
					case c0.kind == sendStmt:
						an = c0.child[0]
						pn = c0.child[1]
					}
				}
				if an == nil {
					continue
				}
				if cur == nil {
					// First channel init action, the entry point for the select block.
					n.start = an.start
				} else {
					// Chain channel init action to the previous one.
					cur.tnext = an.start
				}
				if pn != nil {
					// Chain channect init action to send data init action.
					// (already done by wireChild, but let's be explicit).
					an.tnext = pn
					cur = pn
				}
			}
			if cur == nil {
				// There is no channel init action, call select directly.
				n.start = n.child[0]
			} else {
				// Select is called after the last channel init action.
				cur.tnext = n.child[0]
			}

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
				n.typ = ptrOf(n.child[0].typ)
			default:
				// dereference expression
				wireChild(n)

				err = check.starExpr(n.child[0])
				if err != nil {
					break
				}

				if c0 := n.child[0]; c0.typ.cat == valueT {
					n.typ = valueTOf(c0.typ.rtype.Elem())
				} else {
					n.typ = c0.typ.val
				}
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
			sc = sc.pop()
			sbn := n.lastChild() // switch block node
			clauses := sbn.child
			l := len(clauses)
			if l == 0 {
				// Switch is empty
				break
			}
			// Chain case clauses.
			for i := l - 1; i >= 0; i-- {
				c := clauses[i]
				if len(c.child) == 0 {
					c.tnext = n // Clause body is empty, exit.
				} else {
					body := c.lastChild()
					c.tnext = body.start
					c.child[0].tnext = c
					c.start = c.child[0].start

					if i < l-1 && len(body.child) > 0 && body.lastChild().kind == fallthroughtStmt {
						if n.kind == typeSwitch {
							err = body.lastChild().cfgErrorf("cannot fallthrough in type switch")
						}
						if len(clauses[i+1].child) == 0 {
							body.tnext = n // Fallthrough to next with empty body, just exit.
						} else {
							body.tnext = clauses[i+1].lastChild().start
						}
					} else {
						body.tnext = n // Exit switch at end of clause body.
					}
				}

				if i == l-1 {
					setFNext(clauses[i], n)
					continue
				}
				if len(clauses[i+1].child) > 1 {
					setFNext(c, clauses[i+1].start)
				} else {
					setFNext(c, clauses[i+1])
				}
			}
			n.start = n.child[0].start
			n.child[0].tnext = sbn.start

		case switchIfStmt: // like an if-else chain
			sc = sc.pop()
			sbn := n.lastChild() // switch block node
			clauses := sbn.child
			l := len(clauses)
			if l == 0 {
				// Switch is empty
				break
			}
			// Wire case clauses in reverse order so the next start node is already resolved when used.
			for i := l - 1; i >= 0; i-- {
				c := clauses[i]
				c.gen = nop
				if len(c.child) == 0 {
					c.tnext = n
					c.fnext = n
				} else {
					body := c.lastChild()
					if len(c.child) > 1 {
						cond := c.child[0]
						cond.tnext = body.start
						if i == l-1 {
							setFNext(cond, n)
						} else {
							setFNext(cond, clauses[i+1].start)
						}
						c.start = cond.start
					} else {
						c.start = body.start
					}
					// If last case body statement is a fallthrough, then jump to next case body
					if i < l-1 && len(body.child) > 0 && body.lastChild().kind == fallthroughtStmt {
						body.tnext = clauses[i+1].lastChild().start
					} else {
						body.tnext = n
					}
				}
			}
			sbn.start = clauses[0].start
			n.start = n.child[0].start
			n.child[0].tnext = sbn.start

		case typeAssertExpr:
			if len(n.child) == 1 {
				// The "o.(type)" is handled by typeSwitch.
				n.gen = nop
				break
			}

			wireChild(n)
			c0, c1 := n.child[0], n.child[1]
			if c1.typ == nil {
				if c1.typ, err = nodeType(interp, sc, c1); err != nil {
					return
				}
			}

			err = check.typeAssertionExpr(c0, c1.typ)
			if err != nil {
				break
			}

			if n.anc.action != aAssignX {
				if c0.typ.cat == valueT && isFunc(c1.typ) {
					// Avoid special wrapping of interfaces and func types.
					n.typ = valueTOf(c1.typ.TypeOf())
				} else {
					n.typ = c1.typ
				}
				n.findex = sc.add(n.typ)
			}

		case sliceExpr:
			wireChild(n)

			err = check.sliceExpr(n)
			if err != nil {
				break
			}

			if n.typ, err = nodeType(interp, sc, n); err != nil {
				return
			}
			n.findex = sc.add(n.typ)

		case unaryExpr:
			wireChild(n)

			err = check.unaryExpr(n)
			if err != nil {
				break
			}

			n.typ = n.child[0].typ
			if n.action == aRecv {
				// Channel receive operation: set type to the channel data type
				if n.typ.cat == valueT {
					n.typ = valueTOf(n.typ.rtype.Elem())
				} else {
					n.typ = n.typ.val
				}
			}
			if n.typ == nil {
				if n.typ, err = nodeType(interp, sc, n); err != nil {
					return
				}
			}

			// TODO: Optimisation: avoid allocation if boolean branch op (i.e. '!' in an 'if' expr)
			if n.child[0].rval.IsValid() && !isInterface(n.typ) && constOp[n.action] != nil {
				n.typ.TypeOf() // init reflect type
				constOp[n.action](n)
			}
			switch {
			case n.rval.IsValid():
				n.gen = nop
				n.findex = notInFrame
			case n.anc.kind == assignStmt && n.anc.action == aAssign && n.anc.nright == 1:
				dest := n.anc.child[childPos(n)-n.anc.nright]
				n.typ = dest.typ
				n.findex = dest.findex
				n.level = dest.level
			case n.anc.kind == returnStmt:
				pos := childPos(n)
				n.typ = sc.def.typ.ret[pos]
				n.findex = pos
			default:
				n.findex = sc.add(n.typ)
			}

		case valueSpec:
			n.gen = reset
			l := len(n.child) - 1
			if n.typ = n.child[l].typ; n.typ == nil {
				if n.typ, err = nodeType(interp, sc, n.child[l]); err != nil {
					return
				}
			}

			for _, c := range n.child[:l] {
				var index int
				if sc.global {
					// Global object allocation is already performed in GTA.
					index = sc.sym[c.ident].index
					c.level = globalFrame
				} else {
					index = sc.add(n.typ)
					sc.sym[c.ident] = &symbol{index: index, kind: varSym, typ: n.typ}
				}
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

func compDefineX(sc *scope, n *node) error {
	l := len(n.child) - 1
	types := []*itype{}

	switch src := n.child[l]; src.kind {
	case callExpr:
		funtype, err := nodeType(n.interp, sc, src.child[0])
		if err != nil {
			return err
		}
		for funtype.cat == valueT && funtype.val != nil {
			// Retrieve original interpreter type from a wrapped function.
			// Struct fields of function types are always wrapped in valueT to ensure
			// their possible use in runtime. In that case, the val field retains the
			// original interpreter type, which is used now.
			funtype = funtype.val
		}
		if funtype.cat == valueT {
			// Handle functions imported from runtime.
			for i := 0; i < funtype.rtype.NumOut(); i++ {
				types = append(types, valueTOf(funtype.rtype.Out(i)))
			}
		} else {
			types = funtype.ret
		}
		if n.child[l-1].isType(sc) {
			l--
		}
		if len(types) != l {
			return n.cfgErrorf("assignment mismatch: %d variables but %s returns %d values", l, src.child[0].name(), len(types))
		}
		n.gen = nop

	case indexExpr:
		types = append(types, src.typ, sc.getType("bool"))
		n.child[l].gen = getIndexMap2
		n.gen = nop

	case typeAssertExpr:
		if n.child[0].ident == "_" {
			n.child[l].gen = typeAssertStatus
		} else {
			n.child[l].gen = typeAssertLong
		}
		types = append(types, n.child[l].child[1].typ, sc.getType("bool"))
		n.gen = nop

	case unaryExpr:
		if n.child[l].action == aRecv {
			types = append(types, src.typ, sc.getType("bool"))
			n.child[l].gen = recv2
			n.gen = nop
		}

	default:
		return n.cfgErrorf("unsupported assign expression")
	}

	for i, t := range types {
		index := sc.add(t)
		sc.sym[n.child[i].ident] = &symbol{index: index, kind: varSym, typ: t}
		n.child[i].typ = t
		n.child[i].findex = index
	}

	return nil
}

// TODO used for allocation optimization, temporarily disabled
// func isAncBranch(n *node) bool {
//	switch n.anc.kind {
//	case If0, If1, If2, If3:
//		return true
//	}
//	return false
// }

func childPos(n *node) int {
	for i, c := range n.anc.child {
		if n == c {
			return i
		}
	}
	return -1
}

func (n *node) cfgErrorf(format string, a ...interface{}) *cfgError {
	pos := n.interp.fset.Position(n.pos)
	posString := n.interp.fset.Position(n.pos).String()
	if pos.Filename == DefaultSourceName {
		posString = strings.TrimPrefix(posString, DefaultSourceName+":")
	}
	a = append([]interface{}{posString}, a...)
	return &cfgError{n, fmt.Errorf("%s: "+format, a...)}
}

func genRun(nod *node) error {
	var err error

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

func genGlobalVars(roots []*node, sc *scope) (*node, error) {
	var vars []*node
	for _, n := range roots {
		vars = append(vars, getVars(n)...)
	}

	if len(vars) == 0 {
		return nil, nil
	}

	varNode, err := genGlobalVarDecl(vars, sc)
	if err != nil {
		return nil, err
	}
	setExec(varNode.start)
	return varNode, nil
}

func getVars(n *node) (vars []*node) {
	for _, child := range n.child {
		if child.kind == varDecl {
			vars = append(vars, child.child...)
		}
	}
	return vars
}

func genGlobalVarDecl(nodes []*node, sc *scope) (*node, error) {
	varNode := &node{kind: varDecl, action: aNop, gen: nop}

	deps := map[*node][]*node{}
	for _, n := range nodes {
		deps[n] = getVarDependencies(n, sc)
	}

	inited := map[*node]bool{}
	revisit := []*node{}
	for {
		for _, n := range nodes {
			canInit := true
			for _, d := range deps[n] {
				if !inited[d] {
					canInit = false
				}
			}
			if !canInit {
				revisit = append(revisit, n)
				continue
			}

			varNode.child = append(varNode.child, n)
			inited[n] = true
		}

		if len(revisit) == 0 || equalNodes(nodes, revisit) {
			break
		}

		nodes = revisit
		revisit = []*node{}
	}

	if len(revisit) > 0 {
		return nil, revisit[0].cfgErrorf("variable definition loop")
	}
	wireChild(varNode)
	return varNode, nil
}

func getVarDependencies(nod *node, sc *scope) (deps []*node) {
	nod.Walk(func(n *node) bool {
		if n.kind == identExpr {
			if sym, _, ok := sc.lookup(n.ident); ok {
				if sym.kind != varSym || !sym.global || sym.node == nod {
					return false
				}
				deps = append(deps, sym.node)
			}
		}
		return true
	}, nil)
	return deps
}

// setFnext sets the cond fnext field to next, propagates it for parenthesis blocks
// and sets the action to branch.
func setFNext(cond, next *node) {
	if cond.action == aNop {
		cond.action = aBranch
		cond.gen = branch
		cond.fnext = next
	}
	if cond.kind == parenExpr {
		setFNext(cond.lastChild(), next)
		return
	}
	cond.fnext = next
}

// GetDefault return the index of default case clause in a switch statement, or -1.
func getDefault(n *node) int {
	for i, c := range n.lastChild().child {
		switch len(c.child) {
		case 0:
			return i
		case 1:
			if c.child[0].kind == caseBody {
				return i
			}
		}
	}
	return -1
}

func isBinType(v reflect.Value) bool { return v.IsValid() && v.Kind() == reflect.Ptr && v.IsNil() }

// isType returns true if node refers to a type definition, false otherwise.
func (n *node) isType(sc *scope) bool {
	switch n.kind {
	case arrayType, chanType, chanTypeRecv, chanTypeSend, funcType, interfaceType, mapType, structType:
		return true
	case parenExpr, starExpr:
		if len(n.child) == 1 {
			return n.child[0].isType(sc)
		}
	case selectorExpr:
		pkg, name := n.child[0].ident, n.child[1].ident
		baseName := filepath.Base(n.interp.fset.Position(n.pos).Filename)
		suffixedPkg := filepath.Join(pkg, baseName)
		sym, _, ok := sc.lookup(suffixedPkg)
		if !ok {
			sym, _, ok = sc.lookup(pkg)
			if !ok {
				return false
			}
		}
		if sym.kind != pkgSym {
			return false
		}
		path := sym.typ.path
		if p, ok := n.interp.binPkg[path]; ok && isBinType(p[name]) {
			return true // Imported binary type
		}
		if p, ok := n.interp.srcPkg[path]; ok && p[name] != nil && p[name].kind == typeSym {
			return true // Imported source type
		}
	case identExpr:
		return sc.getType(n.ident) != nil
	}
	return false
}

// wireChild wires AST nodes for CFG in subtree.
func wireChild(n *node, exclude ...nkind) {
	child := excludeNodeKind(n.child, exclude)

	// Set start node, in subtree (propagated to ancestors by post-order processing)
	for _, c := range child {
		switch c.kind {
		case arrayType, chanType, chanTypeRecv, chanTypeSend, funcDecl, importDecl, mapType, basicLit, identExpr, typeDecl:
			continue
		default:
			n.start = c.start
		}
		break
	}

	// Chain sequential operations inside a block (next is right sibling)
	for i := 1; i < len(child); i++ {
		switch child[i].kind {
		case funcDecl:
			child[i-1].tnext = child[i]
		default:
			switch child[i-1].kind {
			case breakStmt, continueStmt, gotoStmt, returnStmt:
				// tnext is already computed, no change
			default:
				child[i-1].tnext = child[i].start
			}
		}
	}

	// Chain subtree next to self
	for i := len(child) - 1; i >= 0; i-- {
		switch child[i].kind {
		case arrayType, chanType, chanTypeRecv, chanTypeSend, importDecl, mapType, funcDecl, basicLit, identExpr, typeDecl:
			continue
		case breakStmt, continueStmt, gotoStmt, returnStmt:
			// tnext is already computed, no change
		default:
			child[i].tnext = n
		}
		break
	}
}

func excludeNodeKind(child []*node, kinds []nkind) []*node {
	if len(kinds) == 0 {
		return child
	}
	var res []*node
	for _, c := range child {
		exclude := false
		for _, k := range kinds {
			if c.kind == k {
				exclude = true
			}
		}
		if !exclude {
			res = append(res, c)
		}
	}
	return res
}

func (n *node) name() (s string) {
	switch {
	case n.ident != "":
		s = n.ident
	case n.action == aGetSym:
		s = n.child[0].ident + "." + n.child[1].ident
	}
	return s
}

// isNatural returns true if node type is natural, false otherwise.
func (n *node) isNatural() bool {
	if isUint(n.typ.TypeOf()) {
		return true
	}
	if n.rval.IsValid() {
		t := n.rval.Type()
		if isUint(t) {
			return true
		}
		if isInt(t) && n.rval.Int() >= 0 {
			// positive untyped integer constant is ok
			return true
		}
		if isFloat(t) {
			// positive untyped float constant with null decimal part is ok
			f := n.rval.Float()
			if f == math.Trunc(f) && f >= 0 {
				n.rval = reflect.ValueOf(uint(f))
				n.typ.rtype = n.rval.Type()
				return true
			}
		}
		if isConstantValue(t) {
			c := n.rval.Interface().(constant.Value)
			switch c.Kind() {
			case constant.Int:
				i, _ := constant.Int64Val(c)
				if i >= 0 {
					return true
				}
			case constant.Float:
				f, _ := constant.Float64Val(c)
				if f == math.Trunc(f) {
					n.rval = reflect.ValueOf(constant.ToInt(c))
					n.typ.rtype = n.rval.Type()
					return true
				}
			}
		}
	}
	return false
}

// isNil returns true if node is a literal nil value, false otherwise.
func (n *node) isNil() bool { return n.kind == basicLit && !n.rval.IsValid() }

// fieldType returns the nth parameter field node (type) of a fieldList node.
func (n *node) fieldType(m int) *node {
	k := 0
	l := len(n.child)
	for i := 0; i < l; i++ {
		cl := len(n.child[i].child)
		if cl < 2 {
			if k == m {
				return n.child[i].lastChild()
			}
			k++
			continue
		}
		for j := 0; j < cl-1; j++ {
			if k == m {
				return n.child[i].lastChild()
			}
			k++
		}
	}
	return nil
}

// lastChild returns the last child of a node.
func (n *node) lastChild() *node { return n.child[len(n.child)-1] }

func isKey(n *node) bool {
	return n.anc.kind == fileStmt ||
		(n.anc.kind == selectorExpr && n.anc.child[0] != n) ||
		(n.anc.kind == funcDecl && isMethod(n.anc)) ||
		(n.anc.kind == keyValueExpr && isStruct(n.anc.typ) && n.anc.child[0] == n) ||
		(n.anc.kind == fieldExpr && len(n.anc.child) > 1 && n.anc.child[0] == n)
}

func isField(n *node) bool {
	return n.kind == selectorExpr && len(n.child) > 0 && n.child[0].typ != nil && isStruct(n.child[0].typ)
}

func isRecursiveField(n *node) bool {
	if !isField(n) {
		return false
	}
	t := n.typ
	for t != nil {
		if t.recursive {
			return true
		}
		t = t.val
	}
	return false
}

func isInConstOrTypeDecl(n *node) bool {
	anc := n.anc
	for anc != nil {
		switch anc.kind {
		case constDecl, typeDecl:
			return true
		case varDecl, funcDecl:
			return false
		}
		anc = anc.anc
	}
	return false
}

// isNewDefine returns true if node refers to a new definition.
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

func isFuncField(n *node) bool {
	return isField(n) && isFunc(n.typ)
}

func isMapEntry(n *node) bool {
	return n.action == aGetIndex && isMap(n.child[0].typ)
}

func isCall(n *node) bool {
	return n.action == aCall || n.action == aCallSlice
}

func isBinCall(n *node) bool {
	return isCall(n) && n.child[0].typ.cat == valueT && n.child[0].typ.rtype.Kind() == reflect.Func
}

func isOffsetof(n *node) bool {
	return isCall(n) && n.child[0].typ.cat == valueT && n.child[0].rval.String() == "Offsetof"
}

func mustReturnValue(n *node) bool {
	if len(n.child) < 2 {
		return false
	}
	for _, f := range n.child[1].child {
		if len(f.child) > 1 {
			return false
		}
	}
	return true
}

func isRegularCall(n *node) bool {
	return isCall(n) && n.child[0].typ.cat == funcT
}

func variadicPos(n *node) int {
	if len(n.child[0].typ.arg) == 0 {
		return -1
	}
	last := len(n.child[0].typ.arg) - 1
	if n.child[0].typ.arg[last].cat == variadicT {
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
		if c.tnext == nil {
			c.tnext = s.node.start
		}
	}
}

func compositeGenerator(n *node, typ *itype, rtyp reflect.Type) (gen bltnGenerator) {
	switch typ.cat {
	case aliasT, ptrT:
		gen = compositeGenerator(n, n.typ.val, rtyp)
	case arrayT, sliceT:
		gen = arrayLit
	case mapT:
		gen = mapLit
	case structT:
		switch {
		case len(n.child) == 0:
			gen = compositeLitNotype
		case n.lastChild().kind == keyValueExpr:
			if n.nleft == 1 {
				gen = compositeLitKeyed
			} else {
				gen = compositeLitKeyedNotype
			}
		default:
			if n.nleft == 1 {
				gen = compositeLit
			} else {
				gen = compositeLitNotype
			}
		}
	case valueT:
		if rtyp == nil {
			rtyp = n.typ.rtype
		}
		// TODO(mpl): I do not understand where this side-effect is coming from, and why it happens. quickfix for now.
		if rtyp == nil {
			rtyp = n.typ.val.rtype
		}
		switch k := rtyp.Kind(); k {
		case reflect.Struct:
			if n.nleft == 1 {
				gen = compositeBinStruct
			} else {
				gen = compositeBinStructNotype
			}
		case reflect.Map:
			// TODO(mpl): maybe needs a NoType version too
			gen = compositeBinMap
		case reflect.Ptr:
			gen = compositeGenerator(n, typ, n.typ.val.rtype)
		case reflect.Slice, reflect.Array:
			gen = compositeBinSlice
		default:
			log.Panic(n.cfgErrorf("compositeGenerator not implemented for type kind: %s", k))
		}
	}
	return gen
}

// arrayTypeLen returns the node's array length. If the expression is an
// array variable it is determined from the value's type, otherwise it is
// computed from the source definition.
func arrayTypeLen(n *node) int {
	if n.typ != nil && n.typ.cat == arrayT {
		return n.typ.length
	}
	max := -1
	for i, c := range n.child[1:] {
		r := i
		if c.kind == keyValueExpr {
			if v := c.child[0].rval; v.IsValid() {
				r = int(c.child[0].rval.Int())
			}
		}
		if r > max {
			max = r
		}
	}
	return max + 1
}

// isValueUntyped returns true if value is untyped.
func isValueUntyped(v reflect.Value) bool {
	// Consider only constant values.
	if v.CanSet() {
		return false
	}
	return v.Type().Implements(constVal)
}

// isArithmeticAction returns true if the node action is an arithmetic operator.
func isArithmeticAction(n *node) bool {
	switch n.action {
	case aAdd, aAnd, aAndNot, aBitNot, aMul, aNeg, aOr, aPos, aQuo, aRem, aShl, aShr, aSub, aXor:
		return true
	}
	return false
}

func isBoolAction(n *node) bool {
	switch n.action {
	case aEqual, aGreater, aGreaterEqual, aLand, aLor, aLower, aLowerEqual, aNot, aNotEqual:
		return true
	}
	return false
}
