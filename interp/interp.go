package interp

import (
	"reflect"
)

// Node structure for AST and CFG
type Node struct {
	child    []*Node          // child subtrees (AST)
	anc      *Node            // ancestor (AST)
	start    *Node            // entry point in subtree (CFG)
	tnext    *Node            // true branch successor (CFG)
	fnext    *Node            // false branch successor (CFG)
	interp   *Interpreter     // interpreter context
	frame    *Frame           // frame pointer used for closures only (TODO: suppress this)
	index    int              // node index (dot display)
	findex   int              // index of value in frame or frame size (func def, type def)
	fsize    int              // number of entries in frame (call expressions)
	flen     int              // frame length (function definition)
	level    int              // number of frame indirections to access value
	kind     Kind             // kind of node
	sym      *Symbol          // associated symbol
	typ      *Type            // type of value in frame, or nil
	recv     *Receiver        // method receiver node for call, or nil
	types    []reflect.Type   // frame types, used by function litterals only
	framepos []int            // frame positions of function parameters
	action   Action           // action
	exec     Builtin          // generated function to execute
	gen      BuiltinGenerator // generator function to produce above bltn
	val      interface{}      // static generic value (CFG execution)
	rval     reflect.Value    // reflection value to let runtime access interpreter (CFG)
	ident    string           // set if node is a var or func
}

// Receiver stores method receiver object access path
type Receiver struct {
	node  *Node
	index []int
}

// Frame contains values for the current execution level
type Frame struct {
	anc  *Frame          // ancestor frame (global space)
	data []reflect.Value // values
}

// LibValueMap stores the map of extern values per package
type LibValueMap map[string]map[string]reflect.Value

// LibTypeMap stores the map of extern types per package
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
	Name     string            // program name
	Frame    *Frame            // programe data storage during execution
	fsize    int               // global interpreter frame size
	nindex   int               // next node index
	universe *Scope            // interpreter global level scope
	scope    map[string]*Scope // package level scopes, indexed by package name
	binValue LibValueMap       // runtime binary values used in interpreter
	binType  LibTypeMap        // runtime binary types used in interpreter
}

// ExportValue exposes interpreter values
var ExportValue = LibValueMap{}

// ExportType exposes interpreter types
var ExportType = LibTypeMap{}

func init() {
	me := "github.com/containous/dyngo/interp"
	ExportValue[me] = map[string]reflect.Value{
		"New": reflect.ValueOf(New),
	}
	ExportType[me] = map[string]reflect.Type{
		"Interpreter": reflect.TypeOf((*Interpreter)(nil)).Elem(),
		"Opt":         reflect.TypeOf((*Opt)(nil)).Elem(),
	}
	ExportValue[me]["ExportType"] = reflect.ValueOf(ExportType)
	ExportValue[me]["ExportValue"] = reflect.ValueOf(ExportValue)
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

// New creates and returns a new interpreter object
func New(opt Opt) *Interpreter {
	return &Interpreter{
		Opt:      opt,
		universe: initUniverse(),
		scope:    map[string]*Scope{},
		binValue: LibValueMap{},
		binType:  LibTypeMap{},
		Frame:    &Frame{data: []reflect.Value{}},
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
		"cap":     &Symbol{kind: Bltn, builtin: _cap},
		"len":     &Symbol{kind: Bltn, builtin: _len},
		"make":    &Symbol{kind: Bltn, builtin: _make},
		"panic":   &Symbol{kind: Bltn, builtin: _panic},
		"println": &Symbol{kind: Bltn, builtin: _println},
		// TODO: close, complex, copy, delete, imag, new, print, real, recover
	}}
	return scope
}

// resizeFrame resizes the global frame of interpeter
func (i *Interpreter) resizeFrame() {
	f := &Frame{data: make([]reflect.Value, i.fsize)}
	copy(f.data, i.Frame.data)
	i.Frame = f
}

// Eval evaluates Go code represented as a string. It returns a map on
// current interpreted package exported symbols
func (i *Interpreter) Eval(src string) (reflect.Value, error) {
	var err error
	// Parse source to AST
	pkgName, root := i.ast(src, i.Name)
	if i.AstDot {
		root.AstDot(DotX(), i.Name)
	}

	// Global type analysis
	i.Gta(root)

	// Annotate AST with CFG infos
	initNodes := i.Cfg(root)
	if pkgName != "_" {
		if sym := i.scope[pkgName].sym[i.Entry]; sym != nil {
			initNodes = append(initNodes, sym.node)
		}
	} else {
		root.types = frameTypes(root, i.fsize+1)
		setExec(root.start)
	}

	if i.CfgDot {
		root.CfgDot(DotX())
	}

	var res reflect.Value
	// Execute CFG
	if !i.NoRun {
		genRun(root)
		i.fsize++
		i.resizeFrame()
		i.run(root, nil)

		for _, n := range initNodes {
			i.run(n, i.Frame)
		}
		v := genValue(root)
		res = v(i.Frame)
	}

	// If result is an interpreter node, wrap it in a runtime callable function
	if res.IsValid() {
		if n, ok := res.Interface().(*Node); ok {
			res = genNodeWrapper(n)(i.Frame)
		}
	}
	return res, err
}

// Use loads binary runtime symbols in the interpreter context so
// they can be used in interpreted code
func (i *Interpreter) Use(values LibValueMap, types LibTypeMap) {
	for k, v := range values {
		i.binValue[k] = v
	}
	for k, v := range types {
		i.binType[k] = v
	}
}
