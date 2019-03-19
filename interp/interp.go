package interp

import (
	"bufio"
	"fmt"
	"go/build"
	"go/scanner"
	"go/token"
	"os"
	"reflect"
)

// Node structure for AST and CFG
type Node struct {
	child  []*Node          // child subtrees (AST)
	anc    *Node            // ancestor (AST)
	start  *Node            // entry point in subtree (CFG)
	tnext  *Node            // true branch successor (CFG)
	fnext  *Node            // false branch successor (CFG)
	interp *Interpreter     // interpreter context
	frame  *Frame           // frame pointer used for closures only (TODO: suppress this)
	index  int              // node index (dot display)
	findex int              // index of value in frame or frame size (func def, type def)
	level  int              // number of frame indirections to access value
	kind   Kind             // kind of node
	fset   *token.FileSet   // fileset to locate node in source code
	pos    token.Pos        // position in source code, relative to fset
	sym    *Symbol          // associated symbol
	typ    *Type            // type of value in frame, or nil
	recv   *Receiver        // method receiver node for call, or nil
	types  []reflect.Type   // frame types, used by function literals only
	action Action           // action
	exec   Builtin          // generated function to execute
	gen    BuiltinGenerator // generator function to produce above bltn
	val    interface{}      // static generic value (CFG execution)
	rval   reflect.Value    // reflection value to let runtime access interpreter (CFG)
	ident  string           // set if node is a var or func
}

// Receiver stores method receiver object access path
type Receiver struct {
	node  *Node         // receiver value for alias and struct types
	val   reflect.Value // receiver value for interface type
	index []int         // path in receiver value for interface type
}

// Frame contains values for the current execution level (a function context)
type Frame struct {
	anc       *Frame            // ancestor frame (global space)
	data      []reflect.Value   // values
	deferred  [][]reflect.Value // defer stack
	recovered interface{}       // to handle panic recover
}

// LibValueMap stores the map of external values per package
type LibValueMap map[string]map[string]reflect.Value

// Opt stores interpreter options
type Opt struct {
	AstDot bool // display AST graph (debug)
	CfgDot bool // display CFG graph (debug)
	NoRun  bool // compile, but do not run
	GoPath string
	Entry  string // interpreter entry point
}

// Interpreter contains global resources and state
type Interpreter struct {
	Opt
	Name     string            // program name
	Frame    *Frame            // program data storage during execution
	nindex   int               // next node index
	universe *Scope            // interpreter global level scope
	scope    map[string]*Scope // package level scopes, indexed by package name
	binValue LibValueMap       // runtime binary values used in interpreter
}

// ExportValue exposes interpreter values
var ExportValue = LibValueMap{}

func init() {
	me := "github.com/containous/dyngo/interp"
	ExportValue[me] = map[string]reflect.Value{
		"New":         reflect.ValueOf(New),
		"Interpreter": reflect.ValueOf((*Interpreter)(nil)),
		"Opt":         reflect.ValueOf((*Opt)(nil)),
	}
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
	if len(opt.GoPath) == 0 {
		opt.GoPath = build.Default.GOPATH
	}

	return &Interpreter{
		Opt:      opt,
		universe: initUniverse(),
		scope:    map[string]*Scope{},
		binValue: LibValueMap{},
		Frame:    &Frame{data: []reflect.Value{}},
	}
}

func initUniverse() *Scope {
	scope := &Scope{global: true, sym: SymMap{
		// predefined Go types
		"bool":        &Symbol{kind: Typ, typ: &Type{cat: BoolT, name: "bool"}},
		"byte":        &Symbol{kind: Typ, typ: &Type{cat: ByteT, name: "byte"}},
		"complex64":   &Symbol{kind: Typ, typ: &Type{cat: Complex64T, name: "complex64"}},
		"complex128":  &Symbol{kind: Typ, typ: &Type{cat: Complex128T, name: "complex128"}},
		"error":       &Symbol{kind: Typ, typ: &Type{cat: ErrorT, name: "error"}},
		"float32":     &Symbol{kind: Typ, typ: &Type{cat: Float32T, name: "float32"}},
		"float64":     &Symbol{kind: Typ, typ: &Type{cat: Float64T, name: "float64"}},
		"int":         &Symbol{kind: Typ, typ: &Type{cat: IntT, name: "int"}},
		"int8":        &Symbol{kind: Typ, typ: &Type{cat: Int8T, name: "int8"}},
		"int16":       &Symbol{kind: Typ, typ: &Type{cat: Int16T, name: "int16"}},
		"int32":       &Symbol{kind: Typ, typ: &Type{cat: Int32T, name: "int32"}},
		"int64":       &Symbol{kind: Typ, typ: &Type{cat: Int64T, name: "int64"}},
		"interface{}": &Symbol{kind: Typ, typ: &Type{cat: InterfaceT}},
		"rune":        &Symbol{kind: Typ, typ: &Type{cat: RuneT, name: "rune"}},
		"string":      &Symbol{kind: Typ, typ: &Type{cat: StringT, name: "string"}},
		"uint":        &Symbol{kind: Typ, typ: &Type{cat: UintT, name: "uint"}},
		"uint8":       &Symbol{kind: Typ, typ: &Type{cat: Uint8T, name: "uint8"}},
		"uint16":      &Symbol{kind: Typ, typ: &Type{cat: Uint16T, name: "uint16"}},
		"uint32":      &Symbol{kind: Typ, typ: &Type{cat: Uint32T, name: "uint32"}},
		"uint64":      &Symbol{kind: Typ, typ: &Type{cat: Uint64T, name: "uint64"}},
		"uintptr":     &Symbol{kind: Typ, typ: &Type{cat: UintptrT, name: "uintptr"}},

		// predefined Go constants
		"false": &Symbol{kind: Const, typ: &Type{cat: BoolT}, val: false},
		"true":  &Symbol{kind: Const, typ: &Type{cat: BoolT}, val: true},
		"iota":  &Symbol{kind: Const, typ: &Type{cat: IntT}},

		// predefined Go zero value
		"nil": &Symbol{typ: &Type{cat: NilT, untyped: true}},

		// predefined Go builtins
		"append":  &Symbol{kind: Bltn, builtin: _append},
		"cap":     &Symbol{kind: Bltn, builtin: _cap},
		"close":   &Symbol{kind: Bltn, builtin: _close},
		"complex": &Symbol{kind: Bltn, builtin: _complex},
		"imag":    &Symbol{kind: Bltn, builtin: _imag},
		"len":     &Symbol{kind: Bltn, builtin: _len},
		"make":    &Symbol{kind: Bltn, builtin: _make},
		"panic":   &Symbol{kind: Bltn, builtin: _panic},
		"println": &Symbol{kind: Bltn, builtin: _println},
		"real":    &Symbol{kind: Bltn, builtin: _real},
		"recover": &Symbol{kind: Bltn, builtin: _recover},
		// TODO: complex, copy, delete, imag, new, print, real
	}}
	return scope
}

// resizeFrame resizes the global frame of interpreter
func (i *Interpreter) resizeFrame() {
	l := len(i.universe.types)
	b := len(i.Frame.data)
	if l-b <= 0 {
		return
	}
	data := make([]reflect.Value, l)
	copy(data, i.Frame.data)
	for j, t := range i.universe.types[b:] {
		data[b+j] = reflect.New(t).Elem()
	}
	i.Frame.data = data
}

// Eval evaluates Go code represented as a string. It returns a map on
// current interpreted package exported symbols
func (i *Interpreter) Eval(src string) (reflect.Value, error) {
	var res reflect.Value

	// Parse source to AST
	pkgName, root, err := i.ast(src, i.Name)
	if err != nil {
		return res, err
	}

	if i.AstDot {
		root.AstDot(DotX(), i.Name)
	}

	// Global type analysis
	if err = i.Gta(root, pkgName); err != nil {
		return res, err
	}

	// Annotate AST with CFG infos
	initNodes, err := i.Cfg(root)
	if err != nil {
		return res, err
	}

	if pkgName != "_" {
		if sym := i.scope[pkgName].sym[i.Entry]; sym != nil {
			initNodes = append(initNodes, sym.node)
		}
	} else {
		setExec(root.start)
	}
	if i.universe.sym[pkgName] == nil {
		// Make the package visible under a path identical to its name
		i.universe.sym[pkgName] = &Symbol{typ: &Type{cat: SrcPkgT}, path: pkgName}
	}

	if i.CfgDot {
		root.CfgDot(DotX())
	}

	// Execute CFG
	if !i.NoRun {
		if err = genRun(root); err != nil {
			return res, err
		}
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
func (i *Interpreter) Use(values LibValueMap) {
	for k, v := range values {
		i.binValue[k] = v
	}
}

// Repl performs a Read-Eval-Print-Loop on input file descriptor.
// Results are printed on output.
func (i *Interpreter) Repl(in, out *os.File) {
	s := bufio.NewScanner(in)
	prompt := getPrompt(in, out)
	prompt()
	src := ""
	for s.Scan() {
		src += s.Text() + "\n"
		if v, err := i.Eval(src); err != nil {
			switch err.(type) {
			case scanner.ErrorList:
				// Early failure in the scanner: the source is incomplete
				// and no AST could be produced, neither compiled / run.
				// Get one more line, and retry
				continue
			default:
				fmt.Fprintln(out, err)
			}
		} else if v.IsValid() {
			fmt.Fprintln(out, v)
		}
		src = ""
		prompt()
	}
}

// getPrompt returns a function which prints a prompt only if input is a terminal
func getPrompt(in, out *os.File) func() {
	if stat, err := in.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		return func() { fmt.Fprint(out, "> ") }
	}
	return func() {}
}
