package interp

import (
	"log"
	"path"
	"reflect"
	"unicode"
)

// Symbol type defines symbol values entities
type Symbol struct {
	typ    *Type       // Type of value
	node   *Node       // Node value if index is negative
	index  int         // index of value in frame or -1
	pkgbin *SymMap     // Map of package symbols if typ.cat is BinPkgT, or nil
	pkgsrc *NodeMap    // Map of package symbols if typ.cat is SrcPkgT, or nil
	bin    interface{} // Symbol from imported bin package if typ.cat is BinT, or nil
}

// Scope type stores the list of visible symbols at current scope level
type Scope struct {
	anc   *Scope             // Ancestor upper scope
	level int                // Frame level: number of frame indirections to access var
	sym   map[string]*Symbol // Symbol table indexed by idents
}

// Create a new scope and chain it to the current one
func (s *Scope) push(indirect int) *Scope {
	return &Scope{anc: s, level: s.level + indirect, sym: map[string]*Symbol{}}
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

// FrameIndex type defines metadata for Tracking frame index for variables in function context
type FrameIndex struct {
	anc *FrameIndex // Ancestor upper frame
	max int         // The highest index in frame
}

// Cfg generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables. A list of nodes of init functions is returned.
// Following this pass, the CFG is ready to run
func (interp *Interpreter) Cfg(root *Node, sdef *NodeMap) []*Node {
	scope := &Scope{sym: map[string]*Symbol{}}
	frameIndex := &FrameIndex{}
	var loop, loopRestart *Node
	var funcDef bool // True if a function is defined in the current frame context
	var initNodes []*Node
	var exports *SymMap
	var expval *ValueMap
	var iotaValue int

	// Fill root scope with initial symbol definitions
	for name, node := range *sdef {
		scope.sym[name] = &Symbol{node: node, index: -1}
	}

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
			pkgName := n.child[0].ident
			if pkg, ok := interp.Exports[pkgName]; ok {
				exports = pkg
				expval = interp.Expval[pkgName]
			} else {
				x := make(SymMap)
				exports = &x
				interp.Exports[pkgName] = exports
				y := make(ValueMap)
				expval = &y
				interp.Expval[pkgName] = expval
			}

		case For0, ForRangeStmt:
			loop, loopRestart = n, n.child[0]
			scope = scope.push(0)

		case For1, For2, For3, For3a, For4:
			loop, loopRestart = n, n.child[len(n.child)-1]
			scope = scope.push(0)

		case FuncDecl, FuncLit:
			// Add a frame indirection level as we enter in a func
			frameIndex = &FrameIndex{anc: frameIndex}
			scope = scope.push(1)
			if n.child[1].ident == "init" {
				initNodes = append(initNodes, n)
			}
			if len(n.child[0].child) > 0 {
				// function is a method, add it to the related type
				var t *Type
				var tname string
				n.ident = n.child[1].ident
				recv := n.child[0].child[0]
				if len(recv.child) < 2 {
					// Receiver var name is skipped in method declaration (fix that in AST ?)
					tname = recv.child[0].ident
				} else {
					tname = recv.child[1].ident
				}
				if tname == "" {
					tname = recv.child[1].child[0].ident
					elemtype := interp.types[tname]
					t = &Type{cat: PtrT, val: elemtype}
					elemtype.method = append(elemtype.method, n)
				} else {
					t = interp.types[tname]
				}
				t.method = append(t.method, n)
			}
			if len(n.child[2].child) == 2 {
				// allocate entries for return values at start of frame
				frameIndex.max += len(n.child[2].child[1].child)
			}
			funcDef = false

		case If0, If1, If2, If3:
			scope = scope.push(0)

		case Switch0:
			// Make sure default clause is in last position
			c := n.child[1].child
			if i, l := getDefault(n), len(c)-1; i >= 0 && i != l {
				c[i], c[l] = c[l], c[i]
			}
			scope = scope.push(0)

		case TypeSpec:
			// Type analysis is performed recursively and no post-order processing
			// needs to be done for types, so do not dive in subtree
			typeName := n.child[0].ident
			if n.child[1].kind == Ident {
				// Create a type alias of an existing one
				interp.types[typeName] = &Type{cat: AliasT, val: nodeType(interp.types, n.child[1])}
			} else {
				// Define a new type
				interp.types[typeName] = nodeType(interp.types, n.child[1])
			}
			//if canExport(typeName) {
			//	(*exports)[funcName] = reflect.MakeFunc(n.child[2].typ.TypeOf(), n.wrapNode).Interface()
			//}
			return false

		case ArrayType, BasicLit, ChanType, MapType, StructType:
			n.typ = nodeType(interp.types, n)
			return false
		}
		return true
	}, func(n *Node) {
		// Post-order processing
		switch n.kind {
		case Address:
			wireChild(n)
			n.typ = &Type{cat: PtrT, val: n.child[0].typ}

		case ArrayType:
			// TODO: move to pre-processing ? See when handling complex array type def
			n.typ = &Type{cat: ArrayT, val: interp.types[n.child[1].ident]}

		case Define, AssignStmt:
			wireChild(n)
			if n.kind == Define || n.anc.kind == VarDecl {
				// Force definition of assigned ident in current scope
				frameIndex.max++
				scope.sym[n.child[0].ident] = &Symbol{index: frameIndex.max}
				n.child[0].findex = frameIndex.max
				n.child[0].typ = n.child[1].typ
			}
			n.findex = n.child[0].findex
			// Propagate type
			// TODO: Check that existing destination type matches source type
			//log.Println(n.index, "Assign child1:", n.child[1].index, n.child[1].typ)
			n.typ = n.child[0].typ
			//n.run = setInt // Temporary, debug
			if sym, level, ok := scope.lookup(n.child[0].ident); ok {
				sym.typ = n.typ
				n.level = level
			}
			// If LHS is an indirection, get reference instead of value, to allow setting
			if n.child[0].action == GetIndex {
				if n.child[0].child[0].typ.cat == MapT {
					n.child[0].run = getMap
					n.run = assignMap
				} else if n.child[0].child[0].typ.cat == PtrT {
					// Handle the case where the receiver is a pointer to an object
					n.child[0].run = getPtrIndexAddr
					n.run = assignPtrField
				} else {
					n.child[0].run = getIndexAddr
					n.run = assignField
				}
			} else if n.child[0].action == Star {
				n.findex = n.child[0].child[0].findex
				n.run = indirectAssign
			}
			if n.anc.kind == ConstDecl {
				iotaValue++
			}

		case IncDecStmt:
			wireChild(n)
			n.findex = n.child[0].findex
			n.child[0].typ = interp.types["int"]
			n.typ = n.child[0].typ
			if sym, level, ok := scope.lookup(n.child[0].ident); ok {
				sym.typ = n.typ
				n.level = level
			}
			if n.child[0].action == Star {
				n.findex = n.child[0].child[0].findex
				n.run = indirectInc
			}

		case DefineX, AssignXStmt:
			wireChild(n)
			l := len(n.child) - 1
			if n.kind == DefineX {
				// Force definition of assigned idents in current scope
				for _, c := range n.child[:l] {
					frameIndex.max++
					scope.sym[c.ident] = &Symbol{index: frameIndex.max}
					c.findex = frameIndex.max
				}
			}

		case BinaryExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.child[0].typ

		case IndexExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.child[0].typ.val
			n.recv = n
			if n.child[0].typ.cat == MapT {
				n.run = getIndexMap
			}

		case BlockStmt:
			wireChild(n)
			n.findex = n.child[len(n.child)-1].findex
			scope = scope.anc

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
			frameIndex.max++
			n.findex = frameIndex.max
			if builtin, ok := goBuiltin[n.child[0].ident]; ok {
				n.run = builtin
				n.child[0].typ = &Type{cat: BuiltinT}
				if n.child[0].ident == "make" {
					if n.typ = interp.types[n.child[1].ident]; n.typ == nil {
						n.typ = nodeType(interp.types, n.child[1])
					}
					n.child[1].val = n.typ
					n.child[1].kind = BasicLit
				}
			}
			// TODO: Should process according to child type, not kind.
			if n.child[0].kind == SelectorImport {
				n.fsize = n.child[0].fsize
				rtype := n.child[0].val.(reflect.Value).Type()
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
					n.run = callBinX
				} else {
					n.run = callBin
				}
			} else if n.child[0].kind == SelectorExpr {
				if n.child[0].child[0].typ.cat == ValueT {
					n.run = callBinMethod
					// TODO: handle multiple return value
					n.child[0].kind = BasicLit // Temporary hack for force value() to return n.val at run
					//recv, methodName := n.child[0].child[0], n.child[0].child[1].ident
					//log.Println(n.index, "callexpr bin", recv.typ.rtype, methodName)
					//if method, found := recv.typ.rtype.MethodByName(methodName); found {
					//	log.Println(n.index, "method", method)
					//} else {
					//n.typ = &Type{cat: ValueT, rtype: n.child[0].val.(reflect.Value).Type()}
					n.typ = &Type{cat: ValueT, rtype: n.child[0].typ.rtype}
					n.fsize = n.child[0].fsize
					//}
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
							n.typ = interp.types[def.child[2].child[j].child[0].child[0].ident]
						}
						n.fsize = l
					}
				} else {
					// Resolve method and receiver path, store them in node static value for run
					if methodDecl := n.child[0].val.(*Node); len(methodDecl.child[2].child) > 1 {
						// Allocate frame for method return values (if any)
						n.fsize = len(methodDecl.child[2].child[1].child)
					} else {
						n.fsize = 0
					}
					n.child[0].findex = -1 // To force reading value from node instead of frame (methods)
				}
			} else if sym, _, _ := scope.lookup(n.child[0].ident); sym != nil {
				if sym.typ != nil && sym.typ.cat == BinT {
					n.run = callBin
					n.typ = &Type{cat: ValueT}
					n.child[0].fsize = reflect.TypeOf(sym.bin).NumOut()
					n.child[0].val = reflect.ValueOf(sym.bin)
					n.child[0].kind = BasicLit
				} else if sym.typ != nil && sym.typ.cat == ValueT {
					n.run = callDirectBin
					n.typ = &Type{cat: ValueT}
				} else {
					n.val = sym.node
					if def := n.val.(*Node); def != nil {
						// Reserve as many frame entries as nb of ret values for called function
						// node frame index should point to the first entry
						j := len(def.child[2].child) - 1
						l := len(def.child[2].child[j].child) // Number of return values for def
						if l == 1 {
							// If def returns exactly one value, propagate its type in call node.
							// Multiple return values will be handled differently through AssignX.
							n.typ = interp.types[def.child[2].child[j].child[0].child[0].ident]
						}
						n.fsize = l
					}
				}
			}
			// Reserve entries in frame to store results of call
			frameIndex.max += n.fsize
			if funcDef {
				// Trigger frame indirection to handle nested functions
				n.action = CallF
			}
			//log.Println(n.index, "callExpr:", n.child[0].ident, "frame index:", n.findex)

		case CaseClause:
			frameIndex.max++
			n.findex = frameIndex.max
			n.tnext = n.child[len(n.child)-1].start

		case CompositeLitExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			if n.child[0].typ == nil {
				n.child[0].typ = interp.types[n.child[0].ident]
			}
			// TODO: Check that composite litteral expr matches corresponding type
			n.typ = n.child[0].typ
			switch n.typ.cat {
			case ArrayT:
				n.run = arrayLit
			case MapT:
				n.run = mapLit
			case StructT:
				n.action, n.run = CompositeLit, compositeLit
				// Handle object assign from sparse key / values
				if len(n.child) > 1 && n.child[1].kind == KeyValueExpr {
					n.run = compositeSparse
					n.typ = interp.types[n.child[0].ident]
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
			n.typ = nodeType(interp.types, n.child[l])
			if l == 0 {
				frameIndex.max++
				n.findex = frameIndex.max
			} else {
				for _, f := range n.child[:l] {
					scope.sym[f.ident].typ = n.typ
				}
			}

		case File:
			wireChild(n)
			n.fsize = frameIndex.max + 1

		case For0: // for {}
			body := n.child[0]
			n.start = body.start
			body.tnext = n.start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For1: // for cond {}
			cond, body := n.child[0], n.child[1]
			n.start = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For2: // for init; cond; {}
			init, cond, body := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For3: // for ; cond; post {}
			cond, post, body := n.child[0], n.child[1], n.child[2]
			n.start = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = post.start
			post.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For3a: // for int; ; post {}
			init, post, body := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			init.tnext = body.start
			body.tnext = post.start
			post.tnext = body.start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For4: // for init; cond; post {}
			init, cond, post, body := n.child[0], n.child[1], n.child[2], n.child[3]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = body.start
			cond.fnext = n
			body.tnext = post.start
			post.tnext = cond.start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case ForRangeStmt:
			loop, loopRestart = nil, nil
			n.start = n.child[0].start
			n.findex = n.child[0].findex
			scope = scope.anc

		case FuncDecl:
			n.findex = frameIndex.max + 1
			if len(n.child[0].child) > 0 {
				// Store receiver frame location (used at run)
				n.child[0].findex = n.child[0].child[0].child[0].findex
			}
			scope = scope.anc
			frameIndex = frameIndex.anc
			funcName := n.child[1].ident
			if canExport(funcName) {
				(*exports)[funcName] = reflect.MakeFunc(n.child[2].typ.TypeOf(), n.wrapNode).Interface()
				(*expval)[funcName] = reflect.MakeFunc(n.child[2].typ.TypeOf(), n.wrapNode)
			}
			n.typ = n.child[2].typ
			n.val = n
			scope.sym[funcName].typ = n.typ

		case FuncLit:
			n.typ = n.child[2].typ
			n.val = n
			n.findex = -1
			n.findex = frameIndex.max + 1
			scope = scope.anc
			frameIndex = frameIndex.anc
			funcDef = true

		case FuncType:
			n.typ = nodeType(interp.types, n)
			// Store list of parameter frame indices in params val
			var list []int
			for _, c := range n.child[0].child {
				for _, f := range c.child[:len(c.child)-1] {
					list = append(list, f.findex)
				}
			}
			n.child[0].val = list
			// TODO: do the same for return values

		case GoStmt:
			wireChild(n)
			// TODO: should error if call expression refers to a builtin
			n.child[0].run = callGoRoutine

		case Ident:
			if n.ident == "false" {
				n.val = false
				n.typ = defaultTypes["bool"]
				n.kind = BasicLit
			} else if n.ident == "true" {
				n.val = true
				n.typ = defaultTypes["bool"]
				n.kind = BasicLit
			} else if n.ident == "iota" {
				n.val = iotaValue
				n.typ = defaultTypes["int"]
				n.kind = BasicLit
			} else if sym, level, ok := scope.lookup(n.ident); ok {
				n.typ, n.findex, n.level = sym.typ, sym.index, level
				if n.findex < 0 {
					n.val = sym.node
				} else if n.typ != nil {
					if n.typ.cat == BinPkgT {
						n.val = sym.pkgbin
					} else if n.typ.cat == SrcPkgT {
						log.Println(n.index, "ident SrcPkgT", n.ident)
						n.val = sym.pkgsrc
					}
				}
				n.recv = n
			} else {
				frameIndex.max++
				scope.sym[n.ident] = &Symbol{index: frameIndex.max}
				n.findex = frameIndex.max
			}

		case If0: // if cond {}
			cond, tbody := n.child[0], n.child[1]
			n.start = cond.start
			cond.tnext = tbody.start
			cond.fnext = n
			tbody.tnext = n
			scope = scope.anc

		case If1: // if cond {} else {}
			cond, tbody, fbody := n.child[0], n.child[1], n.child[2]
			n.start = cond.start
			cond.tnext = tbody.start
			cond.fnext = fbody.start
			tbody.tnext = n
			fbody.tnext = n
			scope = scope.anc

		case If2: // if init; cond {}
			init, cond, tbody := n.child[0], n.child[1], n.child[2]
			n.start = init.start
			tbody.tnext = n
			init.tnext = cond.start
			cond.tnext = tbody.start
			cond.fnext = n
			scope = scope.anc

		case If3: // if init; cond {} else {}
			init, cond, tbody, fbody := n.child[0], n.child[1], n.child[2], n.child[3]
			n.start = init.start
			init.tnext = cond.start
			cond.tnext = tbody.start
			cond.fnext = fbody.start
			tbody.tnext = n
			fbody.tnext = n
			scope = scope.anc

		case ImportSpec:
			var name, ipath string
			if len(n.child) == 2 {
				ipath = n.child[1].val.(string)
				name = n.child[0].ident
			} else {
				ipath = n.child[0].val.(string)
				name = path.Base(ipath)
			}
			if pkg, ok := interp.binPkg[ipath]; ok {
				if name == "." {
					for n, s := range *pkg {
						scope.sym[n] = &Symbol{typ: &Type{cat: BinT}, bin: s}
					}
				} else {
					scope.sym[name] = &Symbol{typ: &Type{cat: BinPkgT}, pkgbin: pkg}
				}
			} else {
				// TODO: make sure we do not import a src package more than once
				interp.importSrcFile(ipath)
				scope.sym[name] = &Symbol{typ: &Type{cat: SrcPkgT}, pkgsrc: interp.srcPkg[name]}
			}

		case KeyValueExpr:
			wireChild(n)

		case LandExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n.child[1].start
			n.child[0].fnext = n
			n.child[1].tnext = n
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.child[0].typ

		case LorExpr:
			n.start = n.child[0].start
			n.child[0].tnext = n
			n.child[0].fnext = n.child[1].start
			n.child[1].tnext = n
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.child[0].typ

		case RangeStmt:
			n.start = n
			n.child[3].tnext = n
			n.tnext = n.child[3].start
			frameIndex.max++
			n.findex = frameIndex.max

		case ReturnStmt:
			wireChild(n)
			n.tnext = nil

		case SelectorExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.child[0].typ
			n.recv = n.child[0].recv
			if n.typ == nil {
				log.Fatal("typ should not be nil:", n.index)
			}
			if n.typ.cat == ValueT {
				// Handle object defined in runtime
				log.Println(n.index, "selector", n.typ.cat)
				if method, ok := n.typ.rtype.MethodByName(n.child[1].ident); ok {
					log.Println(n.index, "selector method", method.Func, n.typ.rtype)
					if method.Func.IsValid() {
						n.rval = method.Func
						n.run = nop
					} else {
						n.val = method.Index
						//n.run = getIndexBinMethod
						n.run = nop
					}
					n.fsize = method.Type.NumOut()
				}
			} else if n.typ.cat == PtrT && n.typ.val.cat == ValueT {
				// Handle pointer on object defined in runtime
				if field, ok := n.typ.val.rtype.FieldByName(n.child[1].ident); ok {
					n.typ = &Type{cat: ValueT, rtype: field.Type}
					n.val = field.Index
					n.run = getPtrIndexBin
				} else if method, ok := n.typ.val.rtype.MethodByName(n.child[1].ident); ok {
					n.val = method.Func
					n.fsize = method.Type.NumOut()
					n.run = nop
				}
			} else if n.typ.cat == BinPkgT {
				// Resolve binary package symbol
				name := n.child[1].ident
				pkgSym := n.child[0].val.(*SymMap)
				if s, ok := (*pkgSym)[name]; ok {
					n.kind = SelectorImport
					n.val = reflect.ValueOf(s)
					if typ := reflect.TypeOf(s); typ.Kind() == reflect.Func {
						n.typ = &Type{cat: ValueT, rtype: typ}
						n.fsize = typ.NumOut()
					} else if typ.Kind() == reflect.Ptr {
						// a symbol of kind pointer must be dereferenced to access type
						n.typ = &Type{cat: ValueT, rtype: typ.Elem(), rzero: n.val.(reflect.Value).Elem()}
					} else {
						n.typ = &Type{cat: ValueT, rtype: typ}
						n.rval = n.val.(reflect.Value)
						n.kind = Rvalue
					}
					n.run = nop
				}
			} else if n.typ.cat == ArrayT {
				log.Println(n.index, "selector array", n.typ.val.cat)
				n.typ = n.typ.val
				n.run = nop
			} else if n.typ.cat == SrcPkgT {
				// Resolve source package symbol
				pkgSrc := n.child[0].val.(*NodeMap)
				name := n.child[1].ident
				if node, ok := (*pkgSrc)[name]; ok {
					log.Println(n.index, "src import sym", node.child[1].ident)
					n.val = node
					n.run = nop
					n.kind = SelectorSrc
				}
			} else if fi := n.typ.fieldIndex(n.child[1].ident); fi >= 0 {
				// Resolve struct field index
				if n.typ.cat == PtrT {
					n.run = getPtrIndex
				}
				n.typ = n.typ.fieldType(fi)
				n.child[1].kind = BasicLit
				n.child[1].val = fi
			} else if m, lind := n.typ.lookupMethod(n.child[1].ident); m != nil {
				// Handle method
				n.run = nop
				n.val = m
				n.child[1].val = lind
				n.typ = m.typ
			} else {
				// Handle promoted field in embedded struct
				if ti := n.typ.lookupField(n.child[1].ident); len(ti) > 0 {
					n.child[1].kind = BasicLit
					n.child[1].val = ti
					n.run = getIndexSeq
				} else {
					log.Println(n.index, "Selector not found:", n.child[1].ident)
					n.run = nop
					//panic("Field not found in selector")
				}
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
			scope = scope.anc

		case TypeAssertExpr:
			if n.child[1].typ == nil {
				n.child[1].typ = interp.types[n.child[1].ident]
			}
			log.Println(n.index, "TypeAssertExpr", n.child[0].typ.cat, n.child[1].ident, n.child[1].typ.cat)
			n.typ = n.child[1].typ

		case ValueSpec:
			l := len(n.child) - 1
			if n.typ = n.child[l].typ; n.typ == nil {
				n.typ = interp.types[n.child[l].ident]
			}
			for _, c := range n.child[:l] {
				c.typ = n.typ
				scope.sym[c.ident].typ = n.typ
			}
		}
	})
	return initNodes
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

// Wire AST nodes for CFG in subtree
func wireChild(n *Node) {
	// Set start node, in subtree (propagated to ancestors by post-order processing)
	for _, child := range n.child {
		switch child.kind {
		case ArrayType, ChanType, FuncDecl, MapType, BasicLit, FuncLit, Ident:
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
		case ArrayType, ChanType, MapType, FuncDecl, FuncLit, BasicLit, Ident:
			continue
		case Break, Continue, ReturnStmt:
			// tnext is already computed, no change
		default:
			n.child[i].tnext = n
		}
		break
	}
}

func canExport(name string) bool {
	if r := []rune(name); len(r) > 0 && unicode.IsUpper(r[0]) {
		return true
	}
	return false
}
