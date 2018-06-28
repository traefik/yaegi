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
	Unset Cat = iota
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
)

var cats = [...]string{
	Unset:       "Unset",
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
	cat      Cat           // Type category
	field    []StructField // Array of struct fields if StrucT or nil
	key      *Type         // Type of key element if MapT or nil
	val      *Type         // Type of value element if ChanT, MapT, PtrT, AliasT or ArrayT
	arg      []*Type       // Argument types if FuncT or nil
	ret      []*Type       // Return types if FuncT or nil
	method   []*Node       // Associated methods or nil
	rtype    reflect.Type  // Reflection type if ValueT, or nil
	rzero    reflect.Value // Reflection zero settable value, or nil
	variadic bool          // true if type is variadic
	nindex   int           // node index (for debug only)
}

// TypeMap defines a map of Types indexed by type names
type TypeMap map[string]*Type

var defaultTypes TypeMap = map[string]*Type{
	"bool":       &Type{cat: BoolT},
	"byte":       &Type{cat: ByteT},
	"complex64":  &Type{cat: Complex64T},
	"complex128": &Type{cat: Complex128T},
	"error":      &Type{cat: ErrorT},
	"float32":    &Type{cat: Float32T},
	"float64":    &Type{cat: Float64T},
	"int":        &Type{cat: IntT},
	"int8":       &Type{cat: Int8T},
	"int16":      &Type{cat: Int16T},
	"int32":      &Type{cat: Int32T},
	"int64":      &Type{cat: Int64T},
	"rune":       &Type{cat: RuneT},
	"string":     &Type{cat: StringT},
	"uint":       &Type{cat: UintT},
	"uint8":      &Type{cat: Uint8T},
	"uint16":     &Type{cat: Uint16T},
	"uint32":     &Type{cat: Uint32T},
	"uint64":     &Type{cat: Uint64T},
	"uintptr":    &Type{cat: UintptrT},
}

// return a type definition for the corresponding AST subtree
func nodeType(interp *Interpreter, n *Node) *Type {
	if n.typ != nil {
		return n.typ
	}
	var t = &Type{nindex: n.index}
	switch n.kind {
	case ArrayType:
		t.cat = ArrayT
		t.val = nodeType(interp, n.child[0])
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
	case ChanType:
		t.cat = ChanT
		t.val = nodeType(interp, n.child[0])
	case Ellipsis:
		t = nodeType(interp, n.child[0])
		t.variadic = true
	case FuncType:
		t.cat = FuncT
		for _, arg := range n.child[0].child {
			t.arg = append(t.arg, nodeType(interp, arg.child[len(arg.child)-1]))
		}
		if len(n.child) == 2 {
			for _, ret := range n.child[1].child {
				t.ret = append(t.ret, nodeType(interp, ret.child[len(ret.child)-1]))
			}
		}
	case Ident:
		t = interp.types[n.ident]
	case InterfaceType:
		t.cat = InterfaceT
		//for _, method := range n.child[0].child {
		//	t.method = append(t.method, nodeType(interp, method))
		//}
	case MapType:
		t.cat = MapT
		t.key = nodeType(interp, n.child[0])
		t.val = nodeType(interp, n.child[1])
	case SelectorExpr:
		pkgName, typeName := n.child[0].ident, n.child[1].ident
		if pkg, ok := interp.binPkg[pkgName]; ok {
			if typ, ok := (*pkg)[typeName]; ok {
				t.cat = ValueT
				t.rtype = reflect.TypeOf(typ).Elem()
				log.Println("found bin type", t.rtype)
			}
		}
	case StarExpr:
		t.cat = PtrT
		t.val = nodeType(interp, n.child[0])
	case StructType:
		t.cat = StructT
		for _, c := range n.child[0].child {
			if len(c.child) == 1 {
				t.field = append(t.field, StructField{typ: nodeType(interp, c.child[0])})
			} else {
				l := len(c.child)
				typ := nodeType(interp, c.child[l-1])
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

var zeroValues = [...]interface{}{
	BoolT:       false,
	ByteT:       byte(0),
	Complex64T:  complex64(0),
	Complex128T: complex128(0),
	ErrorT:      error(nil),
	Float32T:    float32(0),
	Float64T:    float64(0),
	IntT:        int(0),
	Int8T:       int8(0),
	Int16T:      int16(0),
	Int32T:      int32(0),
	Int64T:      int64(0),
	RuneT:       rune(0),
	StringT:     "",
	UintT:       uint(0),
	Uint8T:      uint8(0),
	Uint16T:     uint16(0),
	Uint32T:     uint32(0),
	Uint64T:     uint64(0),
	UintptrT:    uintptr(0),
	ValueT:      nil,
}

// zero instantiates and return a zero value object for the givent type t
func (t *Type) zero() interface{} {
	if t.cat >= Cat(len(zeroValues)) {
		return nil
	}
	switch t.cat {
	case AliasT:
		return t.val.zero()
	case StructT:
		z := make([]interface{}, len(t.field))
		for i, f := range t.field {
			z[i] = f.typ.zero()
		}
		return z
	case ValueT:
		return t.rzero
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

// lookupField return a list of indices, i.e. a path to access a field in a struct object
func (t *Type) lookupField(name string) []int {
	var index []int
	if fi := t.fieldIndex(name); fi < 0 {
		for i, f := range t.field {
			if f.name == "" {
				if index2 := f.typ.lookupField(name); len(index2) > 0 {
					index = append([]int{i}, index2...)
					break
				}
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

// TypeOf returns the reflection type of dynamic interpreter type t.
func (t *Type) TypeOf() reflect.Type {
	switch t.cat {
	case ArrayT:
		return reflect.SliceOf(t.val.TypeOf())
	case ChanT:
		return reflect.ChanOf(reflect.BothDir, t.val.TypeOf())
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
			if !canExport(f.name) {
				continue
			}
			fields = append(fields, reflect.StructField{Name: f.name, Type: f.typ.TypeOf()})
		}
		return reflect.StructOf(fields)
	case ValueT:
		return t.rtype
	default:
		return reflect.TypeOf(t.zero())
	}
}
