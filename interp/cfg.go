package interp

// TODO:
// - hierarchical scopes for symbol resolution
// - universe (global) scope
// - closures
// - &&, ||, break, continue, goto
// - slices / map expressions
// - switch
// - go routines
// - channels
// - select
// - import
// - type declarations and checking
// - type assertions and conversions
// - interfaces
// - pointers
// - diagnostics and proper error handling
// Done:
// - basic literals
// - variable definition and assignment
// - arithmetic and logical expressions
// - if / else statement, including init
// - for statement
// - variables definition (1 scope per function)
// - function definition
// - function calls
// - assignements, including to/from multi value
// - return, including multiple values
// - for range
// - arrays

// n.Cfg() generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables.
// Following this pass, the CFG is ready to run
func (e *Node) Cfg(def Def) int {
	symIndex := make(map[string]int)
	maxIndex := 0
	var loop, loopRestart *Node

	e.Walk(func(n *Node) {
		// Pre-order processing
		switch n.kind {
		case For0, ForRangeStmt:
			loop, loopRestart = n, n.Child[0]
		case For1, For2, For3, For4:
			loop, loopRestart = n, n.Child[len(n.Child)-1]
		case FuncDecl:
			symIndex = make(map[string]int)
			// allocate entries for return values at start of frame
			if len(n.Child[1].Child) == 2 {
				maxIndex = len(n.Child[1].Child[1].Child)
			} else {
				maxIndex = 0
			}
		}
	}, func(n *Node) {
		// Post-order processing
		switch n.kind {
		case FuncDecl:
			n.findex = maxIndex + 1
			//i.def[n.Child[0].ident] = n

		case BlockStmt, ExprStmt, ParenExpr:
			wireChild(n)
			n.findex = n.Child[len(n.Child)-1].findex

		case ReturnStmt:
			wireChild(n)

		case AssignStmt, IncDecStmt:
			wireChild(n)
			n.findex = n.Child[0].findex

		case BinaryExpr, CompositeLit, IndexExpr:
			wireChild(n)
			maxIndex++
			n.findex = maxIndex

		case Field:
			// A single child node (no ident, just type) means that the field refers
			// to a return value, and space on frame should be accordingly allocated.
			// Otherwise, just point to corresponding location in frame, resolved in
			// ident child.
			if len(n.Child) == 1 {
				maxIndex++
				n.findex = maxIndex
			} else {
				n.findex = n.Child[0].findex
			}

		case Break:
			n.tnext = loop

		case Continue:
			n.tnext = loopRestart

		case CallExpr:
			wireChild(n)
			// FIXME: should reserve as many entries as nb of ret values for called function
			// node frame index should point to the first entry
			maxIndex++
			n.findex = maxIndex
			n.val = def[n.Child[0].ident]

		case If0: // if cond {}
			cond, tbody := n.Child[0], n.Child[1]
			n.Start = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = n
			tbody.tnext = n

		case If1: // if cond {} else {}
			cond, tbody, fbody := n.Child[0], n.Child[1], n.Child[2]
			n.Start = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = fbody.Start
			tbody.tnext = n
			fbody.tnext = n

		case If2: // if init; cond {}
			init, cond, tbody := n.Child[0], n.Child[1], n.Child[2]
			n.Start = init.Start
			tbody.tnext = n
			init.tnext = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = n

		case If3: // if init; cond {} else {}
			init, cond, tbody, fbody := n.Child[0], n.Child[1], n.Child[2], n.Child[3]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = tbody.Start
			cond.fnext = fbody.Start
			tbody.tnext = n
			fbody.tnext = n

		case For0: // for {}
			body := n.Child[0]
			n.Start = body.Start
			body.tnext = n.Start
			loop, loopRestart = nil, nil

		case For1: // for cond {}
			cond, body := n.Child[0], n.Child[1]
			n.Start = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = cond.Start
			loop, loopRestart = nil, nil

		case For2: // for init; cond; {}
			init, cond, body := n.Child[0], n.Child[1], n.Child[2]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = cond.Start
			loop, loopRestart = nil, nil

		case For3: // for ; cond; post {}
			cond, post, body := n.Child[0], n.Child[1], n.Child[2]
			n.Start = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = post.Start
			post.tnext = cond.Start
			loop, loopRestart = nil, nil

		case For4: // for init; cond; post {}
			init, cond, post, body := n.Child[0], n.Child[1], n.Child[2], n.Child[3]
			n.Start = init.Start
			init.tnext = cond.Start
			cond.tnext = body.Start
			cond.fnext = n
			body.tnext = post.Start
			post.tnext = cond.Start
			loop, loopRestart = nil, nil

		case ForRangeStmt:
			loop, loopRestart = nil, nil
			n.Start = n.Child[0].Start
			n.findex = n.Child[0].findex

		case RangeStmt:
			n.Start = n
			n.Child[3].tnext = n
			n.tnext = n.Child[3].Start
			maxIndex++
			n.findex = maxIndex

		case Ident:
			// Lookup identifier in frame symbol table. If not found
			// should check if ident can be defined (assign, param passing...)
			// or should lookup in upper scope of variables
			// For now, simply allocate a new entry in local sym table
			if n.findex = symIndex[n.ident]; n.findex == 0 {
				maxIndex++
				symIndex[n.ident] = maxIndex
				n.findex = symIndex[n.ident]
			}
		}
	})
	return maxIndex + 1
}

// Wire AST nodes for CFG in subtree
func wireChild(n *Node) {
	// Set start node, in subtree (propagated to ancestors due to post-order processing)
	for _, child := range n.Child {
		switch child.kind {
		case ArrayType, BasicLit, Ident:
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
		case BasicLit, Ident:
			continue
		case Break, Continue:
			// Next is already computed, no change
		default:
			n.Child[i].tnext = n
		}
		break
	}
}

// optimisation: rewire CFG to skip nop nodes
func (e *Node) OptimCfg() {
	e.Walk(nil, func(n *Node) {
		for n.tnext != nil && n.tnext.action == Nop {
			n.tnext = n.tnext.tnext
		}
	})
}
