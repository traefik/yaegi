package interp

import (
	"log"
	"reflect"
	"strconv"
)

// A sKind represents the kind of symbol.
type sKind uint

// Symbol kinds for the Go interpreter.
const (
	undefSym   sKind = iota
	binSym           // Binary from runtime
	bltnSym          // Builtin
	constSym         // Constant
	funcSym          // Function
	labelSym         // Label
	pkgSym           // Package
	typeSym          // Type
	varTypeSym       // Variable type (generic)
	varSym           // Variable
)

var symKinds = [...]string{
	undefSym:   "undefSym",
	binSym:     "binSym",
	bltnSym:    "bltnSym",
	constSym:   "constSym",
	funcSym:    "funcSym",
	labelSym:   "labelSym",
	pkgSym:     "pkgSym",
	typeSym:    "typeSym",
	varTypeSym: "varTypeSym",
	varSym:     "varSym",
}

func (k sKind) String() string {
	if k < sKind(len(symKinds)) {
		return symKinds[k]
	}
	return "SymKind(" + strconv.Itoa(int(k)) + ")"
}

// A symbol represents an interpreter object such as type, constant, var, func,
// label, builtin or binary object. Symbols are defined within a scope.
type symbol struct {
	kind    sKind
	typ     *itype        // Type of value
	node    *node         // Node value if index is negative
	from    []*node       // list of goto nodes jumping to this label node, or nil
	recv    *receiver     // receiver node value, if sym refers to a method
	index   int           // index of value in frame or -1
	rval    reflect.Value // default value (used for constants)
	builtin bltnGenerator // Builtin function or nil
	global  bool          // true if symbol is defined in global space
}

// scope type stores symbols in maps, and frame layout as array of types
// The purposes of scopes are to manage the visibility of each symbol
// and to store the memory frame layout information (type and index in frame)
// at each level (global, package, functions)
//
// scopes are organized in a stack fashion: a first scope (universe) is created
// once at global level, and for each block (package, func, for, etc...), a new
// scope is pushed at entry, and poped at exit.
//
// Nested scopes with the same level value use the same frame: it allows to have
// exactly one frame per function, with a fixed position for each variable (named
// or not), no matter the inner complexity (number of nested blocks in the function)
//
// In symbols, the index value corresponds to the index in scope.types, and at
// execution to the index in frame, created exactly from the types layout.
type scope struct {
	anc         *scope             // ancestor upper scope
	child       []*scope           // included scopes
	def         *node              // function definition node this scope belongs to, or nil
	loop        *node              // loop exit node for break statement
	loopRestart *node              // loop restart node for continue statement
	pkgID       string             // unique id of package in which scope is defined
	pkgName     string             // package name for the package
	types       []reflect.Type     // frame layout, may be shared by same level scopes
	level       int                // frame level: number of frame indirections to access var during execution
	sym         map[string]*symbol // map of symbols defined in this current scope
	global      bool               // true if scope refers to global space (single frame for universe and package level scopes)
	iota        int                // iota value in this scope
}

// push creates a new child scope and chain it to the current one.
func (s *scope) push(indirect bool) *scope {
	sc := &scope{anc: s, level: s.level, sym: map[string]*symbol{}}
	s.child = append(s.child, sc)
	if indirect {
		sc.types = []reflect.Type{}
		sc.level = s.level + 1
	} else {
		// Propagate size, types, def and global as scopes at same level share the same frame.
		sc.types = s.types
		sc.def = s.def
		sc.global = s.global
		sc.level = s.level
	}
	// inherit loop state and pkgID from ancestor
	sc.loop, sc.loopRestart, sc.pkgID = s.loop, s.loopRestart, s.pkgID
	return sc
}

func (s *scope) pushBloc() *scope { return s.push(false) }
func (s *scope) pushFunc() *scope { return s.push(true) }

func (s *scope) pop() *scope {
	if s.level == s.anc.level {
		// Propagate size and types, as scopes at same level share the same frame.
		s.anc.types = s.types
	}
	return s.anc
}

func (s *scope) upperLevel() *scope {
	level := s.level
	for s != nil && s.level == level {
		s = s.anc
	}
	return s
}

// lookup searches for a symbol in the current scope, and upper ones if not found
// it returns the symbol, the number of indirections level from the current scope
// and status (false if no result).
func (s *scope) lookup(ident string) (*symbol, int, bool) {
	level := s.level
	for {
		if sym, ok := s.sym[ident]; ok {
			if sym.global {
				return sym, globalFrame, true
			}
			return sym, level - s.level, true
		}
		if s.anc == nil {
			break
		}
		s = s.anc
	}
	return nil, 0, false
}

func (s *scope) rangeChanType(n *node) *itype {
	if sym, _, found := s.lookup(n.child[1].ident); found {
		if t := sym.typ; len(n.child) == 3 && t != nil && (t.cat == chanT || t.cat == chanRecvT) {
			return t
		}
	}

	c := n.child[1]
	if c.typ == nil {
		return nil
	}
	switch {
	case c.typ.cat == chanT, c.typ.cat == chanRecvT:
		return c.typ
	case c.typ.cat == valueT && c.typ.rtype.Kind() == reflect.Chan:
		dir := chanSendRecv
		switch c.typ.rtype.ChanDir() {
		case reflect.RecvDir:
			dir = chanRecv
		case reflect.SendDir:
			dir = chanSend
		}
		return chanOf(valueTOf(c.typ.rtype.Elem()), dir)
	}

	return nil
}

// fixType returns the input type, or a valid default type for untyped constant.
func (s *scope) fixType(t *itype) *itype {
	if !t.untyped || t.cat != valueT {
		return t
	}
	switch typ := t.TypeOf(); typ.Kind() {
	case reflect.Int64:
		return s.getType("int")
	case reflect.Uint64:
		return s.getType("uint")
	case reflect.Float64:
		return s.getType("float64")
	case reflect.Complex128:
		return s.getType("complex128")
	}
	return t
}

func (s *scope) getType(ident string) *itype {
	var t *itype
	if sym, _, found := s.lookup(ident); found {
		if sym.kind == typeSym {
			t = sym.typ
		}
	}
	return t
}

// add adds a type to the scope types array, and returns its index.
func (s *scope) add(typ *itype) (index int) {
	if typ == nil {
		log.Panic("nil type")
	}
	index = len(s.types)
	t := typ.frameType()
	if t == nil {
		log.Panic("nil reflect type")
	}
	s.types = append(s.types, t)
	return
}

func (interp *Interpreter) initScopePkg(pkgID, pkgName string) *scope {
	sc := interp.universe

	interp.mutex.Lock()
	if _, ok := interp.scopes[pkgID]; !ok {
		interp.scopes[pkgID] = sc.pushBloc()
	}
	sc = interp.scopes[pkgID]
	sc.pkgID = pkgID
	sc.pkgName = pkgName
	interp.mutex.Unlock()
	return sc
}

// Globals returns a map of global variables and constants in the main package.
func (interp *Interpreter) Globals() map[string]reflect.Value {
	syms := map[string]reflect.Value{}
	interp.mutex.RLock()
	defer interp.mutex.RUnlock()

	v, ok := interp.srcPkg["main"]
	if !ok {
		return syms
	}

	for n, s := range v {
		switch s.kind {
		case constSym:
			syms[n] = s.rval
		case varSym:
			syms[n] = interp.frame.data[s.index]
		}
	}

	return syms
}
