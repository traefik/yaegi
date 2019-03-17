package interp

import (
	"log"
	"reflect"
	"strconv"
)

// A SymKind represents the kind of symbol
type SymKind uint

// Symbol kinds for the Go interpreter
const (
	Bin   SymKind = iota // Binary from runtime
	Bltn                 // Builtin
	Const                // Constant
	Func                 // Function
	Label                // Label
	Typ                  // Type
	Var                  // Variable
)

var symKinds = [...]string{
	Bin:   "Bin",
	Bltn:  "Bltn",
	Const: "Const",
	Func:  "Func",
	Label: "Label",
	Typ:   "Typ",
	Var:   "Var",
}

func (k SymKind) String() string {
	if k < SymKind(len(symKinds)) {
		return symKinds[k]
	}
	return "SymKind(" + strconv.Itoa(int(k)) + ")"
}

// A Symbol represents an interpreter object such as type, constant, var, func,
// label, builtin or binary object. Symbols are defined within a scope.
type Symbol struct {
	kind    SymKind
	typ     *Type            // Type of value
	node    *Node            // Node value if index is negative
	from    []*Node          // list of nodes jumping to node if kind is label, or nil
	recv    *Receiver        // receiver node value, if sym refers to a method
	index   int              // index of value in frame or -1
	val     interface{}      // default value (used for constants)
	path    string           // package path if typ.cat is SrcPkgT or BinPkgT
	builtin BuiltinGenerator // Builtin function or nil
	global  bool             // true if symbol is defined in global space
	//constant bool             // true if symbol value is constant
}

// A SymMap stores symbols indexed by name
type SymMap map[string]*Symbol

// Scope type stores symbols in maps, and frame layout as array of types
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
type Scope struct {
	anc    *Scope         // Ancestor upper scope
	def    *Node          // function definition node this scope belongs to, or nil
	types  []reflect.Type // Frame layout, may be shared by same level scopes
	level  int            // Frame level: number of frame indirections to access var during execution
	sym    SymMap         // Map of symbols defined in this current scope
	global bool           // true if scope refers to global space (single frame for universe and package level scopes)
}

// push creates a new scope and chain it to the current one
func (s *Scope) push(indirect bool) *Scope {
	scope := Scope{anc: s, level: s.level, sym: map[string]*Symbol{}}
	if indirect {
		scope.types = []reflect.Type{}
		scope.level = s.level + 1
	} else {
		// propagate size, types, def and global as scopes at same level share the same frame
		scope.types = s.types
		scope.def = s.def
		scope.global = s.global
		scope.level = s.level
	}
	return &scope
}

func (s *Scope) pushBloc() *Scope { return s.push(false) }
func (s *Scope) pushFunc() *Scope { return s.push(true) }

func (s *Scope) pop() *Scope {
	if s.level == s.anc.level {
		// propagate size and types, as scopes at same level share the same frame
		s.anc.types = s.types
	}
	return s.anc
}

// lookup searches for a symbol in the current scope, and upper ones if not found
// it returns the symbol, the number of indirections level from the current scope
// and status (false if no result)
func (s *Scope) lookup(ident string) (*Symbol, int, bool) {
	level := s.level
	for s != nil {
		if sym, ok := s.sym[ident]; ok {
			return sym, level - s.level, true
		}
		s = s.anc
	}
	return nil, 0, false
}

func (s *Scope) getType(ident string) *Type {
	var t *Type
	if sym, _, found := s.lookup(ident); found {
		if sym.kind == Typ {
			t = sym.typ
		}
	}
	return t
}

// add adds a type to the scope types array, and returns its index
func (s *Scope) add(typ *Type) (index int) {
	if typ == nil {
		log.Panic("nil type")
	}
	index = len(s.types)
	var t reflect.Type
	switch typ.cat {
	case FuncT:
		t = reflect.TypeOf((*Node)(nil))
	case InterfaceT:
		t = reflect.TypeOf((*valueInterface)(nil)).Elem()
	default:
		t = typ.TypeOf()
		if t == nil {
			log.Panic("nil reflect type")
		}
	}
	s.types = append(s.types, t)
	return
}
