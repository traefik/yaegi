package interp

import (
	"log"
	"reflect"
	"strconv"
	"unicode"
)

type SymKind uint

const (
	Const SymKind = iota
	Typ
	Var
	Func
	Bin
	Bltn
)

var symKinds = [...]string{
	Const: "Const",
	Var:   "Var",
	Func:  "Func",
	Bin:   "Bin",
	Bltn:  "Bltn",
}

func (k SymKind) String() string {
	if k < SymKind(len(symKinds)) {
		return symKinds[k]
	}
	return "SymKind(" + strconv.Itoa(int(k)) + ")"
}

// A Symbol represents an interpreter object such as type, constant, var, func, builtin or binary object
type Symbol struct {
	kind     SymKind
	typ      *Type            // Type of value
	node     *Node            // Node value if index is negative
	recv     *Receiver        // receiver node value, if sym refers to a method
	index    int              // index of value in frame or -1
	val      interface{}      // default value (used for constants)
	path     string           // package path if typ.cat is SrcPkgT or BinPkgT
	builtin  BuiltinGenerator // Builtin function or nil
	global   bool             // true if symbol is defined in global space
	constant bool             // true if symbol value is constant
}

// A SymMap stores symbols indexed by name
type SymMap map[string]*Symbol

// Scope type stores the list of visible symbols at current scope level
type Scope struct {
	anc    *Scope // Ancestor upper scope
	level  int    // Frame level: number of frame indirections to access var during execution
	size   int    // Frame size: number of entries to allocate during execution (package scope only)
	sym    SymMap // Map of symbols defined in this current scope
	global bool   // true if scope refers to global space (single frame for universe and package level scopes)
}

// Create a new scope and chain it to the current one
func (s *Scope) push(indirect int) *Scope {
	var size int
	if indirect == 0 {
		size = s.size // propagate size as scopes at same level share the same frame
	}
	global := s.global && indirect == 0
	return &Scope{anc: s, global: global, level: s.level + indirect, size: size, sym: map[string]*Symbol{}}
}

func (s *Scope) pop() *Scope {
	if s.level == s.anc.level {
		s.anc.size = s.size // propagate size as scopes at same level share the same frame
	}
	return s.anc
}

// Lookup for a symbol in the current scope, and upper ones if not found
func (s *Scope) lookup(ident string) (*Symbol, int, bool) {
	level := s.level
	for s != nil {
		if sym, ok := s.sym[ident]; ok {
			return sym, level - s.level, true
		}
		s = s.anc
	}
	return nil, 0, false
}

func (s *Scope) getType(ident string) *Type {
	var t *Type
	if sym, _, found := s.lookup(ident); found {
		if sym.kind == Typ {
			t = sym.typ
		}
	}
	return t
}

// Inc increments the size of the scope data frame and returns the new size
func (scope *Scope) inc(interp *Interpreter) int {
	if scope.global {
		interp.fsize++
		scope.size = interp.fsize
	} else {
		scope.size++
	}
	return scope.size
}

// Cfg generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables. A list of nodes of init functions is returned.
// Following this pass, the CFG is ready to run
func (interp *Interpreter) Cfg(root *Node) []*Node {
	scope := interp.universe
	var loop, loopRestart *Node
	var initNodes []*Node
	var exports *BinMap
	var expval *ValueMap
	var iotaValue int
	var pkgName string

	root.Walk(func(n *Node) bool {
		// Pre-order processing
		switch n.kind {
		case Define, AssignStmt:
			if l := len(n.child); n.anc.kind == ConstDecl && l == 1 {
				// Implicit iota assignment. TODO: replicate previous explicit assignment
				n.child = append(n.child, &Node{anc: n, interp: interp, kind: Ident, ident: "iota"})
			} else if l%2 == 1 {
				// Odd number of children: remove the type node, useless for assign
				i := l / 2
				n.child = append(n.child[:i], n.child[i+1:]...)
			}

		case BlockStmt:
			scope = scope.push(0)

		case File:
			pkgName = n.child[0].ident
			if _, ok := interp.scope[pkgName]; !ok {
				interp.scope[pkgName] = scope.push(0)
			}
			scope = interp.scope[pkgName]
			scope.size = interp.fsize
			if pkg, ok := interp.Exports[pkgName]; ok {
				exports = pkg
				expval = interp.Expval[pkgName]
			} else {
				exports = &BinMap{}
				interp.Exports[pkgName] = exports
				expval = &ValueMap{}
				interp.Expval[pkgName] = expval
			}

		case For0, ForRangeStmt:
			loop, loopRestart = n, n.child[0]
			scope = scope.push(0)

		case For1, For2, For3, For3a, For4:
			loop, loopRestart = n, n.child[len(n.child)-1]
			scope = scope.push(0)

		case FuncDecl, FuncLit:
			if n.child[1].ident == "init" {
				initNodes = append(initNodes, n)
			}
			// Add a frame indirection level as we enter in a func
			scope = scope.push(1)
			if len(n.child[2].child) == 2 {
				// allocate entries for return values at start of frame
				scope.size += len(n.child[2].child[1].child)
			}

		case If0, If1, If2, If3:
			scope = scope.push(0)

		case Switch0:
			// Make sure default clause is in last position
			c := n.child[1].child
			if i, l := getDefault(n), len(c)-1; i >= 0 && i != l {
				c[i], c[l] = c[l], c[i]
			}
			scope = scope.push(0)

		case ImportSpec, TypeSpec:
			// processing already done in GTA pass
			return false

		case ArrayType, BasicLit, ChanType, MapType, StructType:
			n.typ = nodeType(interp, scope, n)
			return false
		}
		return true
	}, func(n *Node) {
		// Post-order processing
		switch n.kind {
		case Address:
			wireChild(n)
			n.typ = &Type{cat: PtrT, val: n.child[0].typ}
			n.findex = scope.inc(interp)

		case Define, AssignStmt:
			wireChild(n)
			name := n.child[0].ident
			sym, level, _ := scope.lookup(name)
			if n.kind == Define {
				n.child[0].val = n.child[1].val
				n.child[0].typ = n.child[1].typ
				n.child[0].recv = n.child[1].recv
				n.child[0].findex = sym.index
				if n.child[1].action == GetFunc {
					scope.sym[name].index = -1
					scope.sym[name].node = n.child[1]
				}
				if n.child[1].kind == BasicLit {
					scope.sym[name].val = n.child[1].val
				}
			}
			n.findex = n.child[0].findex
			n.val = n.child[0].val
			// Propagate type
			// TODO: Check that existing destination type matches source type
			if n.child[1].action == Recv {
				// Assign by reading from a receiving channel
				n.gen = nop
				n.child[1].findex = n.child[0].findex // Set recv address to LHS
				n.child[0].typ = n.child[1].typ.val
			} else if n.child[1].action == CompositeLit {
				n.gen = nop
				n.child[1].findex = n.child[0].findex
			}
			n.typ = n.child[0].typ
			if sym != nil {
				sym.typ = n.typ
				sym.recv = n.child[1].recv
			}
			n.level = level
			//log.Println(n.index, "assign", n.child[0].ident, n.typ.cat, n.findex, n.level)
			// If LHS is an indirection, get reference instead of value, to allow setting
			if n.child[0].action == GetIndex {
				if n.child[0].child[0].typ.cat == MapT {
					n.gen = assignMap
					n.child[0].gen = nop // skip getIndexMap
				}
			}
			//} else if n.child[0].child[0].typ.cat == PtrT {
			//	// Handle the case where the receiver is a pointer to an object
			//	n.child[0].gen = getPtrIndexAddr
			//	n.gen = assignPtrField
			//} else if n.child[0].action == Star {
			//		n.findex = n.child[0].child[0].findex
			//		n.level = n.child[0].child[0].level
			//		n.gen = indirectAssign
			//	}
			if n.anc.kind == ConstDecl {
				iotaValue++
			}

		case IncDecStmt:
			wireChild(n)
			n.findex = n.child[0].findex
			n.level = n.child[0].level
			n.child[0].typ = scope.getType("int")
			n.typ = n.child[0].typ
			if sym, level, ok := scope.lookup(n.child[0].ident); ok {
				sym.typ = n.typ
				n.level = level
			}
			//if n.child[0].action == Star {
			//	n.findex = n.child[0].child[0].findex
			//	n.level = n.child[0].child[0].level
			//	n.gen = indirectInc
			//}

		case DefineX, AssignXStmt:
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

			case IndexExpr:
				types = append(types, n.child[l].child[0].typ.val, scope.getType("bool"))
				n.child[l].gen = getIndexMap2
				n.gen = nop

			case TypeAssertExpr:
				types = append(types, n.child[l].child[1].typ, scope.getType("error"))

			default:
				log.Fatalln(n.index, "Assign expression unsupported:", n.child[l].kind)
			}
			for i, c := range n.child[:l] {
				sym, _, ok := scope.lookup(c.ident)
				if !ok {
					log.Panic("symbol not found", c.ident)
				}
				sym.typ = types[i]
				c.typ = sym.typ
			}

		case BinaryExpr:
			wireChild(n)
			n.findex = scope.inc(interp)
			nilSym := interp.universe.sym["nil"]
			switch n.action {
			case NotEqual:
				n.typ = scope.getType("bool")
				if n.child[0].sym == nilSym || n.child[1].sym == nilSym {
					n.gen = isNotNil
				}
			case Equal:
				n.typ = scope.getType("bool")
				if n.child[0].sym == nilSym || n.child[1].sym == nilSym {
					n.gen = isNil
				}
			case Greater, Lower:
				n.typ = scope.getType("bool")
			default:
				n.typ = n.child[0].typ
			}

		case IndexExpr:
			wireChild(n)
			n.findex = scope.inc(interp)
			n.typ = n.child[0].typ.val
			n.recv = &Receiver{node: n}
			if n.child[0].typ.cat == MapT {
				scope.size++ // Reserve an entry for getIndexMap 2nd return value
				n.gen = getIndexMap
			} else if n.child[0].typ.cat == ArrayT {
				n.gen = getIndexArray
			}

		case BlockStmt:
			wireChild(n)
			if len(n.child) > 0 {
				n.findex = n.child[len(n.child)-1].findex
			}
			scope = scope.pop()

		case ConstDecl:
			wireChild(n)
			iotaValue = 0

		case DeclStmt, ExprStmt, VarDecl, ParenExpr, SendStmt:
			wireChild(n)
			n.findex = n.child[len(n.child)-1].findex

		case Break:
			n.tnext = loop

		case CallExpr:
			wireChild(n)
			n.findex = scope.inc(interp)
			if isBuiltinCall(n) {
				n.gen = n.child[0].sym.builtin
				n.child[0].typ = &Type{cat: BuiltinT}
				switch n.child[0].ident {
				case "cap", "len":
					n.typ = scope.getType("int")
				case "make":
					if n.typ = scope.getType(n.child[1].ident); n.typ == nil {
						n.typ = nodeType(interp, scope, n.child[1])
					}
					n.child[1].val = n.typ
					n.child[1].kind = BasicLit
				}
			} else if n.child[0].isType(scope) {
				// Type conversion expression
				n.typ = n.child[0].typ
				if n.typ.cat == AliasT {
					n.gen = convert
				} else {
					if n.child[1].typ.cat == FuncT {
						// Convert type of an interpreter function to another binary type:
						// generate a different callback wrapper
						n.gen = convertFuncBin
					} else {
						n.gen = convertBin
					}
				}
			} else if isBinCall(n) {
				n.gen = callBin
				n.fsize = n.child[0].fsize
			} else {
				if typ := n.child[0].typ; len(typ.ret) > 0 {
					n.typ = n.child[0].typ.ret[0]
					n.fsize = len(typ.ret)
				}
			}
			// Reserve entries in frame to store results of call
			if scope.global {
				interp.fsize += n.fsize
				scope.size = interp.fsize
			} else {
				scope.size += n.fsize
			}

			/*
				else if n.child[0].kind == SelectorImport {
					// TODO: Should process according to child type, not kind.
					n.fsize = n.child[0].fsize
					var rtype reflect.Type
					switch t := n.child[0].val.(type) {
					case reflect.Type:
						rtype = n.child[0].val.(reflect.Type)
					case reflect.Value:
						rtype = n.child[0].val.(reflect.Value).Type()
					default:
						log.Printf("unexpected type %T\n", t)
					}
					if rtype.NumOut() > 0 {
						n.typ = &Type{cat: ValueT, rtype: rtype.Out(0)}
					}
					n.child[0].kind = BasicLit
					for i, c := range n.child[1:] {
						// Wrap function defintion so it can be called from runtime
						if c.kind == FuncLit {
							n.child[1+i].rval = reflect.MakeFunc(rtype.In(i), c.wrapNode)
							n.child[1+i].kind = Rvalue
						} else if c.ident == "nil" {
							n.child[1+i].rval = reflect.New(rtype.In(i)).Elem()
							n.child[1+i].kind = Rvalue
						} else if c.typ != nil && c.typ.cat == FuncT {
							n.child[1+i].rval = reflect.MakeFunc(rtype.In(i), c.wrapNode)
							n.child[1+i].kind = Rvalue
						}
					}
					// TODO: handle multiple return value
					if len(n.child) == 2 && n.child[1].fsize > 1 {
						n.gen = callBinX
					} else {
						n.gen = callBin
					}
				} else if n.child[0].kind == SelectorExpr {
					if n.child[0].typ.cat == ValueT {
						n.gen = callBinMethod
						// TODO: handle multiple return value (_test/time2.go)
						n.child[0].kind = BasicLit // Temporary hack for force value() to return n.val at run
						n.typ = &Type{cat: ValueT, rtype: n.child[0].typ.rtype}
						n.fsize = n.child[0].fsize
					} else if n.child[0].typ.cat == PtrT && n.child[0].typ.val.cat == ValueT {
						n.gen = callBinMethod
						n.child[0].kind = BasicLit // Temporary hack for force value() to return n.val at run
						n.fsize = n.child[0].fsize
						// TODO: handle type ?
					} else if n.child[0].typ.cat == SrcPkgT {
						n.val = n.child[0].val
						if def := n.val.(*Node); def != nil {
							// Reserve as many frame entries as nb of ret values for called function
							// node frame index should point to the first entry
							j := len(def.child[2].child) - 1
							l := len(def.child[2].child[j].child) // Number of return values for def
							if l == 1 {
								// If def returns exactly one value, propagate its type in call node.
								// Multiple return values will be handled differently through AssignX.
								n.typ = scope.getType(def.child[2].child[j].child[0].child[0].ident)
							}
							n.fsize = l
						}
					} else {
						// Get method and receiver path, store them in node static value for run
						if methodDecl, ok := n.child[0].val.(*Node); ok {
							// method is already resolved, use it
							if len(methodDecl.child[2].child) > 1 {
								// Allocate frame for method return values (if any)
								n.fsize = len(methodDecl.child[2].child[1].child)
								n.typ = methodDecl.typ.ret[0]
								// TODO: handle multiple return values
							} else {
								n.fsize = 0
							}
						} else {
							log.Println(n.index, "unresolve call")
							// method must be resolved here due to declaration after use
						}
						n.child[0].findex = -1 // To force reading value from node instead of frame (methods)
					}
				} else if sym, _, _ := scope.lookup(n.child[0].ident); sym != nil {
					if sym.typ != nil && sym.typ.cat == BinT {
						n.gen = callBin
						n.typ = &Type{cat: ValueT}
						r := sym.val.(reflect.Value)
						n.child[0].fsize = r.Type().NumOut()
						n.child[0].val = r
						n.child[0].kind = BasicLit
					} else if sym.typ != nil && sym.typ.cat == ValueT {
						n.gen = callDirectBin
						n.typ = &Type{cat: ValueT}
					} else {
						n.val = sym.node
						n.fsize = len(sym.typ.ret)
						if n.fsize == 1 {
							// If called func returns exactly one value, propagate its type in call node.
							// Multiple return values will be handled differently through AssignX.
							n.typ = sym.typ.ret[0]
						}
					}
				} else if n.child[0].kind == SelectorSrc {
					// Forward type of first returned value
					// Multiple return values will be handled differently through AssignX.
					if len(n.child[0].typ.ret) > 0 {
						n.typ = n.child[0].typ.ret[0]
						n.fsize = len(n.child[0].typ.ret)
					}
				}
			*/

		case CaseClause:
			n.findex = scope.inc(interp)
			n.tnext = n.child[len(n.child)-1].start

		case CompositeLitExpr:
			wireChild(n)
			if n.anc.action != Assign {
				n.findex = scope.inc(interp)
			}
			if n.child[0].typ == nil {
				n.child[0].typ = nodeType(interp, scope, n.child[0])
			}
			// TODO: Check that composite litteral expr matches corresponding type
			n.typ = n.child[0].typ
			switch n.typ.cat {
			case ArrayT:
				n.gen = arrayLit
			case MapT:
				n.gen = mapLit
			case StructT:
				n.action, n.gen = CompositeLit, compositeLit
				// Handle object assign from sparse key / values
				if len(n.child) > 1 && n.child[1].kind == KeyValueExpr {
					n.gen = compositeSparse
					n.typ = nodeType(interp, scope, n.child[0])
					for _, c := range n.child[1:] {
						c.findex = n.typ.fieldIndex(c.child[0].ident)
					}
				}
			}

		case Continue:
			n.tnext = loopRestart

		case Field:
			// A single child node (no ident, just type) means that the field refers
			// to a return value, and space on frame should be accordingly allocated.
			// Otherwise, just point to corresponding location in frame, resolved in
			// ident child.
			l := len(n.child) - 1
			n.typ = nodeType(interp, scope.anc, n.child[l])
			if l == 0 {
				if n.anc.anc.kind == FuncDecl {
					// Receiver with implicit var decl
					scope.sym[n.child[0].ident].typ = n.typ
					n.child[0].typ = n.typ
				} else {
					n.findex = scope.inc(interp)
				}
			} else {
				for _, f := range n.child[:l] {
					scope.sym[f.ident].typ = n.typ
					f.typ = n.typ
				}
			}

		case File:
			wireChild(n)
			scope = scope.pop()
			n.fsize = scope.size + 1
			n.types = frameTypes(n, n.fsize)

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
			//n.findex = n.child[0].findex
			n.child[0].fnext = n
			scope = scope.pop()

		case FuncDecl:
			n.flen = scope.size + 1
			if len(n.child[0].child) > 0 {
				// Method: restore receiver frame location (used at run)
				n.framepos = append(n.framepos, n.child[0].child[0].child[0].findex)
			}
			n.framepos = append(n.framepos, n.child[2].framepos...)
			scope = scope.pop()
			funcName := n.child[1].ident
			if canExport(funcName) {
				(*exports)[funcName] = reflect.MakeFunc(n.child[2].typ.TypeOf(), n.wrapNode).Interface()
				(*expval)[funcName] = reflect.MakeFunc(n.child[2].typ.TypeOf(), n.wrapNode)
			}
			n.typ = n.child[2].typ
			n.val = n
			n.start = n.child[3].start
			interp.scope[pkgName].sym[funcName].index = -1 // to force value to n.val
			interp.scope[pkgName].sym[funcName].typ = n.typ
			interp.scope[pkgName].sym[funcName].kind = Func
			interp.scope[pkgName].sym[funcName].node = n
			n.types = frameTypes(n, n.flen)
			genFun(n)

		case FuncLit:
			n.typ = n.child[2].typ
			n.val = n
			n.flen = scope.size + 1
			scope = scope.pop()
			n.types = frameTypes(n, n.flen)
			n.framepos = n.child[2].framepos

		case FuncType:
			n.typ = nodeType(interp, scope, n)
			// Store list of parameter frame indices in framepos
			for _, c := range n.child[0].child {
				for _, f := range c.child[:len(c.child)-1] {
					n.framepos = append(n.framepos, f.findex)
				}
			}
			// TODO: do the same for return values

		case GoStmt:
			wireChild(n)
			// TODO: should error if call expression refers to a builtin

		case Ident:
			if isKey(n) {
				// Skip symbol creation/lookup for idents used as key
			} else if isFuncArg(n) {
				n.findex = scope.inc(interp)
				scope.sym[n.ident] = &Symbol{index: scope.size, kind: Var, global: scope.global}
				n.sym = scope.sym[n.ident]
			} else if isNewDefine(n) {
				// Create a new symbol in current scope, type to be set by parent node
				// Note that global symbol should already be defined (gta)
				if _, _, ok := scope.lookup(n.ident); !ok && !scope.global {
					n.findex = scope.inc(interp)
					scope.sym[n.ident] = &Symbol{index: scope.size, kind: Var, global: scope.global}
					n.sym = scope.sym[n.ident]
				}
			} else if sym, level, ok := scope.lookup(n.ident); ok {
				// Found symbol, populate node info
				n.typ, n.findex, n.level = sym.typ, sym.index, level
				if n.findex < 0 {
					n.val = sym.node
					n.kind = sym.node.kind
				} else {
					n.sym = sym
					if sym.kind == Const && sym.val != nil {
						n.val = sym.val
						n.kind = BasicLit
					} else if n.ident == "iota" {
						n.val = iotaValue
						n.kind = BasicLit
					} else if n.ident == "nil" {
						n.kind = BasicLit
						n.val = nil
					} else if sym.kind == Bin {
						if sym.val == nil {
							n.kind = Rtype
						} else {
							n.kind = Rvalue
						}
						n.typ = sym.typ
						if n.typ.rtype.Kind() == reflect.Func {
							n.fsize = n.typ.rtype.NumOut()
						}
						n.rval = sym.val.(reflect.Value)
					}
				}
				if n.sym != nil {
					n.recv = n.sym.recv
				}
			} else {
				log.Println(n.index, "unresolved symbol", n.ident)
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
			n.findex = scope.inc(interp)
			n.typ = n.child[0].typ

		case LorExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n
			n.child[0].fnext = n.child[1].start
			n.child[1].tnext = n
			n.findex = scope.inc(interp)
			n.typ = n.child[0].typ

		case RangeStmt:
			n.start = n.child[2]                // Get array or map object
			n.child[2].tnext = n.child[0].start // then go to iterator init
			n.child[0].tnext = n                // then go to range function
			n.tnext = n.child[3].start          // then go to range body
			n.child[3].tnext = n                // then body go to range function (loop)
			n.child[0].gen = empty              // init filled later by generator
			if n.child[2].typ.cat == MapT {
				scope.sym[n.child[0].ident].typ = n.child[2].typ.key
				n.child[0].typ = n.child[2].typ.key
				n.gen = rangeMap
			} else {
				scope.sym[n.child[0].ident].typ = scope.getType("int")
				n.child[0].typ = scope.getType("int")
			}
			vtype := n.child[2].typ.val
			scope.sym[n.child[1].ident].typ = vtype
			n.child[1].typ = vtype

		case ReturnStmt:
			wireChild(n)
			n.tnext = nil

		case SelectorExpr:
			wireChild(n)
			n.findex = scope.inc(interp)
			n.typ = n.child[0].typ
			n.recv = n.child[0].recv
			if n.typ == nil {
				log.Fatal("typ should not be nil:", n.index, n.child[0])
			}
			//log.Println(n.index, "selector", n.child[0].ident+"."+n.child[1].ident, n.typ.cat)
			if n.typ.cat == ValueT {
				// Handle object defined in runtime
				if method, ok := n.typ.rtype.MethodByName(n.child[1].ident); ok {
					if method.Func.IsValid() {
						n.rval = method.Func
						n.typ.rtype = method.Func.Type()
						n.gen = nop
					} else {
						n.val = method.Index
						n.gen = getIndexBinMethod
					}
					n.fsize = method.Type.NumOut()
				} else {
					// Method can be only resolved from value at execution
					log.Println(n.index, "could not solve method")
					n.gen = nop
					//n.gen = getIndexBinMethod
				}
			} else if n.typ.cat == PtrT && n.typ.val.cat == ValueT {
				// Handle pointer on object defined in runtime
				if field, ok := n.typ.val.rtype.FieldByName(n.child[1].ident); ok {
					n.typ = &Type{cat: ValueT, rtype: field.Type}
					n.val = field.Index
					n.gen = getPtrIndexBin
				} else if method, ok := n.typ.val.rtype.MethodByName(n.child[1].ident); ok {
					n.val = method.Func
					n.fsize = method.Type.NumOut()
					n.gen = nop
				} else if method, ok := reflect.PtrTo(n.typ.val.rtype).MethodByName(n.child[1].ident); ok {
					n.val = method.Func
					n.fsize = method.Type.NumOut()
					n.gen = nop
				} else {
					log.Println(n.index, "selector unresolved")
				}
			} else if n.typ.cat == BinPkgT {
				// Resolve binary package symbol: a type or a value
				name := n.child[1].ident
				pkg := n.child[0].sym.path
				if s, ok := interp.binValue[pkg][name]; ok {
					n.kind = Rvalue
					n.rval = s
					n.typ = &Type{cat: ValueT, rtype: s.Type()}
					if s.Kind() == reflect.Func {
						n.fsize = n.typ.rtype.NumOut()
					}
					n.gen = nop
				} else if s, ok := interp.binType[pkg][name]; ok {
					n.kind = Rtype
					n.typ = &Type{cat: ValueT, rtype: s}
					n.gen = nop
					if s.Kind() == reflect.Func {
						n.fsize = s.NumOut()
					}
				}
			} else if n.typ.cat == ArrayT {
				n.typ = n.typ.val
				n.gen = nop
			} else if n.typ.cat == SrcPkgT {
				// Resolve source package symbol
				if sym, ok := interp.scope[n.child[0].ident].sym[n.child[1].ident]; ok {
					n.val = sym.node
					n.gen = nop
					n.kind = SelectorSrc
					n.typ = sym.typ
				} else {
					log.Println(n.index, "selector unresolved:", n.child[0].ident+"."+n.child[1].ident)
				}
			} else if fi := n.typ.fieldIndex(n.child[1].ident); fi >= 0 {
				// Resolve struct field index
				if n.typ.cat == PtrT {
					n.gen = getPtrIndex
				}
				n.typ = n.typ.fieldType(fi)
				n.child[1].findex = -1
				n.child[1].val = fi
			} else if m, lind := n.typ.lookupMethod(n.child[1].ident); m != nil {
				// Handle method
				n.gen = nop
				n.findex = -1
				n.val = m
				n.typ = m.typ
				n.recv = &Receiver{node: n.child[0], index: lind}
			} else {
				// Handle promoted field in embedded struct
				if ti := n.typ.lookupField(n.child[1].ident); len(ti) > 0 {
					n.child[1].findex = -1
					n.child[1].val = ti
					n.gen = getIndexSeq
				} else {
					log.Println(n.index, "Selector not found:", n.child[1].ident)
					n.gen = nop
					//panic("Field not found in selector")
				}
			}

		case StarExpr:
			wireChild(n)
			n.typ = n.child[0].typ.val
			n.findex = scope.inc(interp)

		case Switch0:
			n.start = n.child[1].start
			// Chain case clauses
			clauses := n.child[1].child
			l := len(clauses)
			for i, c := range clauses[:l-1] {
				// chain to next clause
				c.tnext = c.child[1].start
				c.child[1].tnext = n
				c.fnext = clauses[i+1]
			}
			// Handle last clause
			if c := clauses[l-1]; len(c.child) > 1 {
				// No default clause
				c.tnext = c.child[1].start
				c.fnext = n
				c.child[1].tnext = n
			} else {
				// Default
				c.tnext = c.child[0].start
				c.child[0].tnext = n
			}
			scope = scope.pop()

		case TypeAssertExpr:
			if n.child[1].typ == nil {
				n.child[1].typ = scope.getType(n.child[1].ident)
			}
			n.typ = n.child[1].typ

		case SliceExpr, UnaryExpr:
			wireChild(n)
			n.typ = n.child[0].typ
			log.Println(n.index, n.action)
			if n.action == Negate {
				n.findex = scope.inc(interp)
			}

		case ValueSpec:
			l := len(n.child) - 1
			if n.typ = n.child[l].typ; n.typ == nil {
				n.typ = scope.getType(n.child[l].ident)
			}
			for _, c := range n.child[:l] {
				c.typ = n.typ
				scope.sym[c.ident].typ = n.typ
			}
		}
	})

	return initNodes
}

func genRun(n *Node) {
	n.Walk(func(n *Node) bool {
		if n.kind == FuncType && len(n.anc.child) == 4 {
			setExec(n.anc.child[3].start)
		}
		if n.kind == VarDecl || n.kind == ConstDecl {
			setExec(n.start)
			return false
		}
		return true
	}, nil)
}

// Find default case clause index of a switch statement, if any
func getDefault(n *Node) int {
	for i, c := range n.child[1].child {
		if len(c.child) == 1 {
			return i
		}
	}
	return -1
}

// isType returns true if node refers to a type definition, false otherwise
func (n *Node) isType(scope *Scope) bool {
	switch n.kind {
	case ArrayType, ChanType, FuncType, MapType, StructType, Rtype:
		return true
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
		n.child[i-1].tnext = n.child[i].start
	}

	// Chain subtree next to self
	for i := len(n.child) - 1; i >= 0; i-- {
		switch n.child[i].kind {
		case ArrayType, ChanType, ImportDecl, MapType, FuncDecl, BasicLit, Ident, TypeDecl:
			continue
		case Break, Continue, ReturnStmt:
			// tnext is already computed, no change
		default:
			n.child[i].tnext = n
		}
		break
	}
}

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
	if n.anc.kind == DefineX && n.anc.child[len(n.anc.child)-1] != n {
		return true
	}
	if n.anc.kind == RangeStmt && (n.anc.child[0] == n || n.anc.child[1] == n) {
		return true
	}
	if n.anc.kind == ValueSpec {
		return true
	}
	return false
}

func isFuncArg(n *Node) bool {
	if n.anc.kind != Field {
		return false
	}
	l := len(n.anc.child)
	if l == 1 && n.anc.anc.anc.kind == FuncDecl {
		return true
	}
	if l > 1 && n.anc.child[l-1] != n {
		return true
	}
	return false
}

func isBuiltinCall(n *Node) bool {
	return n.kind == CallExpr && n.child[0].sym != nil && n.child[0].sym.kind == Bltn
}

func isBinCall(n *Node) bool {
	return n.kind == CallExpr && n.child[0].typ.cat == ValueT
}

func isRegularCall(n *Node) bool {
	return n.kind == CallExpr && n.child[0].typ.cat != ValueT && (n.child[0].sym == nil || n.child[0].sym.kind != Bltn)
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

// setExec recursively sets the node exec builtin function
// it does nothing if exec is already defined
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

func valueGenerator(n *Node, i int) func(*Frame) reflect.Value {
	switch n.level {
	case 0:
		return func(f *Frame) reflect.Value { return f.data[i] }
	case 1:
		return func(f *Frame) reflect.Value {
			return f.anc.data[i]
		}
	case 2:
		return func(f *Frame) reflect.Value { return f.anc.anc.data[i] }
	default:
		return func(f *Frame) reflect.Value {
			for level := n.level; level > 0; level-- {
				f = f.anc
			}
			return f.data[i]
		}
	}
}

func genValueRecv(n *Node) func(*Frame) reflect.Value {
	v := genValue(n.recv.node)
	fi := n.recv.index

	if len(fi) == 0 {
		return v
	}

	return func(f *Frame) reflect.Value {
		r := v(f)
		if r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		return r.FieldByIndex(fi)
	}
}

func genValuePtr(n *Node) func(*Frame) reflect.Value {
	v := genValue(n)
	return func(f *Frame) reflect.Value { return v(f).Addr() }
}

func genValue(n *Node) func(*Frame) reflect.Value {
	switch n.kind {
	case BasicLit, FuncDecl, SelectorSrc:
		var v reflect.Value
		if w, ok := n.val.(reflect.Value); ok {
			v = w
		} else {
			v = reflect.ValueOf(n.val)
		}
		return func(f *Frame) reflect.Value { return v }
	case Rvalue:
		v := n.rval
		return func(f *Frame) reflect.Value { return v }
	default:
		if n.sym != nil {
			if n.sym.index < 0 {
				return genValue(n.sym.node)
			}
			i := n.sym.index
			if n.sym.global {
				return func(f *Frame) reflect.Value {
					return n.interp.Frame.data[i]
				}
			}
			return valueGenerator(n, i)
		}
		if n.findex < 0 {
			var v reflect.Value
			if w, ok := n.val.(reflect.Value); ok {
				v = w
			} else {
				v = reflect.ValueOf(n.val)
			}
			return func(f *Frame) reflect.Value { return v }
		}
		return valueGenerator(n, n.findex)
	}
	return nil
}

func genFun(n *Node) {
	start := n.child[3].start
	framepos := n.framepos
	flen := n.flen
	types := n.types
	nout := len(n.typ.ret)

	n.fun = func(f *Frame, in []reflect.Value, goroutine bool) []reflect.Value {
		// Allocate local frame
		nf := Frame{anc: f, data: make([]reflect.Value, flen)}
		for i, t := range types {
			if t != nil {
				nf.data[i] = reflect.New(t).Elem()
			}
		}
		// Copy input parameters from caller frame to local
		for i, pos := range framepos {
			nf.data[pos].Set(in[i])
		}
		if goroutine {
			go runCfg(start, &nf)
		} else {
			runCfg(start, &nf)
		}
		// Propagate return values to caller
		return nf.data[:nout]
	}
}

// Experimental, temporary & incomplete
func getValue(n *Node) (int, reflect.Value, bool) {
	var index int
	var val reflect.Value
	var isReflect bool
	switch n.kind {
	case BasicLit:
		val = reflect.ValueOf(n.val)
	default:
		if n.sym != nil {
			index = n.sym.index
			isReflect = true
		} else if n.findex < 0 {
			val = reflect.ValueOf(n.val)
		} else {
			index = n.findex
			isReflect = true
		}
	}
	return index, val, isReflect
}

// frameTypes return a slice of frame types for FuncDecl or FuncLit nodes
func frameTypes(node *Node, size int) []reflect.Type {
	ft := make([]reflect.Type, size)

	node.Walk(func(n *Node) bool {
		if n.kind == FuncDecl || n.kind == ImportDecl || n.kind == TypeDecl || n.kind == FuncLit {
			return n == node // Do not dive in substree, except if this the entry point
		}
		if n.findex < 0 || n.typ == nil || n.level > 0 || n.kind == BasicLit || n.kind == SelectorSrc {
			return true
		}
		if ft[n.findex] == nil {
			if n.typ.cat == FuncT {
				ft[n.findex] = reflect.TypeOf(n)
			} else {
				ft[n.findex] = n.typ.TypeOf()
			}
		} else {
			// TODO: Check that type is identical
		}
		return true
	}, nil)

	return ft
}
