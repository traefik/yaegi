package interp

import (
	"reflect"
	"strconv"
)

// Cat defines interpreter type categories
type Cat uint

// Types for go language
const (
	NilT Cat = iota
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
	NilT:        "NilT",
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
	name  string
	embed bool
	typ   *Type
}

// Type defines the internal representation of types in the interpreter
type Type struct {
	cat        Cat           // Type category
	field      []StructField // Array of struct fields if StrucT or InterfaceT
	key        *Type         // Type of key element if MapT or nil
	val        *Type         // Type of value element if ChanT, MapT, PtrT, AliasT or ArrayT
	arg        []*Type       // Argument types if FuncT or nil
	ret        []*Type       // Return types if FuncT or nil
	method     []*Node       // Associated methods or nil
	name       string        // name of type within its package for a defined type
	pkgPath    string        // for a defined type, the package import path
	size       int           // Size of array if ArrayT
	rtype      reflect.Type  // Reflection type if ValueT, or nil
	variadic   bool          // true if type is variadic
	incomplete bool          // true if type must be parsed again (out of order declarations)
	untyped    bool          // true for a literal value (string or number)
	node       *Node         // root AST node of type definition
	scope      *Scope        // type declaration scope (in case of re-parse incomplete type)
}

// nodeType returns a type definition for the corresponding AST subtree
func nodeType(interp *Interpreter, scope *Scope, n *Node) (*Type, error) {
	var err CfgError

	if n.typ != nil && !n.typ.incomplete {
		return n.typ, err
	}

	var t = &Type{node: n, scope: scope}

	switch n.kind {
	case Address, StarExpr:
		t.cat = PtrT
		t.val, err = nodeType(interp, scope, n.child[0])
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
			t.val, err = nodeType(interp, scope, n.child[1])
			t.incomplete = t.incomplete || t.val.incomplete
		} else {
			t.val, err = nodeType(interp, scope, n.child[0])
			t.incomplete = t.val.incomplete
		}

	case BasicLit:
		switch n.val.(type) {
		case bool:
			t.cat = BoolT
			t.name = "bool"
		case byte:
			t.cat = ByteT
			t.name = "byte"
			t.untyped = true
		case float32:
			t.cat = Float32T
			t.name = "float32"
			t.untyped = true
		case float64:
			t.cat = Float64T
			t.name = "float64"
			t.untyped = true
		case int:
			t.cat = IntT
			t.name = "int"
			t.untyped = true
		case rune:
			t.cat = RuneT
			t.name = "rune"
			t.untyped = true
		case string:
			t.cat = StringT
			t.name = "string"
			t.untyped = true
		default:
			err = n.cfgError("missign support for type %T", n.val)
		}

	case UnaryExpr:
		t, err = nodeType(interp, scope, n.child[0])

	case BinaryExpr:
		t, err = nodeType(interp, scope, n.child[0])
		if err != nil {
			return nil, err
		}
		if t.untyped {
			var t1 *Type
			t1, err = nodeType(interp, scope, n.child[1])
			if !(t1.untyped && isInt(t1) && isFloat(t)) {
				t = t1
			}
		}

	case CallExpr, CompositeLitExpr:
		t, err = nodeType(interp, scope, n.child[0])

	case ChanType:
		t.cat = ChanT
		t.val, err = nodeType(interp, scope, n.child[0])
		t.incomplete = t.val.incomplete

	case Ellipsis:
		t, err = nodeType(interp, scope, n.child[0])
		t.variadic = true

	case FuncLit:
		t, err = nodeType(interp, scope, n.child[2])

	case FuncType:
		t.cat = FuncT
		// Handle input parameters
		for _, arg := range n.child[0].child {
			cl := len(arg.child) - 1
			typ, err := nodeType(interp, scope, arg.child[cl])
			if err != nil {
				return nil, err
			}
			t.arg = append(t.arg, typ)
			for i := 1; i < cl; i++ {
				// Several arguments may be factorized on the same field type
				t.arg = append(t.arg, typ)
			}
			t.incomplete = t.incomplete || typ.incomplete
		}
		if len(n.child) == 2 {
			// Handle returned values
			for _, ret := range n.child[1].child {
				cl := len(ret.child) - 1
				typ, err := nodeType(interp, scope, ret.child[cl])
				if err != nil {
					return nil, err
				}
				t.ret = append(t.ret, typ)
				for i := 1; i < cl; i++ {
					// Several arguments may be factorized on the same field type
					t.ret = append(t.ret, typ)
				}
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
		for _, field := range n.child[0].child {
			typ, err := nodeType(interp, scope, field.child[1])
			if err != nil {
				return nil, err
			}
			t.field = append(t.field, StructField{name: field.child[0].ident, typ: typ})
			t.incomplete = t.incomplete || typ.incomplete
		}

	case MapType:
		t.cat = MapT
		if t.key, err = nodeType(interp, scope, n.child[0]); err != nil {
			return nil, err
		}
		if t.val, err = nodeType(interp, scope, n.child[1]); err != nil {
			return nil, err
		}
		t.incomplete = t.key.incomplete || t.val.incomplete

	case SelectorExpr:
		pkg, name := n.child[0].ident, n.child[1].ident
		if sym, _, found := scope.lookup(pkg); found {
			if sym.typ == nil {
				t.incomplete = true
				break
			}
			switch sym.typ.cat {
			case BinPkgT:
				pkg := interp.binValue[sym.path]
				if v, ok := pkg[name]; ok {
					t.cat = ValueT
					t.rtype = v.Type()
					if isBinType(v) {
						t.rtype = t.rtype.Elem()
					}
				} else {
					t.incomplete = true
				}

			case SrcPkgT:
				spkg := interp.scope[pkg]
				if st, ok := spkg.sym[name]; ok && st.kind == Typ {
					t = st.typ
				}
			}
		} else {
			err = n.cfgError("undefined package: %s", pkg)
		}
		// TODO: handle pkgsrc types

	case StructType:
		t.cat = StructT
		for _, c := range n.child[0].child {
			if len(c.child) == 1 {
				typ, err := nodeType(interp, scope, c.child[0])
				if err != nil {
					return nil, err
				}
				t.field = append(t.field, StructField{name: c.child[0].ident, embed: true, typ: typ})
				t.incomplete = t.incomplete || typ.incomplete
			} else {
				l := len(c.child)
				typ, err := nodeType(interp, scope, c.child[l-1])
				if err != nil {
					return nil, err
				}
				t.incomplete = t.incomplete || typ.incomplete
				for _, d := range c.child[:l-1] {
					t.field = append(t.field, StructField{name: d.ident, typ: typ})
				}
			}
		}

	default:
		err = n.cfgError("type definition not implemented: %s", n.kind)
	}

	return t, err
}

var zeroValues [MaxT]reflect.Value

func init() {
	zeroValues[BoolT] = reflect.ValueOf(false)
	zeroValues[ByteT] = reflect.ValueOf(byte(0))
	zeroValues[Complex64T] = reflect.ValueOf(complex64(0))
	zeroValues[Complex128T] = reflect.ValueOf(complex128(0))
	zeroValues[ErrorT] = reflect.ValueOf(new(error)).Elem()
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

// if type is incomplete, re-parse it.
func (t *Type) finalize() (*Type, error) {
	var err CfgError
	if t.incomplete {
		t, err = nodeType(t.node.interp, t.scope, t.node)
		t.node.typ = t
		if t.incomplete && err == nil {
			err = t.node.cfgError("incomplete type")
		}
	}
	return t, err
}

// id returns a unique type identificator string
func (t *Type) id() string {
	// TODO: if res is nil, build identity from String()

	res := ""
	if t.cat == ValueT {
		res = t.rtype.PkgPath() + "." + t.rtype.Name()
	} else {
		res = t.pkgPath + "." + t.name
	}
	return res
}

// zero instantiates and return a zero value object for the given type during execution
func (t *Type) zero() (v reflect.Value, err error) {
	if t, err = t.finalize(); err != nil {
		return v, err
	}
	switch t.cat {
	case AliasT:
		v, err = t.val.zero()

	case ArrayT, StructT:
		v = reflect.New(t.TypeOf()).Elem()

	case ValueT:
		v = reflect.New(t.rtype).Elem()

	default:
		v = zeroValues[t.cat]
	}
	return v, err
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

// fieldSeq returns the field type from the list of field indexes
func (t *Type) fieldSeq(seq []int) *Type {
	ft := t
	for _, i := range seq {
		if ft.cat == PtrT {
			ft = ft.val
		}
		ft = ft.field[i].typ
	}
	return ft
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
		return t.val.lookupMethod(name)
	}
	var index []int
	m := t.getMethod(name)
	if m == nil {
		for i, f := range t.field {
			if f.embed {
				if n, index2 := f.typ.lookupMethod(name); n != nil {
					index = append([]int{i}, index2...)
					return n, index
				}
			}
		}
	}
	return m, index
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
		}
		return reflect.SliceOf(t.val.TypeOf())

	case BinPkgT, BuiltinT, InterfaceT, SrcPkgT:
		return nil

	case ChanT:
		return reflect.ChanOf(reflect.BothDir, t.val.TypeOf())

	case ErrorT:
		return reflect.TypeOf(new(error)).Elem()

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
		var fields []reflect.StructField
		for _, f := range t.field {
			field := reflect.StructField{Name: exportName(f.name), Type: f.typ.TypeOf()}
			fields = append(fields, field)
		}
		return reflect.StructOf(fields)

	case ValueT:
		return t.rtype

	default:
		z, _ := t.zero()
		if z.IsValid() {
			return z.Type()
		}
		var empty reflect.Type
		return empty
	}
}

func isInt(t *Type) bool {
	typ := t.TypeOf()
	if typ == nil {
		return false
	}
	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

func isUint(t *Type) bool {
	typ := t.TypeOf()
	if typ == nil {
		return false
	}
	switch typ.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

func isFloat(t *Type) bool {
	typ := t.TypeOf()
	if typ == nil {
		return false
	}
	switch typ.Kind() {
	case reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func isString(t *Type) bool {
	return t.TypeOf().Kind() == reflect.String
}
