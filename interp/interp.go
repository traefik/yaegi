// Package interp implements a Go interpreter.
package interp

import (
	"go/ast"
)

// Function to run at CFG execution
type RunFun func(n *Node)

// Structure for AST and CFG
type Node struct {
	child []*Node      // child subtrees
	anc   *Node        // ancestor
	start *Node        // entry point in subtree (CFG)
	snext *Node        // successor (CFG)
	next  [2]*Node     // conditional successors, for false and for true (CFG)
	index int          // node index (dot display)
	run   RunFun       // function to run at CFG execution
	val   *interface{} // pointer on generic value (CFG execution)
	ident string       // set if node is a var or func
	isnop bool         // node is a no op
	anode *ast.Node    // original ast node (temporary, will be removed)
}

// Interpreter execution context
type Interpreter struct {
	sym map[string]*interface{}
}

// Returns true if node is a leaf in the AST
func (n *Node) isLeaf() bool {
	return len((*n).child) == 0
}

// Walk AST in depth first order, call 'in' function at node entry and
// 'out' function at node exit.
func (n *Node) Walk(in func(n *Node), out func(n *Node)) {
	if in != nil {
		in(n)
	}
	for _, child := range n.child {
		child.Walk(in, out)
	}
	if out != nil {
		out(n)
	}
}
