// Package interp implements a Go interpreter.
package interp

// Structure for AST and CFG
type Node struct {
	Child  []*Node     // child subtrees (AST)
	anc    *Node       // ancestor (AST)
	Start  *Node       // entry point in subtree (CFG)
	tnext  *Node       // true branch successor (CFG)
	fnext  *Node       // false branch successor (CFG)
	index  int         // node index (dot display)
	findex int         // index of value in frame or frame size (func def, type def)
	fsize  int         // number of entries in frame (call expressions)
	kind   Kind        // Kind of node
	typ    *Type       // Type of value in frame, or nil
	action Action      // function to run
	run    Builtin     // function to run at CFG execution
	val    interface{} // pointer on generic value (CFG execution)
	ident  string      // set if node is a var or func
}

// A Frame contains values for the current execution level
type Frame []interface{}

// Interpreter contains global resources and state
type Interpreter struct {
	opt InterpOpt
	out Frame
	//	def map[string]*Node // map of defined symbols
}

type InterpOpt struct {
	Ast   bool   // Display AST graph (debug)
	Cfg   bool   // Display CFG graph (debug)
	NoRun bool   // Compile, but do not run
	NbOut int    // Number of output values
	entry string // Interpreter entry point
}

// n.Walk(cbin, cbout) traverses AST n in depth first order, call cbin function
// at node entry and cbout function at node exit.
func (n *Node) Walk(in func(n *Node) bool, out func(n *Node)) {
	if in != nil && !in(n) {
		return
	}
	for _, child := range n.Child {
		child.Walk(in, out)
	}
	if out != nil {
		out(n)
	}
}

// NewInterpreter()creates and returns a new interpreter object
func NewInterpreter(opt InterpOpt) *Interpreter {
	//return &Interpreter{opt: opt, def: make(map[string]*Node)}
	return &Interpreter{opt: opt}
}

// i.Eval(s) evaluates Go code represented as a string
func (i *Interpreter) Eval(src string) Frame {
	// Parse source to AST
	root, sdef := Ast(src, nil)
	if i.opt.Ast {
		root.AstDot(Dotty())
	}

	// Annotate AST with CFG infos
	tdef := initTypes()
	initGoBuiltin()
	root.Cfg(tdef, sdef)

	if i.opt.Cfg {
		root.CfgDot(Dotty())
	}

	// Execute CFG
	if !i.opt.NoRun {
		Run(sdef["main"], &i.out, nil, nil, nil, nil)
	}
	return i.out
}
