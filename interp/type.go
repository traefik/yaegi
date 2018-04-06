package interp

import (
	"strconv"
)

// Type categories
type Cat int

const (
	Unset = Cat(iota)
	AliasT
	ArrayT
	BoolT
	ChanT
	Float64T
	FuncT
	InterfaceT
	IntT
	MapT
	PkgT
	StringT
	StructT
)

var cats = [...]string{
	Unset:      "Unset",
	AliasT:     "AliasT",
	ArrayT:     "ArrayT",
	BoolT:      "BoolT",
	ChanT:      "ChanT",
	Float64T:   "Float64T",
	FuncT:      "FuncT",
	InterfaceT: "InterfaceT",
	IntT:       "IntT",
	MapT:       "MapT",
	PkgT:       "PkgT",
	StringT:    "StringT",
	StructT:    "StructT",
}

func (c Cat) String() string {
	if 0 <= c && c <= Cat(len(cats)) {
		return cats[c]
	}
	return "Cat(" + strconv.Itoa(int(c)) + ")"
}

type StructField struct {
	name string
	typ  *Type
}

// Representation of types in interpreter
type Type struct {
	cat    Cat           // Type category
	field  []StructField // Array of struct fields if StrucT or nil
	key    *Type         // Type of key element if MapT or nil
	val    *Type         // Type of value element if ChanT, MapT, AliasT or ArrayT
	arg    []*Type       // Argument types if FuncT or nil
	ret    []*Type       // Return types if FuncT or nil
	method []*Node       // Associated methods or nil
}

type TypeDef map[string]*Type

// Initialize Go basic types
func initTypes() TypeDef {
	return map[string]*Type{
		"bool":    &Type{cat: BoolT},
		"float64": &Type{cat: Float64T},
		"int":     &Type{cat: IntT},
		"string":  &Type{cat: StringT},
	}
}

// return a type definition for the corresponding AST subtree
func nodeType(tdef TypeDef, n *Node) *Type {
	var t *Type = &Type{}
	switch n.kind {
	case ArrayType:
		t.cat = ArrayT
		t.val = nodeType(tdef, n.Child[0])
	case ChanType:
		t.cat = ChanT
		t.val = nodeType(tdef, n.Child[0])
	case FuncType:
		t.cat = FuncT
		for _, arg := range n.Child[0].Child {
			t.arg = append(t.arg, nodeType(tdef, arg.Child[len(arg.Child)-1]))
		}
		if len(n.Child) == 2 {
			for _, ret := range n.Child[1].Child {
				t.ret = append(t.ret, nodeType(tdef, ret.Child[len(ret.Child)-1]))
			}
		}
	case Ident:
		t = tdef[n.ident]
	case MapType:
		t.cat = MapT
		t.key = nodeType(tdef, n.Child[0])
		t.val = nodeType(tdef, n.Child[1])
	case StructType:
		t.cat = StructT
		for _, c := range n.Child[0].Child {
			if len(c.Child) == 1 {
				t.field = append(t.field, StructField{typ: nodeType(tdef, c.Child[0])})
			} else {
				l := len(c.Child)
				typ := nodeType(tdef, c.Child[l-1])
				for _, d := range c.Child[:l-1] {
					t.field = append(t.field, StructField{name: d.ident, typ: typ})
				}
			}
		}
	}
	return t
}

// t.zero() instantiates and return a zero value object for the givent type t
func (t *Type) zero() interface{} {
	switch t.cat {
	case AliasT:
		return t.val.zero()
	case BoolT:
		return false
	case Float64T:
		return 0.0
	case IntT:
		return 0
	case StringT:
		return ""
	case StructT:
		z := make([]interface{}, len(t.field))
		for i, f := range t.field {
			z[i] = f.typ.zero()
		}
		return &z
	}
	return nil
}

// return the field index from name in a struct, or -1 if not found
func (t *Type) fieldIndex(name string) int {
	for i, field := range t.field {
		if name == field.name {
			return i
		}
	}
	return -1
}

// t.lookupField(name) return a list of indices, i.e. a path to access a field in a struct object
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

// t.getMethod(name) returns a pointer to the method definition
func (t *Type) getMethod(name string) *Node {
	for _, m := range t.method {
		if name == m.ident {
			return m
		}
	}
	return nil
}

// t.lookupMethod(name) returns a pointer to method definition associated to type t
// and the list of indices to access the right struct field, in case of a promoted method
func (t *Type) lookupMethod(name string) (*Node, []int) {
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
