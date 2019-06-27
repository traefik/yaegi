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

// Interpreter node structure for AST and CFG
type node struct {
	child  []*node        // child subtrees (AST)
	anc    *node          // ancestor (AST)
	start  *node          // entry point in subtree (CFG)
	tnext  *node          // true branch successor (CFG)
	fnext  *node          // false branch successor (CFG)
	interp *Interpreter   // interpreter context
	frame  *frame         // frame pointer used for closures only (TODO: suppress this)
	index  int            // node index (dot display)
	findex int            // index of value in frame or frame size (func def, type def)
	level  int            // number of frame indirections to access value
	nleft  int            // number of children in left part (assign)
	nright int            // number of children in right part (assign)
	kind   nkind          // kind of node
	pos    token.Pos      // position in source code, relative to fset
	sym    *symbol        // associated symbol
	typ    *itype         // type of value in frame, or nil
	recv   *receiver      // method receiver node for call, or nil
	types  []reflect.Type // frame types, used by function literals only
	action action         // action
	exec   bltn           // generated function to execute
	gen    bltnGenerator  // generator function to produce above bltn
	val    interface{}    // static generic value (CFG execution)
	rval   reflect.Value  // reflection value to let runtime access interpreter (CFG)
	ident  string         // set if node is a var or func
}

// receiver stores method receiver object access path
type receiver struct {
	node  *node         // receiver value for alias and struct types
	val   reflect.Value // receiver value for interface type and value type
	index []int         // path in receiver value for interface or value type
}

// frame contains values for the current execution level (a function context)
type frame struct {
	anc       *frame            // ancestor frame (global space)
	data      []reflect.Value   // values
	deferred  [][]reflect.Value // defer stack
	recovered interface{}       // to handle panic recover
}

// PkgSet stores the map of external values per package
type PkgSet map[string]map[string]reflect.Value

// opt stores interpreter options
type opt struct {
	astDot bool   // display AST graph (debug)
	cfgDot bool   // display CFG graph (debug)
	noRun  bool   // compile, but do not run
	goPath string // custom GOPATH
}

// Interpreter contains global resources and state
type Interpreter struct {
	Name string // program name
	opt
	frame    *frame            // program data storage during execution
	nindex   int               // next node index
	fset     *token.FileSet    // fileset to locate node in source code
	universe *scope            // interpreter global level scope
	scopes   map[string]*scope // package level scopes, indexed by package name
	binValue PkgSet            // runtime binary values used in interpreter
}

const (
	mainID   = "main"
	selfPath = "github.com/containous/yaegi/interp"
)

// ExportValue exposes interpreter values
var ExportValue = PkgSet{
	selfPath: map[string]reflect.Value{
		"New": reflect.ValueOf(New),

		"Interpreter": reflect.ValueOf((*Interpreter)(nil)),
		"Opt":         reflect.ValueOf((*opt)(nil)),
	},
}

// _error is a wrapper of error interface type
type _error struct {
	WError func() string
}

func (w _error) Error() string { return w.WError() }

func init() { ExportValue[selfPath]["ExportValue"] = reflect.ValueOf(ExportValue) }

// Walk traverses AST n in depth first order, call cbin function
// at node entry and cbout function at node exit.
func (n *node) Walk(in func(n *node) bool, out func(n *node)) {
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

// New returns a new interpreter
func New(options ...func(*Interpreter)) *Interpreter {
	i := Interpreter{
		opt:      opt{goPath: build.Default.GOPATH},
		fset:     token.NewFileSet(),
		universe: initUniverse(),
		scopes:   map[string]*scope{},
		binValue: PkgSet{"": map[string]reflect.Value{"_error": reflect.ValueOf((*_error)(nil))}},
		frame:    &frame{data: []reflect.Value{}},
	}

	for _, option := range options {
		option(&i)
	}

	return &i
}

// GoPath sets GOPATH for the interpreter
func GoPath(s string) func(*Interpreter) {
	return func(interp *Interpreter) { interp.goPath = s }
}

// AstDot activates AST graph display for the interpreter
func AstDot(interp *Interpreter) { interp.astDot = true }

// CfgDot activates AST graph display for the interpreter
func CfgDot(interp *Interpreter) { interp.cfgDot = true }

// NoRun disable the execution (but not the compilation) in the interpreter
func NoRun(interp *Interpreter) { interp.noRun = true }

func initUniverse() *scope {
	sc := &scope{global: true, sym: symMap{
		// predefined Go types
		"bool":        &symbol{kind: typeSym, typ: &itype{cat: boolT, name: "bool"}},
		"byte":        &symbol{kind: typeSym, typ: &itype{cat: byteT, name: "byte"}},
		"complex64":   &symbol{kind: typeSym, typ: &itype{cat: complex64T, name: "complex64"}},
		"complex128":  &symbol{kind: typeSym, typ: &itype{cat: complex128T, name: "complex128"}},
		"error":       &symbol{kind: typeSym, typ: &itype{cat: errorT, name: "error"}},
		"float32":     &symbol{kind: typeSym, typ: &itype{cat: float32T, name: "float32"}},
		"float64":     &symbol{kind: typeSym, typ: &itype{cat: float64T, name: "float64"}},
		"int":         &symbol{kind: typeSym, typ: &itype{cat: intT, name: "int"}},
		"int8":        &symbol{kind: typeSym, typ: &itype{cat: int8T, name: "int8"}},
		"int16":       &symbol{kind: typeSym, typ: &itype{cat: int16T, name: "int16"}},
		"int32":       &symbol{kind: typeSym, typ: &itype{cat: int32T, name: "int32"}},
		"int64":       &symbol{kind: typeSym, typ: &itype{cat: int64T, name: "int64"}},
		"interface{}": &symbol{kind: typeSym, typ: &itype{cat: interfaceT}},
		"rune":        &symbol{kind: typeSym, typ: &itype{cat: runeT, name: "rune"}},
		"string":      &symbol{kind: typeSym, typ: &itype{cat: stringT, name: "string"}},
		"uint":        &symbol{kind: typeSym, typ: &itype{cat: uintT, name: "uint"}},
		"uint8":       &symbol{kind: typeSym, typ: &itype{cat: uint8T, name: "uint8"}},
		"uint16":      &symbol{kind: typeSym, typ: &itype{cat: uint16T, name: "uint16"}},
		"uint32":      &symbol{kind: typeSym, typ: &itype{cat: uint32T, name: "uint32"}},
		"uint64":      &symbol{kind: typeSym, typ: &itype{cat: uint64T, name: "uint64"}},
		"uintptr":     &symbol{kind: typeSym, typ: &itype{cat: uintptrT, name: "uintptr"}},

		// predefined Go constants
		"false": &symbol{kind: constSym, typ: &itype{cat: boolT, name: "bool"}, rval: reflect.ValueOf(false)},
		"true":  &symbol{kind: constSym, typ: &itype{cat: boolT, name: "bool"}, rval: reflect.ValueOf(true)},
		"iota":  &symbol{kind: constSym, typ: &itype{cat: intT}},

		// predefined Go zero value
		"nil": &symbol{typ: &itype{cat: nilT, untyped: true}},

		// predefined Go builtins
		"append":  &symbol{kind: bltnSym, builtin: _append},
		"cap":     &symbol{kind: bltnSym, builtin: _cap},
		"close":   &symbol{kind: bltnSym, builtin: _close},
		"complex": &symbol{kind: bltnSym, builtin: _complex},
		"imag":    &symbol{kind: bltnSym, builtin: _imag},
		"copy":    &symbol{kind: bltnSym, builtin: _copy},
		"delete":  &symbol{kind: bltnSym, builtin: _delete},
		"len":     &symbol{kind: bltnSym, builtin: _len},
		"make":    &symbol{kind: bltnSym, builtin: _make},
		"new":     &symbol{kind: bltnSym, builtin: _new},
		"panic":   &symbol{kind: bltnSym, builtin: _panic},
		"print":   &symbol{kind: bltnSym, builtin: _print},
		"println": &symbol{kind: bltnSym, builtin: _println},
		"real":    &symbol{kind: bltnSym, builtin: _real},
		"recover": &symbol{kind: bltnSym, builtin: _recover},
	}}
	return sc
}

// resizeFrame resizes the global frame of interpreter
func (interp *Interpreter) resizeFrame() {
	l := len(interp.universe.types)
	b := len(interp.frame.data)
	if l-b <= 0 {
		return
	}
	data := make([]reflect.Value, l)
	copy(data, interp.frame.data)
	for j, t := range interp.universe.types[b:] {
		data[b+j] = reflect.New(t).Elem()
	}
	interp.frame.data = data
}

func (interp *Interpreter) main() *node {
	if m, ok := interp.scopes[mainID]; ok && m.sym[mainID] != nil {
		return m.sym[mainID].node
	}
	return nil
}

// Eval evaluates Go code represented as a string. It returns a map on
// current interpreted package exported symbols
func (interp *Interpreter) Eval(src string) (reflect.Value, error) {
	var res reflect.Value

	// Parse source to AST
	pkgName, root, err := interp.ast(src, interp.Name)
	if err != nil || root == nil {
		return res, err
	}

	if interp.astDot {
		root.astDot(dotX(), interp.Name)
		if interp.noRun {
			return res, err
		}
	}

	// Global type analysis
	if err = interp.gta(root, pkgName); err != nil {
		return res, err
	}

	// Annotate AST with CFG infos
	initNodes, err := interp.cfg(root)
	if err != nil {
		return res, err
	}

	// Add main to list of functions to run, after all inits
	if m := interp.main(); m != nil {
		initNodes = append(initNodes, m)
	}

	if root.kind != fileStmt {
		// REPL may skip package statement
		setExec(root.start)
	}
	if interp.universe.sym[pkgName] == nil {
		// Make the package visible under a path identical to its name
		interp.universe.sym[pkgName] = &symbol{typ: &itype{cat: srcPkgT}, path: pkgName}
	}

	if interp.cfgDot {
		root.cfgDot(dotX())
	}

	if interp.noRun {
		return res, err
	}

	// Execute CFG
	if err = genRun(root); err != nil {
		return res, err
	}
	interp.resizeFrame()
	interp.run(root, nil)

	for _, n := range initNodes {
		interp.run(n, interp.frame)
	}
	v := genValue(root)
	res = v(interp.frame)

	// If result is an interpreter node, wrap it in a runtime callable function
	if res.IsValid() {
		if n, ok := res.Interface().(*node); ok {
			res = genFunctionWrapper(n)(interp.frame)
		}
	}

	return res, err
}

// getWrapper returns the wrapper type of the corresponding interface, or nil if not found
func (interp *Interpreter) getWrapper(t reflect.Type) reflect.Type {
	if p, ok := interp.binValue[t.PkgPath()]; ok {
		return p["_"+t.Name()].Type().Elem()
	}
	return nil
}

// Use loads binary runtime symbols in the interpreter context so
// they can be used in interpreted code
func (interp *Interpreter) Use(values PkgSet) {
	for k, v := range values {
		interp.binValue[k] = v
	}
}

// Repl performs a Read-Eval-Print-Loop on input file descriptor.
// Results are printed on output.
func (interp *Interpreter) Repl(in, out *os.File) {
	s := bufio.NewScanner(in)
	prompt := getPrompt(in, out)
	prompt()
	src := ""
	for s.Scan() {
		src += s.Text() + "\n"
		if v, err := interp.Eval(src); err != nil {
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
