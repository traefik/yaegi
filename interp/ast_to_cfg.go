package interp

import (
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
)

// TODO: remove coupling with go/ast and go/token. This should be handled only in SrcToAst

// Generate a CFG from AST (wiring successors in AST)
func (e *Node) AstToCfg() {
	e.Walk(nil, func(n *Node) {
		switch x := (*n.anode).(type) {
		case *ast.BlockStmt:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.Child[len(n.Child)-1].val
		case *ast.IncDecStmt:
			wire_child(n)
			switch x.Tok {
			case token.INC:
				n.run = inc
			}
		case *ast.AssignStmt:
			n.run = assign
			wire_child(n)
		case *ast.ExprStmt:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.Child[len(n.Child)-1].val
		case *ast.ParenExpr:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.Child[len(n.Child)-1].val
		case *ast.BinaryExpr:
			wire_child(n)
			switch x.Op {
			case token.AND:
				n.run = and
			case token.EQL:
				n.run = equal
			case token.LSS:
				n.run = lower
			}
		case *ast.CallExpr:
			wire_child(n)
			n.run = call
		case *ast.IfStmt:
			n.isnop = true
			n.run = nop
			n.Start = n.Child[0].Start
			n.Child[1].snext = n
			if len(n.Child) == 3 {
				n.Child[2].snext = n
			}
			n.Child[0].next[1] = n.Child[1].Start
			if len(n.Child) == 3 {
				n.Child[0].next[0] = n.Child[2].Start
			} else {
				n.Child[0].next[0] = n
			}
		case *ast.ForStmt:
			n.isnop = true
			n.run = nop
			// FIXME: works only if for node has 4 children
			n.Start = n.Child[0].Start
			n.Child[0].snext = n.Child[1].Start
			n.Child[1].next[0] = n
			n.Child[1].next[1] = n.Child[3].Start
			n.Child[3].snext = n.Child[2].Start
			n.Child[2].snext = n.Child[1].Start
		case *ast.BasicLit:
			// FIXME: values must be converted to int or float if possible
			if v, err := strconv.ParseInt(x.Value, 0, 0); err == nil {
				*n.val = v
			} else {
				*n.val = x.Value
			}
		case *ast.Ident:
			n.ident = x.Name
		default:
			println("unknown type:", reflect.TypeOf(*n.anode).String())
		}
	})
}

// Wire AST nodes of sequential blocks
func wire_child(n *Node) {
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
		n.Child[i-1].snext = n.Child[i].Start
	}
	for i := len(n.Child) - 1; i >= 0; i-- {
		if !n.Child[i].isLeaf() {
			n.Child[i].snext = n
			break
		}
	}
}

// optimisation: rewire CFG to skip nop nodes
func (e *Node) OptimCfg() {
	e.Walk(nil, func(n *Node) {
		for s := n.snext; s != nil && s.snext != nil; s = s.snext {
			n.snext = s
		}
		for s := n.next[0]; s != nil && s.snext != nil; s = s.snext {
			n.next[0] = s
		}
	})
}
