// Package interp implements a Go interpreter.
package interp

import (
	"go/ast"
)

// Function to run at CFG execution
type RunFun func(n *Node, i *Interpreter)

// Structure for AST and CFG
type Node struct {
	Child []*Node      // child subtrees
	anc   *Node        // ancestor
	Start *Node        // entry point in subtree (CFG)
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
	res interface{}
}

// Returns true if node is a leaf in the AST
func (n *Node) isLeaf() bool {
	return len((*n).Child) == 0
}

// Walk AST in depth first order, call 'in' function at node entry and
// 'out' function at node exit.
func (n *Node) Walk(in func(n *Node), out func(n *Node)) {
	if in != nil {
		in(n)
	}
	for _, Child := range n.Child {
		Child.Walk(in, out)
	}
	if out != nil {
		out(n)
	}
}

// Create and return a new interpreter object
func NewInterpreter() *Interpreter {
	return &Interpreter{sym: make(map[string]*interface{})}
}

//
func (i *Interpreter) Eval(src string) interface{} {
	root := SrcToAst(src)
	root.AstDot()
	cfg_entry := root.Child[1].Child[2] // FIXME: entry point should be resolved from 'main' name
	cfg_entry.AstToCfg()
	cfg_entry.OptimCfg()
	//cfg_entry.CfgDot()
	i.RunCfg(cfg_entry.Start)
	return i.res
}
