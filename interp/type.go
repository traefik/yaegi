package interp

import (
	"log"
	"reflect"
	"strconv"
)

// Cat defines interpreter type categories
type Cat uint

// Types for go language
const (
	UnsetT Cat = iota
	AliasT
	ArrayT
	BinT
	BinPkgT
	BoolT
	BuiltinT
	ByteT
	ChanT
	Complex64T
	Complex128T
	ErrorT
	Float32T
	Float64T
	FuncT
	InterfaceT
	IntT
	Int8T
	Int16T
	Int32T
	Int64T
	MapT
	PtrT
	RuneT
	SrcPkgT
	StringT
	StructT
	UintT
	Uint8T
	Uint16T
	Uint32T
	Uint64T
	UintptrT
	ValueT
	MaxT
)

var cats = [...]string{
	UnsetT:      "UnsetT",
	AliasT:      "AliasT",
	ArrayT:      "ArrayT",
	BinT:        "BinT",
	BinPkgT:     "BinPkgT",
	ByteT:       "ByteT",
	BoolT:       "BoolT",
	BuiltinT:    "BuiltinT",
	ChanT:       "ChanT",
	Complex64T:  "Complex64T",
	Complex128T: "Complex128T",
	ErrorT:      "ErrorT",
	Float32T:    "Float32",
	Float64T:    "Float64T",
	FuncT:       "FuncT",
	InterfaceT:  "InterfaceT",
	IntT:        "IntT",
	Int8T:       "Int8T",
	Int16T:      "Int16T",
	Int32T:      "Int32T",
	Int64T:      "Int64T",
	MapT:        "MapT",
	PtrT:        "PtrT",
	RuneT:       "RuneT",
	SrcPkgT:     "SrcPkgT",
	StringT:     "StringT",
	StructT:     "StructT",
	UintT:       "UintT",
	Uint8T:      "Uint8T",
	Uint16T:     "Uint16T",
	Uint32T:     "Uint32T",
	Uint64T:     "Uint64T",
	UintptrT:    "UintptrT",
	ValueT:      "ValueT",
}

func (c Cat) String() string {
	if c < Cat(len(cats)) {
		return cats[c]
	}
	return "Cat(" + strconv.Itoa(int(c)) + ")"
}

// StructField type defines a field in a struct
type StructField struct {
	name string
	typ  *Type
}

// Type defines the internal representation of types in the interpreter
type Type struct {
	cat        Cat           // Type category
	field      []StructField // Array of struct fields if StrucT or nil
	key        *Type         // Type of key element if MapT or nil
	val        *Type         // Type of value element if ChanT, MapT, PtrT, AliasT or ArrayT
	arg        []*Type       // Argument types if FuncT or nil
	ret        []*Type       // Return types if FuncT or nil
	method     []*Node       // Associated methods or nil
	size       int           // Size of array if ArrayT
	rtype      reflect.Type  // Reflection type if ValueT, or nil
	variadic   bool          // true if type is variadic
	incomplete bool          // true if type must be parsed again
	node       *Node         // root AST node of type definition
	scope      *Scope
}

// return a type definition for the corresponding AST subtree
func nodeType(interp *Interpreter, scope *Scope, n *Node) *Type {
	if n.typ != nil && !n.typ.incomplete {
		return n.typ
	}

	var t = &Type{node: n, scope: scope}

	switch n.kind {
	case Address, StarExpr:
		t.cat = PtrT
		t.val = nodeType(interp, scope, n.child[0])
		t.incomplete = t.val.incomplete

	case ArrayType:
		t.cat = ArrayT
		if len(n.child) > 1 {
			// An array size is defined
			var ok bool
			if t.size, ok = n.child[0].val.(int); !ok {
				if sym, _, ok := scope.lookup(n.child[0].ident); ok {
					// Resolve symbol to get size value
					if sym.typ != nil && sym.typ.cat == IntT {
						if t.size, ok = sym.val.(int); !ok {
							t.incomplete = true
						}
					} else {
						t.incomplete = true
					}
				} else {
					t.incomplete = true
				}
			}
			t.val = nodeType(interp, scope, n.child[1])
			t.incomplete = t.incomplete || t.val.incomplete
		} else {
			t.val = nodeType(interp, scope, n.child[0])
			t.incomplete = t.val.incomplete
		}

	case BasicLit:
		switch n.val.(type) {
		case bool:
			t.cat = BoolT
		case byte:
			t.cat = ByteT
		case float32:
			t.cat = Float32T
		case float64:
			t.cat = Float64T
		case int:
			t.cat = IntT
		case string:
			t.cat = StringT
		default:
			log.Panicf("Missing support for basic type %T, node %v\n", n.val, n.index)
		}

	case CallExpr, CompositeLitExpr:
		t = nodeType(interp, scope, n.child[0])

	case ChanType:
		t.cat = ChanT
		t.val = nodeType(interp, scope, n.child[0])
		t.incomplete = t.val.incomplete

	case Ellipsis:
		t = nodeType(interp, scope, n.child[0])
		t.variadic = true

	case FuncType:
		t.cat = FuncT
		for _, arg := range n.child[0].child {
			typ := nodeType(interp, scope, arg.child[len(arg.child)-1])
			t.arg = append(t.arg, typ)
			t.incomplete = t.incomplete || typ.incomplete
		}
		if len(n.child) == 2 {
			for _, ret := range n.child[1].child {
				typ := nodeType(interp, scope, ret.child[len(ret.child)-1])
				t.ret = append(t.ret, typ)
				t.incomplete = t.incomplete || typ.incomplete
			}
		}

	case Ident:
		if sym, _, found := scope.lookup(n.ident); found {
			t = sym.typ
		} else {
			t.incomplete = true
		}

	case InterfaceType:
		t.cat = InterfaceT
		//for _, method := range n.child[0].child {
		//	t.method = append(t.method, nodeType(interp, scope, method))
		//}

	case MapType:
		t.cat = MapT
		t.key = nodeType(interp, scope, n.child[0])
		t.val = nodeType(interp, scope, n.child[1])
		t.incomplete = t.key.incomplete || t.val.incomplete

	case SelectorExpr:
		pkgName, typeName := n.child[0].ident, n.child[1].ident
		if sym, _, found := scope.lookup(pkgName); found {
			if sym.typ != nil && sym.typ.cat == BinPkgT {
				pkg := interp.binType[sym.path]
				if typ, ok := pkg[typeName]; ok {
					t.cat = ValueT
					t.rtype = typ
				} else {
					t.incomplete = true
				}
			}
		} else {
			log.Panicln("unknown package", pkgName)
		}
		// TODO: handle pkgsrc types

	case StructType:
		t.cat = StructT
		for _, c := range n.child[0].child {
			if len(c.child) == 1 {
				typ := nodeType(interp, scope, c.child[0])
				t.field = append(t.field, StructField{name: c.child[0].ident, typ: typ})
				t.incomplete = t.incomplete || typ.incomplete
			} else {
				l := len(c.child)
				typ := nodeType(interp, scope, c.child[l-1])
				t.incomplete = t.incomplete || typ.incomplete
				for _, d := range c.child[:l-1] {
					t.field = append(t.field, StructField{name: d.ident, typ: typ})
				}
			}
		}

	default:
		log.Panicln("type definition not implemented for node", n.index, n.kind)
	}

	return t
}

var zeroValues [MaxT]reflect.Value

func init() {
	zeroValues[BoolT] = reflect.ValueOf(false)
	zeroValues[ByteT] = reflect.ValueOf(byte(0))
	zeroValues[Complex64T] = reflect.ValueOf(complex64(0))
	zeroValues[Complex128T] = reflect.ValueOf(complex128(0))
	zeroValues[ErrorT] = reflect.ValueOf(error(nil))
	zeroValues[Float32T] = reflect.ValueOf(float32(0))
	zeroValues[Float64T] = reflect.ValueOf(float64(0))
	zeroValues[IntT] = reflect.ValueOf(int(0))
	zeroValues[Int8T] = reflect.ValueOf(int8(0))
	zeroValues[Int16T] = reflect.ValueOf(int16(0))
	zeroValues[Int32T] = reflect.ValueOf(int32(0))
	zeroValues[Int64T] = reflect.ValueOf(int64(0))
	zeroValues[RuneT] = reflect.ValueOf(rune(0))
	zeroValues[StringT] = reflect.ValueOf("")
	zeroValues[UintT] = reflect.ValueOf(uint(0))
	zeroValues[Uint8T] = reflect.ValueOf(uint8(0))
	zeroValues[Uint16T] = reflect.ValueOf(uint16(0))
	zeroValues[Uint32T] = reflect.ValueOf(uint32(0))
	zeroValues[Uint64T] = reflect.ValueOf(uint64(0))
	zeroValues[UintptrT] = reflect.ValueOf(uintptr(0))
}

// zero instantiates and return a zero value object for the given type during execution
func (t *Type) zero() reflect.Value {
	if t.incomplete {
		// Re-parse type at execution in lazy mode. Hopefully missing info is now present
		t = nodeType(t.node.interp, t.scope, t.node)
		t.node.typ = t
		if t.incomplete {
			log.Panicln("incomplete type", t.node.index)
		}
	}

	switch t.cat {
	case AliasT:
		return t.val.zero()

	case ArrayT, StructT:
		return reflect.New(t.TypeOf()).Elem()

	case ValueT:
		return reflect.New(t.rtype).Elem()

	default:
		return zeroValues[t.cat]
	}
}

// fieldType returns the field type of a struct or *struct type
func (t *Type) fieldType(index int) *Type {
	if t.cat == PtrT {
		return t.val.fieldType(index)
	}
	return t.field[index].typ
}

// fieldIndex returns the field index from name in a struct, or -1 if not found
func (t *Type) fieldIndex(name string) int {
	if t.cat == PtrT {
		return t.val.fieldIndex(name)
	}
	for i, field := range t.field {
		if name == field.name {
			return i
		}
	}
	return -1
}

// lookupField returns a list of indices, i.e. a path to access a field in a struct object
func (t *Type) lookupField(name string) []int {
	var index []int
	if fi := t.fieldIndex(name); fi < 0 {
		for i, f := range t.field {
			if f.typ.cat != StructT {
				continue
			}
			if index2 := f.typ.lookupField(name); len(index2) > 0 {
				index = append([]int{i}, index2...)
				break
			}
		}
	} else {
		index = append(index, fi)
	}
	return index
}

// getMethod returns a pointer to the method definition
func (t *Type) getMethod(name string) *Node {
	for _, m := range t.method {
		if name == m.ident {
			return m
		}
	}
	return nil
}

// lookupMethod returns a pointer to method definition associated to type t
// and the list of indices to access the right struct field, in case of a promoted method
func (t *Type) lookupMethod(name string) (*Node, []int) {
	if t.cat == PtrT {
		m, index := t.val.lookupMethod(name)
		return m, index
	}
	var index []int
	if m := t.getMethod(name); m == nil {
		for i, f := range t.field {
			if f.name == "" {
				if m, index2 := f.typ.lookupMethod(name); m != nil {
					index = append([]int{i}, index2...)
					return m, index
				}
			}
		}
	} else {
		return m, index
	}
	return nil, index
}

// ptrTo returns the pointer type with element t.
func ptrTo(t *Type) *Type {
	return &Type{cat: PtrT, val: t}
}

func exportName(s string) string {
	if canExport(s) {
		return s
	}
	return "X" + s
}

// TypeOf returns the reflection type of dynamic interpreter type t.
func (t *Type) TypeOf() reflect.Type {
	switch t.cat {
	case ArrayT:
		if t.size > 0 {
			return reflect.ArrayOf(t.size, t.val.TypeOf())
		} else {
			return reflect.SliceOf(t.val.TypeOf())
		}

	case ChanT:
		return reflect.ChanOf(reflect.BothDir, t.val.TypeOf())

	case ErrorT:
		var e = new(error)
		return reflect.TypeOf(e).Elem()

	case FuncT:
		in := make([]reflect.Type, len(t.arg))
		out := make([]reflect.Type, len(t.ret))
		for i, v := range t.arg {
			in[i] = v.TypeOf()
		}
		for i, v := range t.ret {
			out[i] = v.TypeOf()
		}
		return reflect.FuncOf(in, out, false)

	case MapT:
		return reflect.MapOf(t.key.TypeOf(), t.val.TypeOf())

	case PtrT:
		return reflect.PtrTo(t.val.TypeOf())

	case StructT:
		var fields = []reflect.StructField{}
		for _, f := range t.field {
			field := reflect.StructField{Name: exportName(f.name), Type: f.typ.TypeOf()}
			fields = append(fields, field)
		}
		return reflect.StructOf(fields)

	case ValueT:
		return t.rtype

	default:
		return t.zero().Type()
	}
}
