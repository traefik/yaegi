// Package interp implements a Go interpreter.
package interp

import (
	"reflect"
)

// Node structure for AST and CFG
type Node struct {
	child  []*Node       // child subtrees (AST)
	anc    *Node         // ancestor (AST)
	start  *Node         // entry point in subtree (CFG)
	tnext  *Node         // true branch successor (CFG)
	fnext  *Node         // false branch successor (CFG)
	interp *Interpreter  // interpreter context
	index  int           // node index (dot display)
	findex int           // index of value in frame or frame size (func def, type def)
	fsize  int           // number of entries in frame (call expressions)
	level  int           // number of frame indirections to access value
	kind   Kind          // kind of node
	typ    *Type         // type of value in frame, or nil
	recv   *Node         // method receiver node for call, or nil
	frame  *Frame        // frame pointer, only used for script callbacks from runtime (wrapNode)
	action Action        // action
	run    Builtin       // function to run at CFG execution
	val    interface{}   // static generic value (CFG execution)
	rval   reflect.Value // reflection value to let runtime access interpreter (CFG)
	ident  string        // set if node is a var or func
}

// Frame contains values for the current execution level
type Frame struct {
	anc  *Frame        // ancestor frame (global space)
	data []interface{} // values
}

// NodeMap defines a Map of symbols (const, variables and functions) indexed by names
type NodeMap map[string]*Node

// PkgSrcMap stores package source nodes
type PkgSrcMap map[string]*NodeMap

// SymMap stores executable symbols indexed by name
type SymMap map[string]interface{}

// PkgMap stores package executable symbols
type PkgMap map[string]*SymMap

// ValueMap stores symbols as reflect values
type ValueMap map[string]reflect.Value

// PkgValueMap stores package value maps
type PkgValueMap map[string]*ValueMap

// Opt stores interpreter options
type Opt struct {
	AstDot bool   // display AST graph (debug)
	CfgDot bool   // display CFG graph (debug)
	NoRun  bool   // compile, but do not run
	Entry  string // interpreter entry point
}

// Interpreter contains global resources and state
type Interpreter struct {
	Opt
	Frame   *Frame
	types   TypeMap
	srcPkg  PkgSrcMap
	binPkg  PkgMap
	Exports PkgMap
	Expval  PkgValueMap
}

// Walk traverses AST n in depth first order, call cbin function
// at node entry and cbout function at node exit.
func (n *Node) Walk(in func(n *Node) bool, out func(n *Node)) {
	if in != nil && !in(n) {
		return
	}
	for _, child := range n.child {
		child.Walk(in, out)
	}
	if out != nil {
		out(n)
	}
}

// NewInterpreter creates and returns a new interpreter object
func NewInterpreter(opt Opt) *Interpreter {
	return &Interpreter{
		Opt:     opt,
		types:   defaultTypes,
		srcPkg:  make(PkgSrcMap),
		binPkg:  make(PkgMap),
		Exports: make(PkgMap),
		Expval:  make(PkgValueMap),
	}
}

// AddImport registers a symbol from an imported package to be visible from the interpreter
func (i *Interpreter) AddImport(pkg string, name string, sym interface{}) {
	if i.binPkg[pkg] == nil {
		s := make(SymMap)
		i.binPkg[pkg] = &s
	}
	(*i.binPkg[pkg])[name] = sym
}

// ImportBin registers symbols contained in pkg map
func (i *Interpreter) ImportBin(pkg *map[string]*map[string]interface{}) {
	for n, p := range *pkg {
		i.binPkg[n] = (*SymMap)(p)
	}
}

// Eval evaluates Go code represented as a string
func (i *Interpreter) Eval(src string) (string, *NodeMap) {
	// Parse source to AST
	root, sdef := i.Ast(src, nil)
	if i.AstDot {
		root.AstDot(DotX())
	}

	// Annotate AST with CFG infos
	initNodes := i.Cfg(root, sdef)
	if entry, ok := (*sdef)[i.Entry]; ok {
		initNodes = append(initNodes, entry)
	}

	if i.CfgDot {
		root.CfgDot(DotX())
	}

	// Execute CFG
	if !i.NoRun {
		i.Frame = &Frame{data: make([]interface{}, root.fsize)}
		runCfg(root.start, i.Frame)
		for _, n := range initNodes {
			Run(n, i.Frame, nil, nil, nil, nil, true, false)
		}
	}
	return root.child[0].ident, sdef
}
