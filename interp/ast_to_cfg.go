package interp

import (
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
)

// TODO: remove coupling with go/ast and go/token. This should be handled only in SrcToAst

// Generate a CFG from AST (wiring successors in AST)
func AstToCfg(root *Node) {
	root.Walk(nil, func(n *Node) {
		switch x := (*n.anode).(type) {
		case *ast.BlockStmt:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.child[len(n.child)-1].val
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
			n.val = n.child[len(n.child)-1].val
		case *ast.ParenExpr:
			wire_child(n)
			// FIXME: could bypass this node at CFG and wire directly last child
			n.isnop = true
			n.run = nop
			n.val = n.child[len(n.child)-1].val
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
			n.start = n.child[0].start
			n.child[1].snext = n
			if len(n.child) == 3 {
				n.child[2].snext = n
			}
			n.child[0].next[1] = n.child[1].start
			if len(n.child) == 3 {
				n.child[0].next[0] = n.child[2].start
			} else {
				n.child[0].next[0] = n
			}
		case *ast.ForStmt:
			n.isnop = true
			n.run = nop
			// FIXME: works only if for node has 4 children
			n.start = n.child[0].start
			n.child[0].snext = n.child[1].start
			n.child[1].next[0] = n
			n.child[1].next[1] = n.child[3].start
			n.child[3].snext = n.child[2].start
			n.child[2].snext = n.child[1].start
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
	for _, child := range n.child {
		if !child.isLeaf() {
			n.start = child.start
			break
		}
	}
	if n.start == nil {
		n.start = n
	}
	for i := 1; i < len(n.child); i++ {
		n.child[i-1].snext = n.child[i].start
	}
	for i := len(n.child) - 1; i >= 0; i-- {
		if !n.child[i].isLeaf() {
			n.child[i].snext = n
			break
		}
	}
}
