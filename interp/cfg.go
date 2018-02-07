package interp

import (
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
)

// TODO: remove coupling with go/ast and go/token. This should be handled only in Ast()

// n.Cfg() generates a control flow graph (CFG) from AST (wiring successors in AST)
func (e *Node) Cfg(i *Interpreter) int {
	symIndex := make(map[string]int)
	maxIndex := 0

	e.Walk(func(n *Node) {
		switch (*n.anode).(type) {
		case *ast.FuncDecl:
			symIndex = make(map[string]int)
			maxIndex = 0
		}
	}, func(n *Node) {
		switch x := (*n.anode).(type) {
		case *ast.FuncDecl:
			n.findex = maxIndex
			n.isConst = true
			// Node value is func body entry point
			n.val = n.Child[2]
			i.def[n.Child[0].ident] = n
		case *ast.BlockStmt:
			wireChild(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isNop = true
			n.run = nop
			n.findex = n.Child[len(n.Child)-1].findex
		case *ast.IncDecStmt:
			wireChild(n)
			switch x.Tok {
			case token.INC:
				n.run = inc
			}
			n.findex = n.Child[0].findex
		case *ast.AssignStmt:
			n.run = assign
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
			switch x.Op {
			case token.AND:
				n.run = and
			case token.EQL:
				n.run = equal
			case token.LSS:
				n.run = lower
			}
			maxIndex++
			n.findex = maxIndex
		case *ast.CallExpr:
			wireChild(n)
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
			if v, err := strconv.ParseInt(x.Value, 0, 0); err == nil {
				n.val = v
			} else {
				n.val = x.Value
			}
		case *ast.Ident:
			// Lookup identifier in frame symbol table. If not found
			// should check that ident is assign target.
			n.ident = x.Name
			if n.findex = symIndex[n.ident]; n.findex == 0 {
				maxIndex++
				symIndex[n.ident] = maxIndex
			}
			n.findex = symIndex[n.ident]
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
