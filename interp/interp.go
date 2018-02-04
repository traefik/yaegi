// Package interp implements a Go interpreter.
package interp

import (
	"go/ast"
)

// Function to run at CFG execution
type RunFun func(n *Node, f *Frame)

// Structure for AST and CFG
type Node struct {
	Child   []*Node      // child subtrees
	anc     *Node        // ancestor
	Start   *Node        // entry point in subtree (CFG)
	snext   *Node        // successor (CFG)
	next    [2]*Node     // conditional successors, for false and for true (CFG)
	rank    int          // rank in child siblings (iterative walk)
	index   int          // node index (dot display)
	findex  int          // index of value in frame or frame size (func def)
	run     RunFun       // function to run at CFG execution
	val     *interface{} // pointer on generic value (CFG execution)
	ident   string       // set if node is a var or func
	isNop   bool         // node run function us a no-op
	isConst bool         // true if node value is constant
	anode   *ast.Node    // original ast node (temporary, will be removed)
}

// A Frame contains values for the current execution level
type Frame struct {
	up   *Frame        // up level in call stack
	down *Frame        // down in call stack
	val  []interface{} // array of values
}

// Interpreter contains global resources and state
type Interpreter struct {
	sym   map[string]*interface{}
	size  int
	frame *Frame
	out   interface{}
}

// n.isLeaf() returns true if Node n is a leaf in the AST
func (n *Node) isLeaf() bool {
	return len((*n).Child) == 0
}

// n.Walk(cbin, cbout) traverses AST n in depth first order, call cbin function
// at node entry and cbout function at node exit.
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

// Same as n.Walk, non recursive
func (e *Node) Walk2(in func(n *Node), out func(n *Node)) {
	if e == nil {
		return
	}
	n := e
	var up *Node
	if in != nil {
		in(n)
	}
	for {
		if len(n.Child) > 0 {
			if up != nil {
				if rank := up.rank + 1; rank < len(n.Child) {
					up = nil
					n = n.Child[rank]
					if in != nil {
						in(n)
					}
				} else {
					if out != nil {
						out(n)
					}
					if n == e {
						break
					}
					up = n
					n = n.anc
				}
			} else {
				n = n.Child[0]
				if in != nil {
					in(n)
				}
			}
		} else {
			if out != nil {
				out(n)
			}
			if n == e {
				break
			}
			up = n
			n = n.anc
		}
	}
}

// NewInterpreter()creates and returns a new interpreter object
func NewInterpreter() *Interpreter {
	return &Interpreter{sym: make(map[string]*interface{})}
}

// i.Eval(s) evaluates Go code represented as a string
func (i *Interpreter) Eval(src string) interface{} {
	root := Ast(src)
	//root.AstDot()
	entry := root.Child[1].Child[2] // FIXME: entry point should be resolved from 'main' name
	i.size = entry.Cfg()
	entry.OptimCfg()
	//entry.CfgDot()
	i.Run(entry.Start)
	return i.out
}
