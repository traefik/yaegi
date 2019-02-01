package interp

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"unicode"
)

// A CfgError represents an error during CFG build stage
type CfgError error

// A SymKind represents the kind of symbol
type SymKind uint

// Symbol kinds for the go language
const (
	Const SymKind = iota // Constant
	Typ                  // Type
	Var                  // Variable
	Func                 // Function
	Bin                  // Binary from runtime
	Bltn                 // Builtin
)

var symKinds = [...]string{
	Const: "Const",
	Typ:   "Typ",
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
	kind    SymKind
	typ     *Type            // Type of value
	node    *Node            // Node value if index is negative
	recv    *Receiver        // receiver node value, if sym refers to a method
	index   int              // index of value in frame or -1
	val     interface{}      // default value (used for constants)
	path    string           // package path if typ.cat is SrcPkgT or BinPkgT
	builtin BuiltinGenerator // Builtin function or nil
	global  bool             // true if symbol is defined in global space
	//constant bool             // true if symbol value is constant
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
func (s *Scope) inc(interp *Interpreter) int {
	if s.global {
		interp.fsize++
		s.size = interp.fsize
	} else {
		s.size++
	}
	return s.size
}

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
			interp.scope[pkgName] = scope.push(0)
		}
		scope = interp.scope[pkgName]
		scope.size = interp.fsize
	}

	root.Walk(func(n *Node) bool {
		// Pre-order processing
		if err != nil {
			return false
		}
		switch n.kind {
		case Define, AssignStmt:
			if l := len(n.child); n.anc.kind == ConstDecl && l == 1 {
				// Implicit iota assignment. TODO: replicate previous explicit assignment
				n.child = append(n.child, &Node{anc: n, interp: interp, kind: Ident, ident: "iota"})
			}

		case BlockStmt:
			// For range block: ensure that array or map type is propagated to iterators
			// prior to process block
			if n.anc != nil && n.anc.kind == RangeStmt {
				switch n.anc.child[2].typ.cat {
				case ValueT:
					typ := n.anc.child[2].typ.rtype

					switch typ.Kind() {
					case reflect.Map:
						scope.sym[n.anc.child[0].ident].typ = &Type{cat: ValueT, rtype: typ.Key()}
						scope.sym[n.anc.child[1].ident].typ = &Type{cat: ValueT, rtype: typ.Elem()}
						n.anc.gen = rangeMap
					case reflect.Array, reflect.Slice:
						scope.sym[n.anc.child[0].ident].typ = scope.getType("int")
						n.anc.child[0].typ = scope.getType("int")
						vtype := &Type{cat: ValueT, rtype: typ.Elem()}
						scope.sym[n.anc.child[1].ident].typ = vtype
						n.anc.child[1].typ = vtype
					}
				case MapT:
					scope.sym[n.anc.child[0].ident].typ = n.anc.child[2].typ.key
					n.anc.child[0].typ = n.anc.child[2].typ.key
					n.anc.gen = rangeMap
					vtype := n.anc.child[2].typ.val
					scope.sym[n.anc.child[1].ident].typ = vtype
					n.anc.child[1].typ = vtype
				default:
					scope.sym[n.anc.child[0].ident].typ = scope.getType("int")
					n.anc.child[0].typ = scope.getType("int")
					vtype := n.anc.child[2].typ.val
					scope.sym[n.anc.child[1].ident].typ = vtype
					n.anc.child[1].typ = vtype
				}
			}
			scope = scope.push(0)

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
				interp.scope[pkgName] = scope.push(0)
			}
			scope = interp.scope[pkgName]
			scope.size = interp.fsize

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
			n.findex = scope.inc(interp)

		case Define, AssignStmt:
			dest, src := n.child[0], n.child[len(n.child)-1]
			sym, level, _ := scope.lookup(dest.ident)
			if n.kind == Define {
				if len(n.child) == 3 {
					// Type is provided in var declaration
					dest.typ, err = nodeType(interp, scope, n.child[1])
				} else {
					dest.typ = src.typ
				}
				dest.val = src.val
				dest.recv = src.recv
				dest.findex = sym.index
				if src.action == GetFunc {
					sym.index = -1
					sym.node = src
				}
				if src.kind == BasicLit {
					sym.val = src.val
				} else if isRegularCall(src) || isBinCall(src) {
					// propagate call return value type
					dest.typ = getReturnedType(src.child[0])
					sym.typ = dest.typ
				}
			}
			wireChild(n)
			// Detect invalid float truncate
			if isInt(dest.typ) && isFloat(src.typ) {
				err = src.cfgError("invalid float truncate")
			}
			n.findex = dest.findex
			n.val = dest.val
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
			//log.Println(n.index, "assign", dest.ident, n.typ.cat, n.findex, n.level)
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
			n.child[0].typ = scope.getType("int")
			n.typ = n.child[0].typ
			if sym, level, ok := scope.lookup(n.child[0].ident); ok {
				sym.typ = n.typ
				n.level = level
			}

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
				types = append(types, n.child[l].child[1].typ, scope.getType("bool"))
				n.child[l].gen = typeAssert2
				n.gen = nop

			case UnaryExpr:
				if n.child[l].action == Recv {
					types = append(types, n.child[l].child[0].typ.val, scope.getType("bool"))
				}

			default:
				err = n.cfgError("unsupported assign expression")
				return
			}
			for i, c := range n.child[:l] {
				if i == len(types) {
					break // skip useless type symbol
				}
				sym, _, ok := scope.lookup(c.ident)
				if !ok {
					err = c.cfgError("undefined: %s", c.ident)
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
			case Greater, GreaterEqual, Lower, LowerEqual:
				n.typ = scope.getType("bool")
			default:
				n.typ, err = nodeType(interp, scope, n)
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

		case DeclStmt, ExprStmt, VarDecl, SendStmt:
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
						n.typ, err = nodeType(interp, scope, n.child[1])
					}
					n.child[1].val = n.typ
					n.child[1].kind = BasicLit
				case "recover":
					n.typ = scope.getType("interface{}")
				}
			} else if n.child[0].isType(scope) {
				// Type conversion expression
				n.typ = n.child[0].typ
				n.gen = convert
			} else if isBinCall(n) {
				n.gen = callBin
				n.fsize = n.child[0].fsize
				if typ := n.child[0].typ.rtype; typ.NumOut() > 0 {
					n.typ = &Type{cat: ValueT, rtype: typ.Out(0)}
				}
			} else if typ := n.child[0].typ; len(typ.ret) > 0 {
				n.typ = n.child[0].typ.ret[0]
				n.fsize = len(typ.ret)
			}

			// Reserve entries in frame to store results of call
			if scope.global {
				interp.fsize += n.fsize
				scope.size = interp.fsize
			} else {
				scope.size += n.fsize
			}

		case CaseClause:
			n.findex = scope.inc(interp)
			n.tnext = n.child[len(n.child)-1].start

		case SelectStmt:
			wireChild(n)
			// Move action to block statement, so select node can be an exit point
			n.child[0].gen = _select
			n.start = n.child[0]

		case CommClause:
			wireChild(n)
			if len(n.child) > 1 {
				n.start = n.child[1].start // Skip chan operation, performed by select
			} else {
				n.start = n.child[0].start // default clause
			}
			n.child[len(n.child)-1].tnext = n.anc.anc // exit node is SelectStmt

		case CompositeLitExpr:
			wireChild(n)
			if n.anc.action != Assign {
				n.findex = scope.inc(interp)
			}
			// TODO: Check that composite literal expr matches corresponding type
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
					if n.typ, err = nodeType(interp, scope, n.child[0]); err != nil {
						return
					}
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
			if n.typ, err = nodeType(interp, scope.anc, n.child[l]); err != nil {
				return
			}
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
					f.typ = n.typ
					if n.typ.variadic {
						scope.sym[f.ident].typ = &Type{cat: ArrayT, val: n.typ}
					} else {
						scope.sym[f.ident].typ = n.typ
					}
				}
			}

		case File:
			wireChild(n)
			scope = scope.pop()
			n.fsize = scope.size + 1

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
			n.flen = scope.size + 1
			if len(n.child[0].child) > 0 {
				// Method: restore receiver frame location (used at run)
				n.framepos = append(n.framepos, n.child[0].child[0].child[0].findex)
			}
			n.framepos = append(n.framepos, n.child[2].framepos...)
			scope = scope.pop()
			funcName := n.child[1].ident
			n.typ = n.child[2].typ
			n.val = n
			n.start = n.child[3].start
			interp.scope[pkgName].sym[funcName].index = -1 // to force value to n.val
			interp.scope[pkgName].sym[funcName].typ = n.typ
			interp.scope[pkgName].sym[funcName].kind = Func
			interp.scope[pkgName].sym[funcName].node = n

		case FuncLit:
			n.typ = n.child[2].typ
			n.val = n
			n.flen = scope.size + 1
			scope = scope.pop()
			n.framepos = n.child[2].framepos

		case FuncType:
			if n.typ, err = nodeType(interp, scope, n); err != nil {
				return
			}
			// Store list of parameter frame indices in framepos
			for _, c := range n.child[0].child {
				for _, f := range c.child[:len(c.child)-1] {
					n.framepos = append(n.framepos, f.findex)
				}
			}
			// TODO: do the same for return values

		case GoStmt:
			wireChild(n)

		case Ident:
			if isKey(n) {
				// Skip symbol creation/lookup for identifier used as key
			} else if isFuncArg(n) {
				n.findex = scope.inc(interp)
				scope.sym[n.ident] = &Symbol{index: scope.size, kind: Var, global: scope.global}
				n.sym = scope.sym[n.ident]
			} else if isNewDefine(n) {
				// Create a new symbol in current scope, type to be set by parent node
				// Note that global symbol should already be defined (gta)
				if _, _, ok := scope.lookup(n.ident); !ok || !scope.global {
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
						if n.typ.rtype.Kind() == reflect.Func {
							n.fsize = n.typ.rtype.NumOut()
						}
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
			n.findex = scope.inc(interp)
			n.typ = n.child[0].typ

		case LorExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n
			n.child[0].fnext = n.child[1].start
			n.child[1].tnext = n
			n.findex = scope.inc(interp)
			n.typ = n.child[0].typ

		case ParenExpr:
			wireChild(n)
			n.findex = n.child[len(n.child)-1].findex
			n.typ = n.child[len(n.child)-1].typ

		case RangeStmt:
			n.start = n.child[2]                // Get array or map object
			n.child[2].tnext = n.child[0].start // then go to iterator init
			n.child[0].tnext = n                // then go to range function
			n.tnext = n.child[3].start          // then go to range body
			n.child[3].tnext = n                // then body go to range function (loop)
			n.child[0].gen = empty              // init filled later by generator

		case ReturnStmt:
			wireChild(n)
			n.tnext = nil

		case SelectorExpr:
			wireChild(n)
			n.findex = scope.inc(interp)
			n.typ = n.child[0].typ
			n.recv = n.child[0].recv
			if n.typ == nil {
				err = n.cfgError("undefined type")
				return
			}
			//log.Println(n.index, "selector", n.child[0].ident+"."+n.child[1].ident, n.typ.cat)
			if n.typ.cat == ValueT {
				// Handle object defined in runtime, try to find field or method
				// Search for method first, as it applies both to types T and *T
				// Search for field must then be performed on type T only (not *T)
				switch method, ok := n.typ.rtype.MethodByName(n.child[1].ident); {
				case ok:
					n.val = method.Index
					n.gen = getIndexBinMethod
					n.typ = &Type{cat: ValueT, rtype: method.Type}
					n.fsize = method.Type.NumOut()
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
					return
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
					n.fsize = method.Type.NumOut()
					n.recv = &Receiver{node: n.child[0]}
					n.gen = getIndexBinMethod
				} else if method, ok := reflect.PtrTo(n.typ.val.rtype).MethodByName(n.child[1].ident); ok {
					n.val = method.Index
					n.fsize = method.Type.NumOut()
					n.gen = getIndexBinMethod
					n.typ = &Type{cat: ValueT, rtype: method.Type}
					n.recv = &Receiver{node: n.child[0]}
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
				log.Println(n.index, "selector:", n.child[0].ident, interp.scope[n.child[0].ident])
				if sym, ok := interp.scope[n.child[0].ident].sym[n.child[1].ident]; ok {
					n.val = sym.node
					n.gen = nop
					n.kind = SelectorSrc
					n.typ = sym.typ
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
				if n.typ.cat == PtrT {
					n.gen = getPtrIndexSeq
				} else {
					n.gen = getIndexSeq
				}
				n.typ = n.typ.fieldSeq(ti)
			} else {
				err = n.cfgError("undefined selector: %s", n.child[1].ident)
			}

		case StarExpr:
			if n.anc.kind == Define && len(n.anc.child) == 3 && n.anc.child[1] == n {
				// pointer type expression in a var definition
				n.gen = nop
			} else {
				// dereference expression
				wireChild(n)
				n.typ = n.child[0].typ.val
				n.findex = scope.inc(interp)
			}

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
			if n.anc.action != AssignX {
				n.typ = n.child[1].typ
				n.findex = scope.inc(interp)
			}

		case SliceExpr, UnaryExpr:
			wireChild(n)
			n.typ = n.child[0].typ
			// TODO: avoid allocation if boolean branch op (i.e. '!' in an 'if' expr)
			n.findex = scope.inc(interp)

		case ValueSpec:
			l := len(n.child) - 1
			if n.typ = n.child[l].typ; n.typ == nil {
				n.typ, err = nodeType(interp, scope, n.child[l])
				if err != nil {
					return
				}
			}
			for _, c := range n.child[:l] {
				c.typ = n.typ
				scope.sym[c.ident].typ = n.typ
			}
		}
	})

	return initNodes, err
}

func (n *Node) cfgError(format string, a ...interface{}) CfgError {
	a = append([]interface{}{n.fset.Position(n.pos)}, a...)
	return CfgError(fmt.Errorf("%s: "+format, a...))
}

func genRun(n *Node) error {
	var err CfgError

	n.Walk(func(n *Node) bool {
		if err != nil {
			return false
		}
		switch n.kind {
		case File:
			if n.types, err = frameTypes(n, n.fsize); err != nil {
				return false
			}
		case FuncDecl, FuncLit:
			if n.types, err = frameTypes(n, n.flen); err != nil {
				return false
			}
		case FuncType:
			if len(n.anc.child) == 4 {
				setExec(n.anc.child[3].start)
			}
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
	case SelectorExpr:
		pkg, name := n.child[0].ident, n.child[1].ident
		if p, ok := n.interp.binType[pkg]; ok && p[name] != nil {
			return true // Imported binary type
		}
		if p, ok := n.interp.scope[pkg]; ok && p.sym[name] != nil {
			return true // Imported source type
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
			n.child[i-1].tnext = n.child[i].start
		}
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
	if n.anc.kind == ValueSpec && n.anc.child[len(n.anc.child)-1] != n {
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

func getReturnedType(n *Node) *Type {
	switch n.typ.cat {
	case BuiltinT:
		return n.anc.typ
	case ValueT:
		//log.Println(n.index, "getReturnedType", n.typ.rtype)
		if n.typ.rtype.NumOut() > 0 {
			return &Type{cat: ValueT, rtype: n.typ.rtype.Out(0)}
		}
		return &Type{cat: ValueT, rtype: n.typ.rtype}
	}
	return n.typ.ret[0]
}

// frameTypes returns a slice of frame types for FuncDecl or FuncLit nodes
func frameTypes(node *Node, size int) ([]reflect.Type, error) {
	ft := make([]reflect.Type, size)
	var err CfgError

	node.Walk(func(n *Node) bool {
		if err != nil {
			return false
		}
		if n.kind == FuncDecl || n.kind == ImportDecl || n.kind == TypeDecl || n.kind == FuncLit {
			return n == node // Do not dive in subtree, except if this is the entry point
		}
		if n.findex < 0 || n.typ == nil || n.level > 0 || n.kind == BasicLit || n.kind == SelectorSrc || n.typ.cat == BinPkgT {
			return true
		}
		if ft[n.findex] == nil {
			if n.typ.incomplete {
				if n.typ, err = n.typ.finalize(); err != nil {
					return false
				}
			}
			if n.typ.cat == FuncT {
				ft[n.findex] = reflect.TypeOf(n)
			} else {
				ft[n.findex] = n.typ.TypeOf()
			}
		}
		// TODO: Check that type is identical
		return true
	}, nil)

	return ft, err
}
