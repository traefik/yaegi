package interp

import (
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
)

// TODO:
// - hierarchical scopes for symbol resolution
// - universe (global) scope
// - closures
// - if init statement
// - &&, ||, break, continue, goto
// - for range
// - array / slices / map expressions
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
// - if / else statement
// - for statement
// - variables definition (1 scope per function)
// - function definition
// - function calls
// - assignements, including to/from multi value
// - return, including multiple values

// n.Cfg() generates a control flow graph (CFG) from AST (wiring successors in AST)
// and pre-compute frame sizes and indexes for all un-named (temporary) and named
// variables.
// Following this pass, the CFG is ready to be run
func (e *Node) Cfg(i *Interpreter) int {
	symIndex := make(map[string]int)
	maxIndex := 0

	e.Walk(func(n *Node) {
		// Pre-order processing
		switch (*n.anode).(type) {
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
			case token.LSS:
				n.run = lower
			case token.SUB:
				n.run = sub
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
			n.Child[1].tnext = n
			if len(n.Child) == 3 {
				n.Child[2].tnext = n
			}
			n.Child[0].tnext = n.Child[1].Start
			if len(n.Child) == 3 {
				n.Child[0].fnext = n.Child[2].Start
			} else {
				n.Child[0].fnext = n
			}

		case *ast.ForStmt:
			n.isNop = true
			n.run = nop
			// FIXME: works only if for node has 4 children
			n.Start = n.Child[0].Start
			n.Child[0].tnext = n.Child[1].Start
			n.Child[1].fnext = n
			n.Child[1].tnext = n.Child[3].Start
			n.Child[3].tnext = n.Child[2].Start
			n.Child[2].tnext = n.Child[1].Start

		case *ast.BasicLit:
			n.isConst = true
			// FIXME: values must be converted to int or float if possible
			if v, err := strconv.ParseInt(a.Value, 0, 0); err == nil {
				n.val = v
			} else {
				n.val = a.Value
			}

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
			println("unknown type:", reflect.TypeOf(*n.anode).String())
		}
	})
	return maxIndex + 1
}

// Wire AST nodes of sequential blocks
func wireChild(n *Node) {
	for _, child := range n.Child {
		if !child.isLeaf() {
			n.Start = child.Start
			break
		}
	}
	if n.Start == nil {
		n.Start = n
	}
	for i := 1; i < len(n.Child); i++ {
		n.Child[i-1].tnext = n.Child[i].Start
	}
	for i := len(n.Child) - 1; i >= 0; i-- {
		if !n.Child[i].isLeaf() {
			n.Child[i].tnext = n
			break
		}
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
