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
	level  int         // number of frame indirections to access value
	kind   Kind        // Kind of node
	typ    *Type       // Type of value in frame, or nil
	recv   *Node       // Method receiver node for call, or nil
	action Action      // Action
	run    Builtin     // function to run at CFG execution
	val    interface{} // pointer on generic value (CFG execution)
	ident  string      // set if node is a var or func
}

// A Frame contains values for the current execution level
type Frame struct {
	anc  *Frame        // Ancestor frame (global space)
	data []interface{} // Values
}

type SymMap map[string]interface{}

type ImportMap map[string]*SymMap

// Interpreter contains global resources and state
type Interpreter struct {
	opt     InterpOpt
	out     Frame
	imports ImportMap
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
	return &Interpreter{opt: opt, imports: make(ImportMap)}
}

// Register a symbol from an imported package to be visible from the interpreter
func (i *Interpreter) AddImport(pkg string, name string, sym interface{}) {
	if i.imports[pkg] == nil {
		s := make(SymMap)
		i.imports[pkg] = &s
	}
	(*i.imports[pkg])[name] = sym
}

func (i *Interpreter) ImportBin(pkg *map[string]*map[string]interface{}) {
	for n, p := range *pkg {
		i.imports[n] = (*SymMap)(p)
	}
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
	i.Cfg(root, tdef, sdef)

	if i.opt.Cfg {
		root.CfgDot(Dotty())
	}

	// Execute CFG
	if !i.opt.NoRun {
		frame := &Frame{data: make([]interface{}, root.fsize)}
		runCfg(root.Start, frame)
		Run(sdef["main"], frame, nil, nil, nil, nil, true, false)
	}
	return i.out
}
