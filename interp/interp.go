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
	kind   Kind        // kind of node
	typ    *Type       // type of value in frame, or nil
	recv   *Node       // method receiver node for call, or nil
	frame  *Frame      // frame pointer, only used for script callbacks from runtime (wrapNode)
	action Action      // action
	run    Builtin     // function to run at CFG execution
	val    interface{} // pointer on generic value (CFG execution)
	ident  string      // set if node is a var or func
}

// A Frame contains values for the current execution level
type Frame struct {
	anc  *Frame        // ancestor frame (global space)
	data []interface{} // values
}

type SymMap map[string]interface{}

type PkgMap map[string]*SymMap

// Interpreter contains global resources and state
type Interpreter struct {
	opt     InterpOpt
	frame   *Frame
	imports PkgMap
	Exports PkgMap
}

type InterpOpt struct {
	Ast   bool   // display AST graph (debug)
	Cfg   bool   // display CFG graph (debug)
	NoRun bool   // compile, but do not run
	NbOut int    // number of output values
	Entry string // interpreter entry point
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
	return &Interpreter{opt: opt, imports: make(PkgMap)}
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
func (i *Interpreter) Eval(src string) {
	// Parse source to AST
	root, sdef := Ast(src, nil)
	if i.opt.Ast {
		root.AstDot(Dotty())
	}

	// Annotate AST with CFG infos
	tdef := initTypes()
	initGoBuiltin()
	initNodes := i.Cfg(root, tdef, sdef)
	if entry, ok := sdef[i.opt.Entry]; ok {
		initNodes = append(initNodes, entry)
	}

	if i.opt.Cfg {
		root.CfgDot(Dotty())
	}

	// Execute CFG
	if !i.opt.NoRun {
		i.frame = &Frame{data: make([]interface{}, root.fsize)}
		runCfg(root.Start, i.frame)
		for _, n := range initNodes {
			Run(n, i.frame, nil, nil, nil, nil, true, false)
		}
	}
}
