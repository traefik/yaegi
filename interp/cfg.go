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

	// Fill root scope with initial symbol definitions
	for name, node := range *sdef {
		scope.sym[name] = &Symbol{node: node, index: -1}
	}

	root.Walk(func(n *Node) bool {
		// Pre-order processing
		switch n.kind {
		case Define, AssignStmt:
			if l := len(n.Child); l%2 == 1 {
				// Odd number of children: remove the type node, useless for assign
				i := l / 2
				n.Child = append(n.Child[:i], n.Child[i+1:]...)
			}

		case BlockStmt:
			scope = scope.push(0)

		case File:
			pkgName := n.Child[0].ident
			if pkg, ok := interp.Exports[pkgName]; ok {
				exports = pkg
			} else {
				x := make(SymMap)
				exports = &x
				interp.Exports[pkgName] = exports
			}

		case For0, ForRangeStmt:
			loop, loopRestart = n, n.Child[0]
			scope = scope.push(0)

		case For1, For2, For3, For3a, For4:
			loop, loopRestart = n, n.Child[len(n.Child)-1]
			scope = scope.push(0)

		case FuncDecl, FuncLit:
			// Add a frame indirection level as we enter in a func
			frameIndex = &FrameIndex{anc: frameIndex}
			scope = scope.push(1)
			if n.Child[1].ident == "init" {
				initNodes = append(initNodes, n)
			}
			if len(n.Child[0].Child) > 0 {
				// function is a method, add it to the related type
				var t *Type
				n.ident = n.Child[1].ident
				tname := n.Child[0].Child[0].Child[1].ident
				if tname == "" {
					tname = n.Child[0].Child[0].Child[1].Child[0].ident
					elemtype := interp.types[tname]
					t = &Type{cat: PtrT, val: elemtype}
					elemtype.method = append(elemtype.method, n)
				} else {
					t = interp.types[tname]
				}
				t.method = append(t.method, n)
			}
			if len(n.Child[2].Child) == 2 {
				// allocate entries for return values at start of frame
				frameIndex.max += len(n.Child[2].Child[1].Child)
			}
			funcDef = false

		case If0, If1, If2, If3:
			scope = scope.push(0)

		case Switch0:
			// Make sure default clause is in last position
			c := n.Child[1].Child
			if i, l := getDefault(n), len(c)-1; i >= 0 && i != l {
				c[i], c[l] = c[l], c[i]
			}
			scope = scope.push(0)

		case TypeSpec:
			// Type analysis is performed recursively and no post-order processing
			// needs to be done for types, so do not dive in subtree
			typeName := n.Child[0].ident
			if n.Child[1].kind == Ident {
				// Create a type alias of an existing one
				interp.types[typeName] = &Type{cat: AliasT, val: nodeType(interp.types, n.Child[1])}
			} else {
				// Define a new type
				interp.types[typeName] = nodeType(interp.types, n.Child[1])
			}
			//if canExport(typeName) {
			//	(*exports)[funcName] = reflect.MakeFunc(n.Child[2].typ.TypeOf(), n.wrapNode).Interface()
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
			n.typ = &Type{cat: PtrT, val: n.Child[0].typ}

		case ArrayType:
			// TODO: move to pre-processing ? See when handling complex array type def
			n.typ = &Type{cat: ArrayT, val: interp.types[n.Child[1].ident]}

		case Define, AssignStmt:
			wireChild(n)
			if n.kind == Define || n.anc.kind == GenDecl {
				// Force definition of assigned ident in current scope
				frameIndex.max++
				scope.sym[n.Child[0].ident] = &Symbol{index: frameIndex.max}
				n.Child[0].findex = frameIndex.max
				n.Child[0].typ = n.Child[1].typ
			}
			n.findex = n.Child[0].findex
			// Propagate type
			// TODO: Check that existing destination type matches source type
			//log.Println(n.index, "Assign child1:", n.Child[1].index, n.Child[1].typ)
			n.typ = n.Child[0].typ
			//n.run = setInt // Temporary, debug
			if sym, _, ok := scope.lookup(n.Child[0].ident); ok {
				sym.typ = n.typ
			}
			// If LHS is an indirection, get reference instead of value, to allow setting
			if n.Child[0].action == GetIndex {
				if n.Child[0].Child[0].typ.cat == MapT {
					n.Child[0].run = getMap
					n.run = assignMap
				} else if n.Child[0].Child[0].typ.cat == PtrT {
					// Handle the case where the receiver is a pointer to an object
					n.Child[0].run = getPtrIndexAddr
					n.run = assignPtrField
				} else {
					n.Child[0].run = getIndexAddr
					n.run = assignField
				}
			} else if n.Child[0].action == Star {
				n.findex = n.Child[0].Child[0].findex
				n.run = indirectAssign
			}

		case IncDecStmt:
			wireChild(n)
			n.findex = n.Child[0].findex
			n.Child[0].typ = interp.types["int"]
			n.typ = n.Child[0].typ
			if sym, _, ok := scope.lookup(n.Child[0].ident); ok {
				sym.typ = n.typ
			}
			if n.Child[0].action == Star {
				n.findex = n.Child[0].Child[0].findex
				n.run = indirectInc
			}

		case DefineX, AssignXStmt:
			wireChild(n)
			l := len(n.Child) - 1
			if n.kind == DefineX {
				// Force definition of assigned idents in current scope
				for _, c := range n.Child[:l] {
					frameIndex.max++
					scope.sym[c.ident] = &Symbol{index: frameIndex.max}
					c.findex = frameIndex.max
				}
			}

		case BinaryExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.Child[0].typ

		case IndexExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.Child[0].typ
			if n.typ.cat == MapT {
				n.run = getIndexMap
			}

		case BlockStmt:
			wireChild(n)
			n.findex = n.Child[len(n.Child)-1].findex
			scope = scope.anc

		case DeclStmt, ExprStmt, GenDecl, ParenExpr, SendStmt:
			wireChild(n)
			n.findex = n.Child[len(n.Child)-1].findex

		case Break:
			n.tnext = loop

		case CallExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			if builtin, ok := goBuiltin[n.Child[0].ident]; ok {
				n.run = builtin
				n.Child[0].typ = &Type{cat: BuiltinT}
				if n.Child[0].ident == "make" {
					if n.typ = interp.types[n.Child[1].ident]; n.typ == nil {
						n.typ = nodeType(interp.types, n.Child[1])
					}
					n.Child[1].val = n.typ
					n.Child[1].kind = BasicLit
				}
			}
			// TODO: Should process according to child type, not kind.
			if n.Child[0].kind == SelectorImport {
				n.fsize = n.Child[0].fsize
				rtype := n.Child[0].val.(reflect.Value).Type()
				if rtype.NumOut() > 0 {
					n.typ = &Type{cat: ValueT, rtype: rtype.Out(0)}
				}
				n.Child[0].kind = BasicLit
				for i, c := range n.Child[1:] {
					// Wrap function defintion so it can be called from runtime
					if c.kind == FuncLit {
						n.Child[1+i].rval = reflect.MakeFunc(rtype.In(i), c.wrapNode)
						n.Child[1+i].kind = Rvalue
					} else if c.ident == "nil" {
						n.Child[1+i].rval = reflect.New(rtype.In(i)).Elem()
						n.Child[1+i].kind = Rvalue
					} else if c.typ != nil && c.typ.cat == FuncT {
						n.Child[1+i].rval = reflect.MakeFunc(rtype.In(i), c.wrapNode)
						n.Child[1+i].kind = Rvalue
					}
				}
				// TODO: handle multiple return value
				if len(n.Child) == 2 && n.Child[1].fsize > 1 {
					n.run = callBinX
				} else {
					n.run = callBin
				}
			} else if n.Child[0].kind == SelectorExpr {
				if n.Child[0].Child[0].typ.cat == ValueT {
					n.run = callBinMethod
					// TODO: handle multiple return value
					n.Child[0].kind = BasicLit // Temporary hack for force value() to return n.val at run
					//recv, methodName := n.Child[0].Child[0], n.Child[0].Child[1].ident
					//log.Println(n.index, "callexpr bin", recv.typ.rtype, methodName)
					//if method, found := recv.typ.rtype.MethodByName(methodName); found {
					//	log.Println(n.index, "method", method)
					//} else {
					//n.typ = &Type{cat: ValueT, rtype: n.Child[0].val.(reflect.Value).Type()}
					n.typ = &Type{cat: ValueT, rtype: n.Child[0].typ.rtype}
					n.fsize = n.Child[0].fsize
					//}
				} else if n.Child[0].typ.cat == SrcPkgT {
					n.val = n.Child[0].val
					if def := n.val.(*Node); def != nil {
						// Reserve as many frame entries as nb of ret values for called function
						// node frame index should point to the first entry
						j := len(def.Child[2].Child) - 1
						l := len(def.Child[2].Child[j].Child) // Number of return values for def
						if l == 1 {
							// If def returns exactly one value, propagate its type in call node.
							// Multiple return values will be handled differently through AssignX.
							n.typ = interp.types[def.Child[2].Child[j].Child[0].Child[0].ident]
						}
						n.fsize = l
					}
				} else {
					// Resolve method and receiver path, store them in node static value for run
					n.Child[0].val, n.Child[0].Child[1].val = n.Child[0].Child[0].typ.lookupMethod(n.Child[0].Child[1].ident)
					if methodDecl := n.Child[0].val.(*Node); len(methodDecl.Child[2].Child) > 1 {
						// Allocate frame for method return values (if any)
						n.fsize = len(methodDecl.Child[2].Child[1].Child)
					} else {
						n.fsize = 0
					}
					n.Child[0].findex = -1 // To force reading value from node instead of frame (methods)
				}
			} else if sym, _, _ := scope.lookup(n.Child[0].ident); sym != nil {
				if sym.typ != nil && sym.typ.cat == BinT {
					n.run = callBin
					n.typ = &Type{cat: ValueT}
					n.Child[0].fsize = reflect.TypeOf(sym.bin).NumOut()
					n.Child[0].val = reflect.ValueOf(sym.bin)
					n.Child[0].kind = BasicLit
				} else if sym.typ != nil && sym.typ.cat == ValueT {
					n.run = callDirectBin
					n.typ = &Type{cat: ValueT}
				} else {
					n.val = sym.node
					if def := n.val.(*Node); def != nil {
						// Reserve as many frame entries as nb of ret values for called function
						// node frame index should point to the first entry
						j := len(def.Child[2].Child) - 1
						l := len(def.Child[2].Child[j].Child) // Number of return values for def
						if l == 1 {
							// If def returns exactly one value, propagate its type in call node.
							// Multiple return values will be handled differently through AssignX.
							n.typ = interp.types[def.Child[2].Child[j].Child[0].Child[0].ident]
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
			//log.Println(n.index, "callExpr:", n.Child[0].ident, "frame index:", n.findex)

		case CaseClause:
			frameIndex.max++
			n.findex = frameIndex.max
			n.tnext = n.Child[len(n.Child)-1].Start

		case CompositeLitExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			if n.Child[0].typ == nil {
				n.Child[0].typ = interp.types[n.Child[0].ident]
			}
			// TODO: Check that composite litteral expr matches corresponding type
			n.typ = n.Child[0].typ
			switch n.typ.cat {
			case ArrayT:
				n.run = arrayLit
			case MapT:
				n.run = mapLit
			case StructT:
				n.action, n.run = CompositeLit, compositeLit
				// Handle object assign from sparse key / values
				if len(n.Child) > 1 && n.Child[1].kind == KeyValueExpr {
					n.run = compositeSparse
					n.typ = interp.types[n.Child[0].ident]
					for _, c := range n.Child[1:] {
						c.findex = n.typ.fieldIndex(c.Child[0].ident)
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
			if len(n.Child) == 1 {
				frameIndex.max++
				n.findex = frameIndex.max
			} else {
				l := len(n.Child) - 1
				t := nodeType(interp.types, n.Child[l])
				for _, f := range n.Child[:l] {
					scope.sym[f.ident].typ = t
				}
			}

		case File:
			wireChild(n)
			n.fsize = frameIndex.max + 1

		case For0: // for {}
			body := n.Child[0]
			n.Start = body.Start
			body.tnext = n.Start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For1: // for cond {}
			cond, body := n.Child[0], n.Child[1]
			n.Start = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = cond.Start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For2: // for init; cond; {}
			init, cond, body := n.Child[0], n.Child[1], n.Child[2]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = cond.Start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For3: // for ; cond; post {}
			cond, post, body := n.Child[0], n.Child[1], n.Child[2]
			n.Start = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = post.Start
			post.tnext = cond.Start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For3a: // for int; ; post {}
			init, post, body := n.Child[0], n.Child[1], n.Child[2]
			n.Start = init.Start
			init.tnext = body.Start
			body.tnext = post.Start
			post.tnext = body.Start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case For4: // for init; cond; post {}
			init, cond, post, body := n.Child[0], n.Child[1], n.Child[2], n.Child[3]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = post.Start
			post.tnext = cond.Start
			loop, loopRestart = nil, nil
			scope = scope.anc

		case ForRangeStmt:
			loop, loopRestart = nil, nil
			n.Start = n.Child[0].Start
			n.findex = n.Child[0].findex
			scope = scope.anc

		case FuncDecl:
			n.findex = frameIndex.max + 1
			if len(n.Child[0].Child) > 0 {
				// Store receiver frame location (used at run)
				n.Child[0].findex = n.Child[0].Child[0].Child[0].findex
			}
			scope = scope.anc
			frameIndex = frameIndex.anc
			funcName := n.Child[1].ident
			if canExport(funcName) {
				(*exports)[funcName] = reflect.MakeFunc(n.Child[2].typ.TypeOf(), n.wrapNode).Interface()
			}
			n.typ = n.Child[2].typ
			n.val = n
			scope.sym[funcName].typ = n.typ

		case FuncLit:
			n.typ = n.Child[2].typ
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
			for _, c := range n.Child[0].Child {
				for _, f := range c.Child[:len(c.Child)-1] {
					list = append(list, f.findex)
				}
			}
			n.Child[0].val = list
			// TODO: do the same for return values

		case GoStmt:
			wireChild(n)
			// TODO: should error if call expression refers to a builtin
			n.Child[0].run = callGoRoutine

		case Ident:
			if sym, level, ok := scope.lookup(n.ident); ok {
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
			cond, tbody := n.Child[0], n.Child[1]
			n.Start = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = n
			tbody.tnext = n
			scope = scope.anc

		case If1: // if cond {} else {}
			cond, tbody, fbody := n.Child[0], n.Child[1], n.Child[2]
			n.Start = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = fbody.Start
			tbody.tnext = n
			fbody.tnext = n
			scope = scope.anc

		case If2: // if init; cond {}
			init, cond, tbody := n.Child[0], n.Child[1], n.Child[2]
			n.Start = init.Start
			tbody.tnext = n
			init.tnext = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = n
			scope = scope.anc

		case If3: // if init; cond {} else {}
			init, cond, tbody, fbody := n.Child[0], n.Child[1], n.Child[2], n.Child[3]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = fbody.Start
			tbody.tnext = n
			fbody.tnext = n
			scope = scope.anc

		case ImportSpec:
			var name, ipath string
			if len(n.Child) == 2 {
				ipath = n.Child[1].val.(string)
				name = n.Child[0].ident
			} else {
				ipath = n.Child[0].val.(string)
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
			n.Start = n.Child[0].Start
			n.Child[0].tnext = n.Child[1].Start
			n.Child[0].fnext = n
			n.Child[1].tnext = n
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.Child[0].typ

		case LorExpr:
			n.Start = n.Child[0].Start
			n.Child[0].tnext = n
			n.Child[0].fnext = n.Child[1].Start
			n.Child[1].tnext = n
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.Child[0].typ

		case RangeStmt:
			n.Start = n
			n.Child[3].tnext = n
			n.tnext = n.Child[3].Start
			frameIndex.max++
			n.findex = frameIndex.max

		case ReturnStmt:
			wireChild(n)
			n.tnext = nil

		case SelectorExpr:
			wireChild(n)
			frameIndex.max++
			n.findex = frameIndex.max
			n.typ = n.Child[0].typ
			n.recv = n.Child[0].recv
			if n.typ == nil {
				log.Fatal("typ should not be nil:", n.index)
			}
			if n.typ.cat == ValueT {
				// Handle object defined in runtime
				log.Println(n.index, "selector", n.typ.cat)
				if method, ok := n.typ.rtype.MethodByName(n.Child[1].ident); ok {
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
				if field, ok := n.typ.val.rtype.FieldByName(n.Child[1].ident); ok {
					n.typ = &Type{cat: ValueT, rtype: field.Type}
					n.val = field.Index
					n.run = getPtrIndexBin
				} else if method, ok := n.typ.val.rtype.MethodByName(n.Child[1].ident); ok {
					n.val = method.Func
					n.fsize = method.Type.NumOut()
					n.run = nop
				}
			} else if n.typ.cat == BinPkgT {
				// Resolve binary package symbol
				name := n.Child[1].ident
				pkgSym := n.Child[0].val.(*SymMap)
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
			} else if n.typ.cat == SrcPkgT {
				// Resolve source package symbol
				log.Println(n.index, "selector srcpkg", n.Child[0].ident, n.Child[1].ident)
				pkgSrc := n.Child[0].val.(*NodeMap)
				name := n.Child[1].ident
				if node, ok := (*pkgSrc)[name]; ok {
					log.Println(n.index, "src import sym", node.Child[1].ident)
					n.val = node
					n.run = nop
					n.kind = SelectorSrc
				}
			} else if fi := n.typ.fieldIndex(n.Child[1].ident); fi >= 0 {
				// Resolve struct field index
				if n.typ.cat == PtrT {
					n.run = getPtrIndex
				}
				n.typ = n.typ.fieldType(fi)
				n.Child[1].kind = BasicLit
				n.Child[1].val = fi
			} else if m, lind := n.typ.lookupMethod(n.Child[1].ident); m != nil {
				// Handle method
				n.run = nop
				n.val = m
				n.Child[1].val = lind
				n.typ = m.typ
			} else {
				// Handle promoted field in embedded struct
				if ti := n.typ.lookupField(n.Child[1].ident); len(ti) > 0 {
					n.Child[1].kind = BasicLit
					n.Child[1].val = ti
					n.run = getIndexSeq
				} else {
					//log.Println(n.index, "Selector not found:", n.Child[1].ident)
					n.run = nop
					//panic("Field not found in selector")
				}
			}

		case Switch0:
			n.Start = n.Child[1].Start
			// Chain case clauses
			clauses := n.Child[1].Child
			l := len(clauses)
			for i, c := range clauses[:l-1] {
				// chain to next clause
				c.tnext = c.Child[1].Start
				c.Child[1].tnext = n
				c.fnext = clauses[i+1]
			}
			// Handle last clause
			if c := clauses[l-1]; len(c.Child) > 1 {
				// No default clause
				c.tnext = c.Child[1].Start
				c.fnext = n
				c.Child[1].tnext = n
			} else {
				// Default
				c.tnext = c.Child[0].Start
				c.Child[0].tnext = n
			}
			scope = scope.anc

		case TypeAssertExpr:
			if n.Child[1].typ == nil {
				n.Child[1].typ = interp.types[n.Child[1].ident]
			}
			log.Println(n.index, "TypeAssertExpr", n.Child[0].typ.cat, n.Child[1].ident, n.Child[1].typ.cat)
			n.typ = n.Child[1].typ

		case ValueSpec:
			l := len(n.Child) - 1
			if n.typ = n.Child[l].typ; n.typ == nil {
				n.typ = interp.types[n.Child[l].ident]
			}
			for _, c := range n.Child[:l] {
				c.typ = n.typ
				scope.sym[c.ident].typ = n.typ
			}
		}
	})
	return initNodes
}

// Find default case clause index of a switch statement, if any
func getDefault(n *Node) int {
	for i, c := range n.Child[1].Child {
		if len(c.Child) == 1 {
			return i
		}
	}
	return -1
}

// Wire AST nodes for CFG in subtree
func wireChild(n *Node) {
	// Set start node, in subtree (propagated to ancestors by post-order processing)
	for _, child := range n.Child {
		switch child.kind {
		case ArrayType, ChanType, FuncDecl, MapType, BasicLit, FuncLit, Ident:
			continue
		default:
			n.Start = child.Start
		}
		break
	}

	// Chain sequential operations inside a block (next is right sibling)
	for i := 1; i < len(n.Child); i++ {
		n.Child[i-1].tnext = n.Child[i].Start
	}

	// Chain subtree next to self
	for i := len(n.Child) - 1; i >= 0; i-- {
		switch n.Child[i].kind {
		case ArrayType, ChanType, MapType, FuncDecl, FuncLit, BasicLit, Ident:
			continue
		case Break, Continue, ReturnStmt:
			// tnext is already computed, no change
		default:
			n.Child[i].tnext = n
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
