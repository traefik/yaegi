package interp

import (
	"reflect"

	"github.com/containous/dyngo/stdlib"
)

// Node structure for AST and CFG
type Node struct {
	child  []*Node                     // child subtrees (AST)
	anc    *Node                       // ancestor (AST)
	start  *Node                       // entry point in subtree (CFG)
	tnext  *Node                       // true branch successor (CFG)
	fnext  *Node                       // false branch successor (CFG)
	interp *Interpreter                // interpreter context
	index  int                         // node index (dot display)
	findex int                         // index of value in frame or frame size (func def, type def)
	fsize  int                         // number of entries in frame (call expressions)
	flen   int                         // frame length (function definition)
	level  int                         // number of frame indirections to access value
	kind   Kind                        // kind of node
	sym    *Symbol                     // associated symbol
	typ    *Type                       // type of value in frame, or nil
	recv   *Node                       // method receiver node for call, or nil
	frame  *Frame                      // frame pointer, only used for script callbacks from runtime (wrapNode)
	action Action                      // action
	value  func(f *Frame) interface{}  // function which returns node value during execution
	pvalue func(f *Frame) *interface{} // function which returns pointer on node value
	run    Builtin                     // function to run at CFG execution
	val    interface{}                 // static generic value (CFG execution)
	rval   reflect.Value               // reflection value to let runtime access interpreter (CFG)
	ident  string                      // set if node is a var or func
}

// Frame contains values for the current execution level
type Frame struct {
	anc  *Frame        // ancestor frame (global space)
	data []interface{} // values
}

// BinMap stores executable symbols indexed by name
type BinMap map[string]interface{}

// PkgMap stores package executable symbols
type PkgMap map[string]*BinMap

// ValueMap stores symbols as reflect values
type ValueMap map[string]reflect.Value

// PkgValueMap stores package value maps
type PkgValueMap map[string]*ValueMap

type LibValueMap map[string]map[string]reflect.Value

type LibTypeMap map[string]map[string]reflect.Type

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
	Frame    *Frame            // programe data storage during execution
	fsize    int               // global interpreter frame size
	nindex   int               // next node index
	universe *Scope            // interpreter global level scope
	scope    map[string]*Scope // package level scopes, indexed by package name
	binValue LibValueMap
	binType  LibTypeMap
	Exports  PkgMap      // exported symbols for use by runtime
	Expval   PkgValueMap // same as abobe (TODO: keep only one)
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
		Opt:      opt,
		universe: initUniverse(),
		scope:    map[string]*Scope{},
		Exports:  make(PkgMap),
		Expval:   make(PkgValueMap),
		binValue: LibValueMap(stdlib.Value),
		binType:  LibTypeMap(stdlib.Type),
		Frame:    &Frame{data: []interface{}{}},
	}
}

func initUniverse() *Scope {
	scope := &Scope{global: true, sym: SymMap{
		// predefined Go types
		"bool":       &Symbol{kind: Typ, typ: &Type{cat: BoolT}},
		"byte":       &Symbol{kind: Typ, typ: &Type{cat: ByteT}},
		"complex64":  &Symbol{kind: Typ, typ: &Type{cat: Complex64T}},
		"complex128": &Symbol{kind: Typ, typ: &Type{cat: Complex128T}},
		"error":      &Symbol{kind: Typ, typ: &Type{cat: ErrorT}},
		"float32":    &Symbol{kind: Typ, typ: &Type{cat: Float32T}},
		"float64":    &Symbol{kind: Typ, typ: &Type{cat: Float64T}},
		"int":        &Symbol{kind: Typ, typ: &Type{cat: IntT}},
		"int8":       &Symbol{kind: Typ, typ: &Type{cat: Int8T}},
		"int16":      &Symbol{kind: Typ, typ: &Type{cat: Int16T}},
		"int32":      &Symbol{kind: Typ, typ: &Type{cat: Int32T}},
		"int64":      &Symbol{kind: Typ, typ: &Type{cat: Int64T}},
		"rune":       &Symbol{kind: Typ, typ: &Type{cat: RuneT}},
		"string":     &Symbol{kind: Typ, typ: &Type{cat: StringT}},
		"uint":       &Symbol{kind: Typ, typ: &Type{cat: UintT}},
		"uint8":      &Symbol{kind: Typ, typ: &Type{cat: Uint8T}},
		"uint16":     &Symbol{kind: Typ, typ: &Type{cat: Uint16T}},
		"uint32":     &Symbol{kind: Typ, typ: &Type{cat: Uint32T}},
		"uint64":     &Symbol{kind: Typ, typ: &Type{cat: Uint64T}},
		"uintptr":    &Symbol{kind: Typ, typ: &Type{cat: UintptrT}},

		// predefined Go constants
		"false": &Symbol{kind: Const, typ: &Type{cat: BoolT}, val: false},
		"true":  &Symbol{kind: Const, typ: &Type{cat: BoolT}, val: true},
		"iota":  &Symbol{kind: Const, typ: &Type{cat: IntT}},

		// predefined Go zero value
		"nil": &Symbol{typ: &Type{cat: UnsetT}},

		// predefined Go builtins
		"append":  &Symbol{kind: Bltn, builtin: _append},
		"len":     &Symbol{kind: Bltn, builtin: _len},
		"make":    &Symbol{kind: Bltn, builtin: _make},
		"panic":   &Symbol{kind: Bltn, builtin: _panic},
		"println": &Symbol{kind: Bltn, builtin: _println},
		"sleep":   &Symbol{kind: Bltn, builtin: sleep}, // Temporary, for debug
		// TODO: cap, close, complex, copy, delete, imag, new, print, real, recover
	}}
	return scope
}

func (i *Interpreter) resizeFrame() {
	f := &Frame{data: make([]interface{}, i.fsize)}
	copy(f.data, i.Frame.data)
	i.Frame = f
}

// Eval evaluates Go code represented as a string. It returns a map on
// current interpreted package exported symbols
func (i *Interpreter) Eval(src string) string {
	// Parse source to AST
	pkgName, root := i.Ast(src)
	if i.AstDot {
		root.AstDot(DotX())
	}

	// Global type analysis
	i.Gta(root)

	// Annotate AST with CFG infos
	initNodes := i.Cfg(root)
	if sym := i.scope[pkgName].sym[i.Entry]; sym != nil {
		initNodes = append(initNodes, sym.node)
	}

	if i.CfgDot {
		root.CfgDot(DotX())
	}

	// Execute CFG
	if !i.NoRun {
		i.fsize++
		i.resizeFrame()
		runCfg(root.start, i.Frame)
		for _, n := range initNodes {
			Run(n, i.Frame, nil, nil, nil, nil, true, false)
		}
	}
	return root.child[0].ident
}
