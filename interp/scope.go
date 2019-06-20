package interp

import (
	"log"
	"reflect"
	"strconv"
)

// A sKind represents the kind of symbol
type sKind uint

// Symbol kinds for the Go interpreter
const (
	undefSym sKind = iota
	binSym         // Binary from runtime
	bltnSym        // Builtin
	constSym       // Constant
	funcSym        // Function
	labelSym       // Label
	pkgSym         // Package
	typeSym        // Type
	varSym         // Variable
)

var symKinds = [...]string{
	undefSym: "undefSym",
	binSym:   "binSym",
	bltnSym:  "bltnSym",
	constSym: "constSym",
	funcSym:  "funcSym",
	labelSym: "labelSym",
	pkgSym:   "pkgSym",
	typeSym:  "typeSym",
	varSym:   "varSym",
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
	from    []*node       // list of nodes jumping to node if kind is label, or nil
	recv    *receiver     // receiver node value, if sym refers to a method
	index   int           // index of value in frame or -1
	rval    reflect.Value // default value (used for constants)
	path    string        // package path if typ.cat is SrcPkgT or BinPkgT
	builtin bltnGenerator // Builtin function or nil
	global  bool          // true if symbol is defined in global space
	//constant bool             // true if symbol value is constant
}

// A symMap stores symbols indexed by name
type symMap map[string]*symbol

// scope type stores symbols in maps, and frame layout as array of types
// The purposes of scopes are to manage the visibility of each symbol
// and to store the memory frame layout informations (type and index in frame)
// at each level (global, package, functions)
//
// scopes are organized in a stack fashion: a first scope (universe) is created
// once at global level, and for each block (package, func, for, etc...), a new
// scope is pushed at entry, and poped at exit.
//
// Nested scopes with the same level value use the same frame: it allows to have
// eaxctly one frame per function, with a fixed position for each variable (named
// or not), no matter the inner complexity (number of nested blocks in the function)
//
// In symbols, the index value corresponds to the index in scope.types, and at
// execution to the index in frame, created exactly from the types layout.
//
type scope struct {
	anc    *scope         // Ancestor upper scope
	def    *node          // function definition node this scope belongs to, or nil
	types  []reflect.Type // Frame layout, may be shared by same level scopes
	level  int            // Frame level: number of frame indirections to access var during execution
	sym    symMap         // Map of symbols defined in this current scope
	global bool           // true if scope refers to global space (single frame for universe and package level scopes)
}

// push creates a new scope and chain it to the current one
func (s *scope) push(indirect bool) *scope {
	sc := scope{anc: s, level: s.level, sym: map[string]*symbol{}}
	if indirect {
		sc.types = []reflect.Type{}
		sc.level = s.level + 1
	} else {
		// propagate size, types, def and global as scopes at same level share the same frame
		sc.types = s.types
		sc.def = s.def
		sc.global = s.global
		sc.level = s.level
	}
	return &sc
}

func (s *scope) pushBloc() *scope { return s.push(false) }
func (s *scope) pushFunc() *scope { return s.push(true) }

func (s *scope) pop() *scope {
	if s.level == s.anc.level {
		// propagate size and types, as scopes at same level share the same frame
		s.anc.types = s.types
	}
	return s.anc
}

// lookup searches for a symbol in the current scope, and upper ones if not found
// it returns the symbol, the number of indirections level from the current scope
// and status (false if no result)
func (s *scope) lookup(ident string) (*symbol, int, bool) {
	level := s.level
	for s != nil {
		if sym, ok := s.sym[ident]; ok {
			return sym, level - s.level, true
		}
		s = s.anc
	}
	return nil, 0, false
}

func (s *scope) rangeChanType(n *node) *itype {
	if sym, _, found := s.lookup(n.child[1].ident); found {
		if t := sym.typ; len(n.child) == 3 && t != nil && t.cat == chanT {
			return t
		}
	}
	return nil
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

// add adds a type to the scope types array, and returns its index
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

func (interp *Interpreter) initScopePkg(n *node) (*scope, string) {
	sc := interp.universe
	pkgName := mainID

	if n.kind == fileStmt {
		pkgName = n.child[0].ident
	}

	if _, ok := interp.scopes[pkgName]; !ok {
		interp.scopes[pkgName] = sc.pushBloc()
	}

	sc = interp.scopes[pkgName]
	return sc, pkgName
}
