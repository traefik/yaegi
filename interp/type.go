package interp

import (
	"strconv"
)

// Type categories
type Cat int

const (
	Unset = iota
	ArrayT
	BasicT
	ChanT
	FuncT
	InterfaceT
	MapT
	StructT
)

var cats = [...]string{
	Unset:      "Unset",
	ArrayT:     "ArrayT",
	BasicT:     "BasicT",
	ChanT:      "ChanT",
	InterfaceT: "InterfaceT",
	MapT:       "MapT",
	StructT:    "StructT",
}

func (c Cat) String() string {
	if 0 <= c && c <= Cat(len(cats)) {
		return cats[c]
	}
	return "Cat(" + strconv.Itoa(int(c)) + ")"
}

// Representation of types in interpreter
type Type struct {
	name     string  // Type name, of field name if used in struct
	index    int     // Index in containing struct, for access in frame
	cat      Cat     // Type category
	embedded bool    // True if struct is embedded
	field    []*Type // Array of fields if StructT
	basic    *Type   // Pointer to existing basic type if BasicT
	key      *Type   // Type of key element if MapT
	val      *Type   // Type of value element if ChanT, MapT or ArrayT
	method   []*Node // Associated methods
}

type TypeDef map[string]*Type

// Initialize Go basic types
func initTypes() TypeDef {
	var tdef TypeDef = make(map[string]*Type)
	tdef["bool"] = &Type{name: "bool", cat: BasicT}
	tdef["bool"].basic = tdef["bool"]
	tdef["float64"] = &Type{name: "float64", cat: BasicT}
	tdef["float64"].basic = tdef["float64"]
	tdef["int"] = &Type{name: "int", cat: BasicT}
	tdef["int"].basic = tdef["int"]
	tdef["string"] = &Type{name: "string", cat: BasicT}
	tdef["string"].basic = tdef["string"]
	return tdef
}

// return a type definition for the corresponding AST subtree
// TODO: complete and replace nodeType
func nodeType2(tdef TypeDef, n *Node) *Type {
	var t *Type = &Type{}
	switch n.kind {
	case ArrayType:
		t.cat = ArrayT
		t.val = tdef[n.Child[0].ident]
	case MapType:
		t.cat = MapT
		t.key = tdef[n.Child[0].ident]
		t.val = tdef[n.Child[1].ident]
	}
	return t
}

// nodeType(tdef, n) returns an array of type definitions from the corresponding
// AST subtree
func nodeType(tdef TypeDef, n *Node) []*Type {
	l := len(n.Child)
	var res []*Type
	if l == 1 {
		res = append(res, &Type{name: n.Child[0].ident, embedded: true})
	} else {
		for _, c := range n.Child[:l-1] {
			res = append(res, &Type{name: c.ident})
		}
	}
	switch n.Child[l-1].kind {
	case ArrayType:
		for _, t := range res {
			t.cat = ArrayT
			t.val = tdef[n.Child[l-1].Child[0].ident]
		}
	case ChanType:
		for _, t := range res {
			t.cat = ChanT
			t.val = tdef[n.Child[l-1].Child[0].ident]
		}
	case Ident:
		td := tdef[n.Child[l-1].ident]
		for _, t := range res {
			t.cat = td.cat
			switch td.cat {
			case BasicT:
				t.basic = td
			case StructT:
				t.field = td.field
				t.method = td.method
			}
		}
	case MapType:
		for _, t := range res {
			t.cat = MapT
			t.key = tdef[n.Child[l-1].Child[0].ident] // TODO: should recurse on type definition
			t.val = tdef[n.Child[l-1].Child[1].ident] // TODO: should recurse on type definition
		}
	case StructType:
		for _, t := range res {
			t.cat = StructT
			i := 0
			for _, c := range n.Child[l-1].Child[0].Child {
				for _, stype := range nodeType(tdef, c) {
					stype.index = i
					i++
					t.field = append(t.field, stype)
				}
			}
		}
	}
	return res
}

// t.zero() instantiates and return a zero value object for the givent type t
func (t *Type) zero() interface{} {
	switch t.cat {
	case BasicT:
		switch t.basic.name {
		case "bool":
			return false
		case "float64":
			return 0.0
		case "int":
			return 0
		case "string":
			return ""
		}
	case StructT:
		z := make([]interface{}, len(t.field))
		for i, f := range t.field {
			z[i] = f.zero()
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
			if f.embedded {
				if index2 := f.lookupField(name); len(index2) > 0 {
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
			if f.embedded {
				if m, index2 := f.lookupMethod(name); m != nil {
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
