package interp

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
)

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
func (e *Node) Cfg(i *Interpreter) int {
	symIndex := make(map[string]int)
	maxIndex := 0
	var loop *Node

	e.Walk(func(n *Node) {
		// Pre-order processing
		switch (*n.anode).(type) {
		case *ast.ForStmt:
			loop = n
		case *ast.FuncDecl:
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
		switch a := (*n.anode).(type) {
		case *ast.FuncDecl:
			n.findex = maxIndex + 1
			n.isConst = true
			i.def[n.Child[0].ident] = n

		case *ast.BlockStmt:
			wireChild(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isNop = true
			n.run = nop
			n.findex = n.Child[len(n.Child)-1].findex

		case *ast.ReturnStmt:
			wireChild(n)
			n.run = _return

		case *ast.IncDecStmt:
			wireChild(n)
			switch a.Tok {
			case token.INC:
				n.run = inc
			}
			n.findex = n.Child[0].findex

		case *ast.AssignStmt:
			if len(a.Lhs) > 1 && len(a.Rhs) == 1 {
				n.run = assignX
			} else {
				n.run = assign
			}
			wireChild(n)
			n.findex = n.Child[0].findex

		case *ast.ExprStmt:
			wireChild(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isNop = true
			n.run = nop
			n.findex = n.Child[len(n.Child)-1].findex

		case *ast.ParenExpr:
			wireChild(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isNop = true
			n.run = nop
			n.findex = n.Child[len(n.Child)-1].findex

		case *ast.BinaryExpr:
			wireChild(n)
			switch a.Op {
			case token.ADD:
				n.run = add
			case token.AND:
				n.run = and
			case token.EQL:
				n.run = equal
			case token.GTR:
				n.run = greater
			case token.LSS:
				n.run = lower
			case token.SUB:
				n.run = sub
			default:
				panic("missing binary operator function")
			}
			maxIndex++
			n.findex = maxIndex

		case *ast.Field:
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

		case *ast.BranchStmt:
			// Break the current loop
			n.tnext = loop
			n.run = nop
			n.isNop = true

		case *ast.CallExpr:
			wireChild(n)
			// FIXME: should reserve as many entries as nb of ret values for called function
			// node frame index should point to the first entry
			n.run = i.call
			maxIndex++
			n.findex = maxIndex

		case *ast.IfStmt:
			n.isNop = true
			n.run = nop
			n.Start = n.Child[0].Start
			icond, ibody, ielse := 0, 1, 2
			if a.Init != nil {
				icond, ibody, ielse = 1, 2, 3
				n.Child[0].tnext = n.Child[1].Start
			}
			n.Child[ibody].tnext = n
			if a.Else != nil {
				n.Child[ielse].tnext = n
			}
			n.Child[icond].tnext = n.Child[ibody].Start
			if a.Else != nil {
				n.Child[icond].fnext = n.Child[ielse].Start
			} else {
				n.Child[icond].fnext = n
			}

		case *ast.ForStmt:
			n.isNop = true
			n.run = nop
			// Child indices for condition, body and post blocks
			icond, ibody, ipost := 0, 2, 1
			if a.Init != nil {
				icond, ibody, ipost = 1, 3, 2
			}
			n.Start = n.Child[0].Start
			if a.Cond != nil {
				n.Child[0].tnext = n.Child[icond].Start
				n.Child[icond].fnext = n
				n.Child[icond].tnext = n.Child[ibody].Start
				if a.Post != nil {
					n.Child[ibody].tnext = n.Child[ipost].Start
					n.Child[ipost].tnext = n.Child[icond].Start
				}
			} else {
				// no condition: Infinite loop
				n.Child[0].tnext = n.Child[0].Start
			}
			loop = nil

		case *ast.RangeStmt:
			n.Start = n
			n.Child[3].tnext = n
			n.tnext = n.Child[3].Start
			//  n.fnext set in wireChild() by ancestor
			n.run = _range
			maxIndex++
			n.findex = maxIndex

		case *ast.BasicLit:
			n.isConst = true
			// FIXME: values must be converted to int or float if possible
			if v, err := strconv.ParseInt(a.Value, 0, 0); err == nil {
				n.val = v
			} else {
				n.val = a.Value
			}

		case *ast.CompositeLit:
			wireChild(n)
			n.run = arrayLit
			maxIndex++
			n.findex = maxIndex

		case *ast.IndexExpr:
			wireChild(n)
			n.run = getIndex
			maxIndex++
			n.findex = maxIndex

		case *ast.Ident:
			// Lookup identifier in frame symbol table. If not found
			// should check if ident can be defined (assign, param passing...)
			// or should lookup in upper scope of variables
			// For now, simply allocate a new entry in local sym table
			n.ident = a.Name
			if n.findex = symIndex[n.ident]; n.findex == 0 {
				maxIndex++
				symIndex[n.ident] = maxIndex
				n.findex = symIndex[n.ident]
			}

		case *ast.FieldList:
		case *ast.FuncType:
		case *ast.File:
		default:
			fmt.Printf("unknown type: %T\n", *n.anode)
		}
	})
	return maxIndex + 1
}

// Wire AST nodes for CFG in subtree
func wireChild(n *Node) {
	// Find start node, in subtree
start:
	for _, child := range n.Child {
		switch (*child.anode).(type) {
		case *ast.BranchStmt:
			n.Start = child
		default:
			if len(child.Child) > 0 {
				n.Start = child.Start
				break start
			}
		}
	}
	if n.Start == nil {
		n.Start = n
	}
	// Chain sequential operations inside a block
	for i := 1; i < len(n.Child); i++ {
		switch (*n.Child[i-1].anode).(type) {
		case *ast.RangeStmt:
			n.Child[i-1].fnext = n.Child[i].Start
		default:
			n.Child[i-1].tnext = n.Child[i].Start
		}
	}
	// Chain block exit
	for i := len(n.Child) - 1; i >= 0; i-- {
		if len(n.Child[i].Child) == 0 {
			continue
		}
		switch (*n.Child[i].anode).(type) {
		case *ast.RangeStmt:
			// The exit node of a range statement is its ancestor
			n.Child[i].fnext = n
		default:
			n.Child[i].tnext = n
		}
		break
	}
}

// optimisation: rewire CFG to skip nop nodes
func (e *Node) OptimCfg() {
	e.Walk(nil, func(n *Node) {
		for n.tnext != nil && n.tnext.isNop {
			n.tnext = n.tnext.tnext
		}
	})
}
