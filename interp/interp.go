// Package interp implements a Go interpreter.
package interp

import (
	"go/ast"
)

// Structure for AST and CFG
type Node struct {
	Child   []*Node     // child subtrees
	anc     *Node       // ancestor
	Start   *Node       // entry point in subtree (CFG)
	tnext   *Node       // true branch successor (CFG)
	fnext   *Node       // false branch successor (CFG)
	index   int         // node index (dot display)
	findex  int         // index of value in frame or frame size (func def)
	kind    Kind        // Kind of node
	action  Action      // function to run
	run     RunFun      // function to run at CFG execution
	val     interface{} // pointer on generic value (CFG execution)
	ident   string      // set if node is a var or func
	isNop   bool        // node run function us a no-op
	isConst bool        // true if node value is constant
	anode   *ast.Node   // original ast node (temporary, will be removed)
}

// A Frame contains values for the current execution level
type Frame []interface{}

// Interpreter contains global resources and state
type Interpreter struct {
	opt InterpOpt
	out interface{}
	def map[string]*Node // map of defined symbols
}

type InterpOpt struct {
	Ast   bool // Display AST graph (debug)
	Cfg   bool // Display CFG graph (debug)
	NoRun bool // Compile, but do not run
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

// NewInterpreter()creates and returns a new interpreter object
func NewInterpreter(opt InterpOpt) *Interpreter {
	return &Interpreter{opt: opt, def: make(map[string]*Node)}
}

// i.Eval(s) evaluates Go code represented as a string
func (i *Interpreter) Eval(src string) interface{} {
	// Parse source to AST
	root := Ast(src)
	if i.opt.Ast {
		root.AstDot(Dotty())
	}

	// Annotate AST with CFG infos
	root.Cfg(i)
	if i.opt.Cfg {
		root.CfgDot(Dotty())
	}
	//root.OptimCfg()

	// Execute CFG
	if !i.opt.NoRun {
		Run(i.def["main"], nil, nil, nil)
	}
	return i.out
}
