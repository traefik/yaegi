package interp

import (
	"fmt"
	"go/constant"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/traefik/yaegi/internal/unsafe2"
)

// tcat defines interpreter type categories.
type tcat uint

// Types for go language.
const (
	nilT tcat = iota
	arrayT
	binT
	binPkgT
	boolT
	builtinT
	chanT
	chanSendT
	chanRecvT
	comparableT
	complex64T
	complex128T
	constraintT
	errorT
	float32T
	float64T
	funcT
	genericT
	interfaceT
	intT
	int8T
	int16T
	int32T
	int64T
	linkedT
	mapT
	ptrT
	sliceT
	srcPkgT
	stringT
	structT
	uintT
	uint8T
	uint16T
	uint32T
	uint64T
	uintptrT
	valueT
	variadicT
	maxT
)

var cats = [...]string{
	nilT:        "nilT",
	arrayT:      "arrayT",
	binT:        "binT",
	binPkgT:     "binPkgT",
	boolT:       "boolT",
	builtinT:    "builtinT",
	chanT:       "chanT",
	comparableT: "comparableT",
	complex64T:  "complex64T",
	complex128T: "complex128T",
	constraintT: "constraintT",
	errorT:      "errorT",
	float32T:    "float32",
	float64T:    "float64T",
	funcT:       "funcT",
	genericT:    "genericT",
	interfaceT:  "interfaceT",
	intT:        "intT",
	int8T:       "int8T",
	int16T:      "int16T",
	int32T:      "int32T",
	int64T:      "int64T",
	linkedT:     "linkedT",
	mapT:        "mapT",
	ptrT:        "ptrT",
	sliceT:      "sliceT",
	srcPkgT:     "srcPkgT",
	stringT:     "stringT",
	structT:     "structT",
	uintT:       "uintT",
	uint8T:      "uint8T",
	uint16T:     "uint16T",
	uint32T:     "uint32T",
	uint64T:     "uint64T",
	uintptrT:    "uintptrT",
	valueT:      "valueT",
	variadicT:   "variadicT",
}

func (c tcat) String() string {
	if c < tcat(len(cats)) {
		return cats[c]
	}
	return "Cat(" + strconv.Itoa(int(c)) + ")"
}

// structField type defines a field in a struct.
type structField struct {
	name  string
	tag   string
	embed bool
	typ   *itype
}

// itype defines the internal representation of types in the interpreter.
type itype struct {
	cat          tcat          // Type category
	field        []structField // Array of struct fields if structT or interfaceT
	key          *itype        // Type of key element if MapT or nil
	val          *itype        // Type of value element if chanT, chanSendT, chanRecvT, mapT, ptrT, linkedT, arrayT, sliceT, variadicT or genericT
	recv         *itype        // Receiver type for funcT or nil
	arg          []*itype      // Argument types if funcT or nil
	ret          []*itype      // Return types if funcT or nil
	ptr          *itype        // Pointer to this type. Might be nil
	method       []*node       // Associated methods or nil
	constraint   []*itype      // For interfaceT: list of types part of interface set
	ulconstraint []*itype      // For interfaceT: list of underlying types part of interface set
	instance     []*itype      // For genericT: list of instantiated types
	name         string        // name of type within its package for a defined type
	path         string        // for a defined type, the package import path
	length       int           // length of array if ArrayT
	rtype        reflect.Type  // Reflection type if ValueT, or nil
	node         *node         // root AST node of type definition
	scope        *scope        // type declaration scope (in case of re-parse incomplete type)
	str          string        // String representation of the type
	incomplete   bool          // true if type must be parsed again (out of order declarations)
	untyped      bool          // true for a literal value (string or number)
	isBinMethod  bool          // true if the type refers to a bin method function
}

type generic struct{}

func untypedBool(n *node) *itype {
	return &itype{cat: boolT, name: "bool", untyped: true, str: "untyped bool", node: n}
}

func untypedString(n *node) *itype {
	return &itype{cat: stringT, name: "string", untyped: true, str: "untyped string", node: n}
}

func untypedRune(n *node) *itype {
	return &itype{cat: int32T, name: "int32", untyped: true, str: "untyped rune", node: n}
}

func untypedInt(n *node) *itype {
	return &itype{cat: intT, name: "int", untyped: true, str: "untyped int", node: n}
}

func untypedFloat(n *node) *itype {
	return &itype{cat: float64T, name: "float64", untyped: true, str: "untyped float", node: n}
}

func untypedComplex(n *node) *itype {
	return &itype{cat: complex128T, name: "complex128", untyped: true, str: "untyped complex", node: n}
}

func errorMethodType(sc *scope) *itype {
	return &itype{cat: funcT, ret: []*itype{sc.getType("string")}, str: "func() string"}
}

type itypeOption func(*itype)

func isBinMethod() itypeOption {
	return func(t *itype) {
		t.isBinMethod = true
	}
}

func withRecv(typ *itype) itypeOption {
	return func(t *itype) {
		t.recv = typ
	}
}

func withNode(n *node) itypeOption {
	return func(t *itype) {
		t.node = n
	}
}

func withScope(sc *scope) itypeOption {
	return func(t *itype) {
		t.scope = sc
	}
}

func withUntyped(b bool) itypeOption {
	return func(t *itype) {
		t.untyped = b
	}
}

// valueTOf returns a valueT itype.
func valueTOf(rtype reflect.Type, opts ...itypeOption) *itype {
	t := &itype{cat: valueT, rtype: rtype, str: rtype.String()}
	for _, opt := range opts {
		opt(t)
	}
	if t.untyped {
		t.str = "untyped " + t.str
	}
	return t
}

// wrapperValueTOf returns a valueT itype wrapping an itype.
func wrapperValueTOf(rtype reflect.Type, val *itype, opts ...itypeOption) *itype {
	t := &itype{cat: valueT, rtype: rtype, val: val, str: rtype.String()}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func variadicOf(val *itype, opts ...itypeOption) *itype {
	t := &itype{cat: variadicT, val: val, str: "..." + val.str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// ptrOf returns a pointer to t.
func ptrOf(val *itype, opts ...itypeOption) *itype {
	if val.ptr != nil {
		return val.ptr
	}
	t := &itype{cat: ptrT, val: val, str: "*" + val.str}
	for _, opt := range opts {
		opt(t)
	}
	val.ptr = t
	return t
}

// namedOf returns a named type of val.
func namedOf(val *itype, path, name string, opts ...itypeOption) *itype {
	str := name
	if path != "" {
		str = path + "." + name
	}
	t := &itype{cat: linkedT, val: val, path: path, name: name, str: str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// funcOf returns a function type with the given args and returns.
func funcOf(args []*itype, ret []*itype, opts ...itypeOption) *itype {
	b := []byte{}
	b = append(b, "func("...)
	b = append(b, paramsTypeString(args)...)
	b = append(b, ')')
	if len(ret) != 0 {
		b = append(b, ' ')
		if len(ret) > 1 {
			b = append(b, '(')
		}
		b = append(b, paramsTypeString(ret)...)
		if len(ret) > 1 {
			b = append(b, ')')
		}
	}

	t := &itype{cat: funcT, arg: args, ret: ret, str: string(b)}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

type chanDir uint8

const (
	chanSendRecv chanDir = iota
	chanSend
	chanRecv
)

// chanOf returns a channel of the underlying type val.
func chanOf(val *itype, dir chanDir, opts ...itypeOption) *itype {
	cat := chanT
	str := "chan "
	switch dir {
	case chanSend:
		cat = chanSendT
		str = "chan<- "
	case chanRecv:
		cat = chanRecvT
		str = "<-chan "
	}
	t := &itype{cat: cat, val: val, str: str + val.str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// arrayOf returns am array type of the underlying val with the given length.
func arrayOf(val *itype, l int, opts ...itypeOption) *itype {
	lstr := strconv.Itoa(l)
	t := &itype{cat: arrayT, val: val, length: l, str: "[" + lstr + "]" + val.str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// sliceOf returns a slice type of the underlying val.
func sliceOf(val *itype, opts ...itypeOption) *itype {
	t := &itype{cat: sliceT, val: val, str: "[]" + val.str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// mapOf returns a map type of the underlying key and val.
func mapOf(key, val *itype, opts ...itypeOption) *itype {
	t := &itype{cat: mapT, key: key, val: val, str: "map[" + key.str + "]" + val.str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// interfaceOf returns an interface type with the given fields.
func interfaceOf(t *itype, fields []structField, constraint, ulconstraint []*itype, opts ...itypeOption) *itype {
	str := "interface{}"
	if len(fields) > 0 {
		str = "interface { " + methodsTypeString(fields) + "}"
	}
	if t == nil {
		t = &itype{}
	}
	t.cat = interfaceT
	t.field = fields
	t.constraint = constraint
	t.ulconstraint = ulconstraint
	t.str = str
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// structOf returns a struct type with the given fields.
func structOf(t *itype, fields []structField, opts ...itypeOption) *itype {
	str := "struct {}"
	if len(fields) > 0 {
		str = "struct { " + fieldsTypeString(fields) + "}"
	}
	if t == nil {
		t = &itype{}
	}
	t.cat = structT
	t.field = fields
	t.str = str
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// genericOf returns a generic type.
func genericOf(val *itype, name, path string, opts ...itypeOption) *itype {
	t := &itype{cat: genericT, name: name, path: path, str: name, val: val}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// seenNode determines if a node has been seen.
//
// seenNode treats the slice of nodes as the path traveled down a node
// tree.
func seenNode(ns []*node, n *node) bool {
	for _, nn := range ns {
		if nn == n {
			return true
		}
	}
	return false
}

// nodeType returns a type definition for the corresponding AST subtree.
func nodeType(interp *Interpreter, sc *scope, n *node) (*itype, error) {
	return nodeType2(interp, sc, n, nil)
}

func nodeType2(interp *Interpreter, sc *scope, n *node, seen []*node) (t *itype, err error) {
	if n.typ != nil && !n.typ.incomplete {
		return n.typ, nil
	}
	if sname := typeName(n); sname != "" {
		sym, _, found := sc.lookup(sname)
		if found && sym.kind == typeSym && sym.typ != nil {
			if sym.typ.isComplete() {
				return sym.typ, nil
			}
			if seenNode(seen, n) {
				// We have seen this node in our tree, so it must be recursive.
				sym.typ.incomplete = false
				return sym.typ, nil
			}
		}
	}
	seen = append(seen, n)
	defer func() { seen = seen[:len(seen)-1] }()

	switch n.kind {
	case addressExpr, starExpr:
		val, err := nodeType2(interp, sc, n.child[0], seen)
		if err != nil {
			return nil, err
		}
		t = ptrOf(val, withNode(n), withScope(sc))
		t.incomplete = val.incomplete

	case arrayType:
		c0 := n.child[0]
		if len(n.child) == 1 {
			val, err := nodeType2(interp, sc, c0, seen)
			if err != nil {
				return nil, err
			}
			t = sliceOf(val, withNode(n), withScope(sc))
			t.incomplete = val.incomplete
			break
		}
		// Array size is defined.
		var (
			length     int
			incomplete bool
		)
		switch v := c0.rval; {
		case v.IsValid():
			// Size if defined by a constant literal value.
			if isConstantValue(v.Type()) {
				c := v.Interface().(constant.Value)
				length = constToInt(c)
			} else {
				switch v.Type().Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					length = int(v.Int())
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
					length = int(v.Uint())
				default:
					return nil, c0.cfgErrorf("non integer constant %v", v)
				}
			}
		case c0.kind == ellipsisExpr:
			// [...]T expression, get size from the length of composite array.
			length, err = arrayTypeLen(n.anc, sc)
			if err != nil {
				incomplete = true
			}
		case c0.kind == identExpr:
			sym, _, ok := sc.lookup(c0.ident)
			if !ok {
				incomplete = true
				break
			}
			// Size is defined by a symbol which must be a constant integer.
			if sym.kind != constSym {
				return nil, c0.cfgErrorf("non-constant array bound %q", c0.ident)
			}
			if sym.typ == nil || !isInt(sym.typ.TypeOf()) || !sym.rval.IsValid() {
				incomplete = true
				break
			}
			length = int(vInt(sym.rval))
		default:
			// Size is defined by a numeric constant expression.
			if _, err := interp.cfg(c0, sc, sc.pkgID, sc.pkgName); err != nil {
				if strings.Contains(err.Error(), " undefined: ") {
					incomplete = true
					break
				}
				return nil, err
			}
			v, ok := c0.rval.Interface().(constant.Value)
			if !ok {
				incomplete = true
				break
			}
			length = constToInt(v)
		}
		val, err := nodeType2(interp, sc, n.child[1], seen)
		if err != nil {
			return nil, err
		}
		t = arrayOf(val, length, withNode(n), withScope(sc))
		t.incomplete = incomplete || val.incomplete

	case basicLit:
		switch v := n.rval.Interface().(type) {
		case bool:
			n.rval = reflect.ValueOf(constant.MakeBool(v))
			t = untypedBool(n)
		case rune:
			// It is impossible to work out rune const literals in AST
			// with the correct type so we must make the const type here.
			n.rval = reflect.ValueOf(constant.MakeInt64(int64(v)))
			t = untypedRune(n)
		case constant.Value:
			switch v.Kind() {
			case constant.Bool:
				t = untypedBool(n)
			case constant.String:
				t = untypedString(n)
			case constant.Int:
				t = untypedInt(n)
			case constant.Float:
				t = untypedFloat(n)
			case constant.Complex:
				t = untypedComplex(n)
			default:
				err = n.cfgErrorf("missing support for type %v", n.rval)
			}
		default:
			err = n.cfgErrorf("missing support for type %T: %v", v, n.rval)
		}

	case unaryExpr:
		// In interfaceType, we process an underlying type constraint definition.
		if isInInterfaceType(n) {
			t1, err := nodeType2(interp, sc, n.child[0], seen)
			if err != nil {
				return nil, err
			}
			t = &itype{cat: constraintT, ulconstraint: []*itype{t1}}
			break
		}
		t, err = nodeType2(interp, sc, n.child[0], seen)

	case binaryExpr:
		// In interfaceType, we process a type constraint union definition.
		if isInInterfaceType(n) {
			t = &itype{cat: constraintT, constraint: []*itype{}, ulconstraint: []*itype{}}
			for _, c := range n.child {
				t1, err := nodeType2(interp, sc, c, seen)
				if err != nil {
					return nil, err
				}
				switch t1.cat {
				case constraintT:
					t.constraint = append(t.constraint, t1.constraint...)
					t.ulconstraint = append(t.ulconstraint, t1.ulconstraint...)
				default:
					t.constraint = append(t.constraint, t1)
				}
			}
			break
		}
		// Get type of first operand.
		if t, err = nodeType2(interp, sc, n.child[0], seen); err != nil {
			return nil, err
		}
		// For operators other than shift, get the type from the 2nd operand if the first is untyped.
		if t.untyped && !isShiftNode(n) {
			var t1 *itype
			t1, err = nodeType2(interp, sc, n.child[1], seen)
			if !(t1.untyped && isInt(t1.TypeOf()) && isFloat(t.TypeOf())) {
				t = t1
			}
		}

		// If the node is to be assigned or returned, the node type is the destination type.
		dt := t

		switch a := n.anc; {
		case a.kind == assignStmt && isEmptyInterface(a.child[0].typ):
			// Because an empty interface concrete type "mutates" as different values are
			// assigned to it, we need to make a new itype from scratch everytime a new
			// assignment is made, and not let different nodes (of the same variable) share the
			// same itype. Otherwise they would overwrite each other.
			a.child[0].typ = &itype{cat: interfaceT, val: dt, str: "interface{}"}

		case a.kind == defineStmt && len(a.child) > a.nleft+a.nright:
			if dt, err = nodeType2(interp, sc, a.child[a.nleft], seen); err != nil {
				return nil, err
			}

		case a.kind == returnStmt:
			dt = sc.def.typ.ret[childPos(n)]
		}

		if isInterfaceSrc(dt) {
			// Set a new interface type preserving the concrete type (.val field).
			t2 := *dt
			t2.val = t
			dt = &t2
		}
		t = dt

	case callExpr:
		if isBuiltinCall(n, sc) {
			// Builtin types are special and may depend from their input arguments.
			switch n.child[0].ident {
			case bltnComplex:
				var nt0, nt1 *itype
				if nt0, err = nodeType2(interp, sc, n.child[1], seen); err != nil {
					return nil, err
				}
				if nt1, err = nodeType2(interp, sc, n.child[2], seen); err != nil {
					return nil, err
				}
				if nt0.incomplete || nt1.incomplete {
					t.incomplete = true
				} else {
					switch t0, t1 := nt0.TypeOf(), nt1.TypeOf(); {
					case isFloat32(t0) && isFloat32(t1):
						t = sc.getType("complex64")
					case isFloat64(t0) && isFloat64(t1):
						t = sc.getType("complex128")
					case nt0.untyped && isNumber(t0) && nt1.untyped && isNumber(t1):
						t = untypedComplex(n)
					case nt0.untyped && isFloat32(t1) || nt1.untyped && isFloat32(t0):
						t = sc.getType("complex64")
					case nt0.untyped && isFloat64(t1) || nt1.untyped && isFloat64(t0):
						t = sc.getType("complex128")
					default:
						err = n.cfgErrorf("invalid types %s and %s", t0.Kind(), t1.Kind())
					}
					if nt0.untyped && nt1.untyped {
						t = untypedComplex(n)
					}
				}
			case bltnReal, bltnImag:
				if t, err = nodeType2(interp, sc, n.child[1], seen); err != nil {
					return nil, err
				}
				if !t.incomplete {
					switch k := t.TypeOf().Kind(); {
					case t.untyped && isNumber(t.TypeOf()):
						t = untypedFloat(n)
					case k == reflect.Complex64:
						t = sc.getType("float32")
					case k == reflect.Complex128:
						t = sc.getType("float64")
					default:
						err = n.cfgErrorf("invalid complex type %s", k)
					}
				}
			case bltnCap, bltnCopy, bltnLen:
				t = sc.getType("int")
			case bltnAppend, bltnMake:
				t, err = nodeType2(interp, sc, n.child[1], seen)
			case bltnNew:
				t, err = nodeType2(interp, sc, n.child[1], seen)
				incomplete := t.incomplete
				t = ptrOf(t, withScope(sc))
				t.incomplete = incomplete
			case bltnRecover:
				t = sc.getType("interface{}")
			default:
				t = &itype{cat: builtinT}
			}
			if err != nil {
				return nil, err
			}
		} else {
			if t, err = nodeType2(interp, sc, n.child[0], seen); err != nil || t == nil {
				return nil, err
			}
			switch t.cat {
			case valueT:
				if rt := t.rtype; rt.Kind() == reflect.Func && rt.NumOut() == 1 {
					t = valueTOf(rt.Out(0), withScope(sc))
				}
			default:
				if len(t.ret) == 1 {
					t = t.ret[0]
				}
			}
		}

	case compositeLitExpr:
		t, err = nodeType2(interp, sc, n.child[0], seen)

	case chanType, chanTypeRecv, chanTypeSend:
		dir := chanSendRecv
		switch n.kind {
		case chanTypeRecv:
			dir = chanRecv
		case chanTypeSend:
			dir = chanSend
		}
		val, err := nodeType2(interp, sc, n.child[0], seen)
		if err != nil {
			return nil, err
		}
		t = chanOf(val, dir, withNode(n), withScope(sc))
		t.incomplete = val.incomplete

	case ellipsisExpr:
		val, err := nodeType2(interp, sc, n.child[0], seen)
		if err != nil {
			return nil, err
		}
		t = variadicOf(val, withNode(n), withScope(sc))
		t.incomplete = t.val.incomplete

	case funcLit:
		t, err = nodeType2(interp, sc, n.child[2], seen)

	case funcType:
		var incomplete bool

		// Handle type parameters.
		for _, arg := range n.child[0].child {
			cl := len(arg.child) - 1
			typ, err := nodeType2(interp, sc, arg.child[cl], seen)
			if err != nil {
				return nil, err
			}
			for _, c := range arg.child[:cl] {
				sc.sym[c.ident] = &symbol{index: -1, kind: varTypeSym, typ: typ}
			}
			incomplete = incomplete || typ.incomplete
		}

		// Handle input parameters.
		args := make([]*itype, 0, len(n.child[1].child))
		for _, arg := range n.child[1].child {
			cl := len(arg.child) - 1
			typ, err := nodeType2(interp, sc, arg.child[cl], seen)
			if err != nil {
				return nil, err
			}
			args = append(args, typ)
			// Several arguments may be factorized on the same field type.
			for i := 1; i < cl; i++ {
				args = append(args, typ)
			}
			incomplete = incomplete || typ.incomplete
		}

		// Handle returned values.
		var rets []*itype
		if len(n.child) == 3 {
			for _, ret := range n.child[2].child {
				cl := len(ret.child) - 1
				typ, err := nodeType2(interp, sc, ret.child[cl], seen)
				if err != nil {
					return nil, err
				}
				rets = append(rets, typ)
				// Several arguments may be factorized on the same field type.
				for i := 1; i < cl; i++ {
					rets = append(rets, typ)
				}
				incomplete = incomplete || typ.incomplete
			}
		}
		t = funcOf(args, rets, withNode(n), withScope(sc))
		t.incomplete = incomplete

	case identExpr:
		sym, _, found := sc.lookup(n.ident)
		if !found {
			// retry with the filename, in case ident is a package name.
			baseName := filepath.Base(interp.fset.Position(n.pos).Filename)
			ident := filepath.Join(n.ident, baseName)
			sym, _, found = sc.lookup(ident)
			if !found {
				t = &itype{name: n.ident, path: sc.pkgName, node: n, incomplete: true, scope: sc}
				sc.sym[n.ident] = &symbol{kind: typeSym, typ: t}
				break
			}
		}
		if sym.kind == varTypeSym {
			t = genericOf(sym.typ, n.ident, sc.pkgName, withNode(n), withScope(sc))
		} else {
			t = sym.typ
		}
		if t == nil {
			if t, err = nodeType2(interp, sc, sym.node, seen); err != nil {
				return nil, err
			}
		}
		if t.incomplete && t.cat == linkedT && t.val != nil && t.val.cat != nilT {
			t.incomplete = false
		}
		if t.incomplete && t.node != n {
			m := t.method
			if t, err = nodeType2(interp, sc, t.node, seen); err != nil {
				return nil, err
			}
			t.method = m
			sym.typ = t
		}
		if t.node == nil {
			t.node = n
		}

	case indexExpr:
		var lt *itype
		if lt, err = nodeType2(interp, sc, n.child[0], seen); err != nil {
			return nil, err
		}
		if lt.incomplete {
			if t == nil {
				t = lt
			} else {
				t.incomplete = true
			}
			break
		}
		switch lt.cat {
		case arrayT, mapT, sliceT, variadicT:
			t = lt.val
		case genericT:
			t1, err := nodeType2(interp, sc, n.child[1], seen)
			if err != nil {
				return nil, err
			}
			if t1.cat == genericT || t1.incomplete {
				t = lt
				break
			}
			name := lt.id() + "[" + t1.id() + "]"
			if sym, _, found := sc.lookup(name); found {
				t = sym.typ
				break
			}
			// A generic type is being instantiated. Generate it.
			t, err = genType(interp, sc, name, lt, []*itype{t1}, seen)
			if err != nil {
				return nil, err
			}
		}

	case indexListExpr:
		// Similar to above indexExpr for generic types, but handle multiple type parameters.
		var lt *itype
		if lt, err = nodeType2(interp, sc, n.child[0], seen); err != nil {
			return nil, err
		}
		if lt.incomplete {
			if t == nil {
				t = lt
			} else {
				t.incomplete = true
			}
			break
		}

		// Index list expressions can be used only in context of generic types.
		if lt.cat != genericT {
			err = n.cfgErrorf("not a generic type: %s", lt.id())
			return nil, err
		}
		name := lt.id() + "["
		out := false
		types := []*itype{}
		for _, c := range n.child[1:] {
			t1, err := nodeType2(interp, sc, c, seen)
			if err != nil {
				return nil, err
			}
			if t1.cat == genericT || t1.incomplete {
				t = lt
				out = true
				break
			}
			types = append(types, t1)
			name += t1.id() + ","
		}
		if out {
			break
		}
		name = strings.TrimSuffix(name, ",") + "]"
		if sym, _, found := sc.lookup(name); found {
			t = sym.typ
			break
		}
		// A generic type is being instantiated. Generate it.
		t, err = genType(interp, sc, name, lt, types, seen)

	case interfaceType:
		if sname := typeName(n); sname != "" {
			if sym, _, found := sc.lookup(sname); found && sym.kind == typeSym {
				t = interfaceOf(sym.typ, sym.typ.field, sym.typ.constraint, sym.typ.ulconstraint, withNode(n), withScope(sc))
			}
		}
		var incomplete bool
		fields := []structField{}
		constraint := []*itype{}
		ulconstraint := []*itype{}
		for _, c := range n.child[0].child {
			c0 := c.child[0]
			if len(c.child) == 1 {
				if c0.ident == "error" {
					// Unwrap error interface inplace rather than embedding it, because
					// "error" is lower case which may cause problems with reflect for method lookup.
					typ := errorMethodType(sc)
					fields = append(fields, structField{name: "Error", typ: typ})
					continue
				}
				typ, err := nodeType2(interp, sc, c0, seen)
				if err != nil {
					return nil, err
				}
				incomplete = incomplete || typ.incomplete
				if typ.cat == constraintT {
					constraint = append(constraint, typ.constraint...)
					ulconstraint = append(ulconstraint, typ.ulconstraint...)
					continue
				}
				fields = append(fields, structField{name: fieldName(c0), embed: true, typ: typ})
				continue
			}
			typ, err := nodeType2(interp, sc, c.child[1], seen)
			if err != nil {
				return nil, err
			}
			fields = append(fields, structField{name: c0.ident, typ: typ})
			incomplete = incomplete || typ.incomplete
		}
		t = interfaceOf(t, fields, constraint, ulconstraint, withNode(n), withScope(sc))
		t.incomplete = incomplete

	case landExpr, lorExpr:
		t = sc.getType("bool")

	case mapType:
		key, err := nodeType2(interp, sc, n.child[0], seen)
		if err != nil {
			return nil, err
		}
		val, err := nodeType2(interp, sc, n.child[1], seen)
		if err != nil {
			return nil, err
		}
		t = mapOf(key, val, withNode(n), withScope(sc))
		t.incomplete = key.incomplete || val.incomplete

	case parenExpr:
		t, err = nodeType2(interp, sc, n.child[0], seen)

	case selectorExpr:
		// Resolve the left part of selector, then lookup the right part on it
		var lt *itype

		// Lookup the package symbol first if we are in a field expression as
		// a previous parameter has the same name as the package, we need to
		// prioritize the package type.
		if n.anc.kind == fieldExpr {
			lt = findPackageType(interp, sc, n.child[0])
		}
		if lt == nil {
			// No package was found or we are not in a field expression, we are looking for a variable.
			if lt, err = nodeType2(interp, sc, n.child[0], seen); err != nil {
				return nil, err
			}
		}

		if lt.incomplete {
			break
		}
		name := n.child[1].ident
		switch lt.cat {
		case binPkgT:
			pkg := interp.binPkg[lt.path]
			if v, ok := pkg[name]; ok {
				rtype := v.Type()
				if isBinType(v) {
					// A bin type is encoded as a pointer on a typed nil value.
					rtype = rtype.Elem()
				}
				t = valueTOf(rtype, withNode(n), withScope(sc))
			} else {
				err = n.cfgErrorf("undefined selector %s.%s", lt.path, name)
			}
		case srcPkgT:
			pkg := interp.srcPkg[lt.path]
			if s, ok := pkg[name]; ok {
				t = s.typ
			} else {
				err = n.cfgErrorf("undefined selector %s.%s", lt.path, name)
			}
		default:
			if m, _ := lt.lookupMethod(name); m != nil {
				t, err = nodeType2(interp, sc, m.child[2], seen)
			} else if bm, _, _, ok := lt.lookupBinMethod(name); ok {
				t = valueTOf(bm.Type, isBinMethod(), withRecv(lt), withScope(sc))
			} else if ti := lt.lookupField(name); len(ti) > 0 {
				t = lt.fieldSeq(ti)
			} else if bs, _, ok := lt.lookupBinField(name); ok {
				t = valueTOf(bs.Type, withScope(sc))
			} else {
				err = lt.node.cfgErrorf("undefined selector %s", name)
			}
		}

	case sliceExpr:
		t, err = nodeType2(interp, sc, n.child[0], seen)
		if err != nil {
			return nil, err
		}

		if t.cat == valueT {
			switch t.rtype.Kind() {
			case reflect.Array, reflect.Ptr:
				t = valueTOf(reflect.SliceOf(t.rtype.Elem()), withScope(sc))
			}
			break
		}
		if t.cat == ptrT {
			t = t.val
		}
		if t.cat == arrayT {
			incomplete := t.incomplete
			t = sliceOf(t.val, withNode(n), withScope(sc))
			t.incomplete = incomplete
		}

	case structType:
		var sym *symbol
		var found bool
		sname := structName(n)
		if sname != "" {
			sym, _, found = sc.lookup(sname)
			if found && sym.kind == typeSym && sym.typ != nil {
				t = structOf(sym.typ, sym.typ.field, withNode(n), withScope(sc))
			} else {
				t = structOf(nil, nil, withNode(n), withScope(sc))
				sc.sym[sname] = &symbol{index: -1, kind: typeSym, typ: t, node: n}
			}
		}
		var incomplete bool
		fields := make([]structField, 0, len(n.child[0].child))
		for _, c := range n.child[0].child {
			switch {
			case len(c.child) == 1:
				typ, err := nodeType2(interp, sc, c.child[0], seen)
				if err != nil {
					return nil, err
				}
				fields = append(fields, structField{name: fieldName(c.child[0]), embed: true, typ: typ})
				incomplete = incomplete || typ.incomplete
			case len(c.child) == 2 && c.child[1].kind == basicLit:
				tag := vString(c.child[1].rval)
				typ, err := nodeType2(interp, sc, c.child[0], seen)
				if err != nil {
					return nil, err
				}
				fields = append(fields, structField{name: fieldName(c.child[0]), embed: true, typ: typ, tag: tag})
				incomplete = incomplete || typ.incomplete
			default:
				var tag string
				l := len(c.child)
				if c.lastChild().kind == basicLit {
					tag = vString(c.lastChild().rval)
					l--
				}
				typ, err := nodeType2(interp, sc, c.child[l-1], seen)
				if err != nil {
					return nil, err
				}
				incomplete = incomplete || typ.incomplete
				for _, d := range c.child[:l-1] {
					fields = append(fields, structField{name: d.ident, typ: typ, tag: tag})
				}
			}
		}
		t = structOf(t, fields, withNode(n), withScope(sc))
		t.incomplete = incomplete
		if sname != "" {
			if sc.sym[sname] == nil {
				sc.sym[sname] = &symbol{index: -1, kind: typeSym, node: n}
			}
			sc.sym[sname].typ = t
		}

	default:
		err = n.cfgErrorf("type definition not implemented: %s", n.kind)
	}

	if err == nil && t != nil && t.cat == nilT && !t.incomplete {
		err = n.cfgErrorf("use of untyped nil %s", t.name)
	}

	// The existing symbol data needs to be recovered, but not in the
	// case where we are aliasing another type.
	if n.anc.kind == typeSpec && n.kind != selectorExpr && n.kind != identExpr {
		name := n.anc.child[0].ident
		if sym := sc.sym[name]; sym != nil {
			t.path = sc.pkgName
			t.name = name
		}
	}

	switch {
	case t == nil:
	case t.name != "" && t.path != "":
		t.str = t.path + "." + t.name
	case t.cat == nilT:
		t.str = "nil"
	}

	return t, err
}

func genType(interp *Interpreter, sc *scope, name string, lt *itype, types []*itype, seen []*node) (t *itype, err error) {
	// A generic type is being instantiated. Generate it.
	g, _, err := genAST(sc, lt.node.anc, types)
	if err != nil {
		return nil, err
	}
	t, err = nodeType2(interp, sc, g.lastChild(), seen)
	if err != nil {
		return nil, err
	}
	lt.instance = append(lt.instance, t)
	// Add generated symbol in the scope of generic source and user.
	sc.sym[name] = &symbol{index: -1, kind: typeSym, typ: t, node: g}
	if lt.scope.sym[name] == nil {
		lt.scope.sym[name] = sc.sym[name]
	}

	for _, nod := range lt.method {
		if err := genMethod(interp, sc, t, nod, types); err != nil {
			return nil, err
		}
	}
	return t, err
}

func genMethod(interp *Interpreter, sc *scope, t *itype, nod *node, types []*itype) error {
	gm, _, err := genAST(sc, nod, types)
	if err != nil {
		return err
	}
	if gm.typ, err = nodeType(interp, sc, gm.child[2]); err != nil {
		return err
	}
	t.addMethod(gm)

	// If the receiver is a pointer to a generic type, generate also the pointer type.
	if rtn := gm.child[0].child[0].lastChild(); rtn != nil && rtn.kind == starExpr {
		pt := ptrOf(t, withNode(t.node), withScope(sc))
		pt.addMethod(gm)
		rtn.typ = pt
	}

	// Compile the method AST in the scope of the generic type.
	scop := nod.typ.scope
	if _, err = interp.cfg(gm, scop, scop.pkgID, scop.pkgName); err != nil {
		return err
	}

	// Generate closures for function body.
	return genRun(gm)
}

// findPackageType searches the top level scope for a package type.
func findPackageType(interp *Interpreter, sc *scope, n *node) *itype {
	// Find the root scope, the package symbols will exist there.
	for {
		if sc.level == 0 {
			break
		}
		sc = sc.anc
	}

	baseName := filepath.Base(interp.fset.Position(n.pos).Filename)
	sym, _, found := sc.lookup(filepath.Join(n.ident, baseName))
	if !found || sym.typ == nil && sym.typ.cat != srcPkgT && sym.typ.cat != binPkgT {
		return nil
	}
	return sym.typ
}

func isBuiltinCall(n *node, sc *scope) bool {
	if n.kind != callExpr {
		return false
	}
	s := n.child[0].sym
	if s == nil {
		if sym, _, found := sc.lookup(n.child[0].ident); found {
			s = sym
		}
	}
	return s != nil && s.kind == bltnSym
}

// struct name returns the name of a struct type.
func typeName(n *node) string {
	if n.anc.kind == typeSpec && len(n.anc.child) == 2 {
		return n.anc.child[0].ident
	}
	return ""
}

func structName(n *node) string {
	if n.anc.kind == typeSpec {
		return n.anc.child[0].ident
	}
	return ""
}

// fieldName returns an implicit struct field name according to node kind.
func fieldName(n *node) string {
	switch n.kind {
	case selectorExpr:
		return fieldName(n.child[1])
	case starExpr:
		return fieldName(n.child[0])
	case identExpr:
		return n.ident
	default:
		return ""
	}
}

var zeroValues [maxT]reflect.Value

func init() {
	zeroValues[boolT] = reflect.ValueOf(false)
	zeroValues[complex64T] = reflect.ValueOf(complex64(0))
	zeroValues[complex128T] = reflect.ValueOf(complex128(0))
	zeroValues[errorT] = reflect.ValueOf(new(error)).Elem()
	zeroValues[float32T] = reflect.ValueOf(float32(0))
	zeroValues[float64T] = reflect.ValueOf(float64(0))
	zeroValues[intT] = reflect.ValueOf(int(0))
	zeroValues[int8T] = reflect.ValueOf(int8(0))
	zeroValues[int16T] = reflect.ValueOf(int16(0))
	zeroValues[int32T] = reflect.ValueOf(int32(0))
	zeroValues[int64T] = reflect.ValueOf(int64(0))
	zeroValues[stringT] = reflect.ValueOf("")
	zeroValues[uintT] = reflect.ValueOf(uint(0))
	zeroValues[uint8T] = reflect.ValueOf(uint8(0))
	zeroValues[uint16T] = reflect.ValueOf(uint16(0))
	zeroValues[uint32T] = reflect.ValueOf(uint32(0))
	zeroValues[uint64T] = reflect.ValueOf(uint64(0))
	zeroValues[uintptrT] = reflect.ValueOf(uintptr(0))
}

// Finalize returns a type pointer and error. It reparses a type from the
// partial AST if necessary (after missing dependecy data is available).
// If error is nil, the type is guarranteed to be completely defined and
// usable for CFG.
func (t *itype) finalize() (*itype, error) {
	var err error
	if t.incomplete {
		sym, _, found := t.scope.lookup(t.name)
		if found && !sym.typ.incomplete {
			sym.typ.method = append(sym.typ.method, t.method...)
			t.method = sym.typ.method
			t.incomplete = false
			return sym.typ, nil
		}
		m := t.method
		if t, err = nodeType(t.node.interp, t.scope, t.node); err != nil {
			return nil, err
		}
		if t.incomplete {
			return nil, t.node.cfgErrorf("incomplete type %s", t.name)
		}
		t.method = m
		t.node.typ = t
		if sym != nil {
			sym.typ = t
		}
	}
	return t, err
}

func (t *itype) addMethod(n *node) {
	for _, m := range t.method {
		if m == n {
			return
		}
	}
	t.method = append(t.method, n)
}

func (t *itype) numIn() int {
	switch t.cat {
	case funcT:
		return len(t.arg)
	case valueT:
		if t.rtype.Kind() != reflect.Func {
			return 0
		}
		in := t.rtype.NumIn()
		if t.recv != nil {
			in--
		}
		return in
	}
	return 0
}

func (t *itype) in(i int) *itype {
	switch t.cat {
	case funcT:
		return t.arg[i]
	case valueT:
		if t.rtype.Kind() == reflect.Func {
			if t.recv != nil && !isInterface(t.recv) {
				i++
			}
			if t.rtype.IsVariadic() && i == t.rtype.NumIn()-1 {
				val := valueTOf(t.rtype.In(i).Elem())
				return &itype{cat: variadicT, val: val, str: "..." + val.str}
			}
			return valueTOf(t.rtype.In(i))
		}
	}
	return nil
}

func (t *itype) numOut() int {
	switch t.cat {
	case funcT:
		return len(t.ret)
	case valueT:
		if t.rtype.Kind() == reflect.Func {
			return t.rtype.NumOut()
		}
	case builtinT:
		switch t.name {
		case "append", "cap", "complex", "copy", "imag", "len", "make", "new", "real", "recover":
			return 1
		}
	}
	return 0
}

func (t *itype) out(i int) *itype {
	switch t.cat {
	case funcT:
		return t.ret[i]
	case valueT:
		if t.rtype.Kind() == reflect.Func {
			return valueTOf(t.rtype.Out(i))
		}
	}
	return nil
}

func (t *itype) concrete() *itype {
	if isInterface(t) && t.val != nil {
		return t.val.concrete()
	}
	return t
}

func (t *itype) underlying() *itype {
	if t.cat == linkedT {
		return t.val.underlying()
	}
	return t
}

// typeDefined returns true if type t1 is defined from type t2 or t2 from t1.
func typeDefined(t1, t2 *itype) bool {
	if t1.cat == linkedT && t1.val == t2 {
		return true
	}
	if t2.cat == linkedT && t2.val == t1 {
		return true
	}
	return false
}

// isVariadic returns true if the function type is variadic.
// If the type is not a function or is not variadic, it will
// return false.
func (t *itype) isVariadic() bool {
	switch t.cat {
	case funcT:
		return len(t.arg) > 0 && t.arg[len(t.arg)-1].cat == variadicT
	case valueT:
		if t.rtype.Kind() == reflect.Func {
			return t.rtype.IsVariadic()
		}
	}
	return false
}

// isComplete returns true if type definition is complete.
func (t *itype) isComplete() bool { return isComplete(t, map[string]bool{}) }

func isComplete(t *itype, visited map[string]bool) bool {
	if t.incomplete {
		return false
	}
	name := t.path + "/" + t.name
	if visited[name] {
		return true
	}
	if t.name != "" {
		visited[name] = true
	}
	switch t.cat {
	case linkedT:
		if t.val != nil && t.val.cat != nilT {
			// A type aliased to a partially defined type is considered complete, to allow recursivity.
			return true
		}
		fallthrough
	case arrayT, chanT, chanRecvT, chanSendT, ptrT, sliceT, variadicT:
		return isComplete(t.val, visited)
	case funcT:
		complete := true
		for _, a := range t.arg {
			complete = complete && isComplete(a, visited)
		}
		for _, a := range t.ret {
			complete = complete && isComplete(a, visited)
		}
		return complete
	case interfaceT, structT:
		complete := true
		for _, f := range t.field {
			// Field implicit type names must be marked as visited, to break false circles.
			visited[f.typ.path+"/"+f.typ.name] = true
			complete = complete && isComplete(f.typ, visited)
		}
		return complete
	case mapT:
		return isComplete(t.key, visited) && isComplete(t.val, visited)
	case nilT:
		return false
	}
	return true
}

// comparable returns true if the type is comparable.
func (t *itype) comparable() bool {
	typ := t.TypeOf()
	return t.cat == nilT || typ != nil && typ.Comparable()
}

func (t *itype) assignableTo(o *itype) bool {
	if t.equals(o) {
		return true
	}

	if t.cat == linkedT && o.cat == linkedT && (t.underlying().id() != o.underlying().id() || !typeDefined(t, o)) {
		return false
	}

	if t.isNil() && o.hasNil() || o.isNil() && t.hasNil() {
		return true
	}

	if t.TypeOf().AssignableTo(o.TypeOf()) {
		return true
	}

	if isInterface(o) && t.implements(o) {
		return true
	}

	if t.cat == sliceT && o.cat == sliceT {
		return t.val.assignableTo(o.val)
	}

	if t.isBinMethod && isFunc(o) {
		// TODO (marc): check that t without receiver as first parameter is equivalent to o.
		return true
	}

	if t.untyped && isNumber(t.TypeOf()) && isNumber(o.TypeOf()) {
		// Assignability depends on constant numeric value (overflow check), to be tested elsewhere.
		return true
	}

	n := t.node
	if n == nil || !n.rval.IsValid() {
		return false
	}
	con, ok := n.rval.Interface().(constant.Value)
	if !ok {
		return false
	}
	if con == nil || !isConstType(o) {
		return false
	}
	return representableConst(con, o.TypeOf())
}

// convertibleTo returns true if t is convertible to o.
func (t *itype) convertibleTo(o *itype) bool {
	if t.assignableTo(o) {
		return true
	}

	// unsafe checks
	tt, ot := t.TypeOf(), o.TypeOf()
	if (tt.Kind() == reflect.Ptr || tt.Kind() == reflect.Uintptr) && ot.Kind() == reflect.UnsafePointer {
		return true
	}
	if tt.Kind() == reflect.UnsafePointer && (ot.Kind() == reflect.Ptr || ot.Kind() == reflect.Uintptr) {
		return true
	}

	return t.TypeOf().ConvertibleTo(o.TypeOf())
}

// ordered returns true if the type is ordered.
func (t *itype) ordered() bool {
	typ := t.TypeOf()
	return isInt(typ) || isFloat(typ) || isString(typ)
}

// Equals returns true if the given type is identical to the receiver one.
func (t *itype) equals(o *itype) bool {
	switch ti, oi := isInterface(t), isInterface(o); {
	case ti && oi:
		return t.methods().equals(o.methods())
	case ti && !oi:
		return o.methods().contains(t.methods())
	case oi && !ti:
		return t.methods().contains(o.methods())
	default:
		return t.id() == o.id()
	}
}

// MethodSet defines the set of methods signatures as strings, indexed per method name.
type methodSet map[string]string

// Contains returns true if the method set m contains the method set n.
func (m methodSet) contains(n methodSet) bool {
	for k, v := range n {
		if m[k] != v {
			return false
		}
	}
	return true
}

// Equal returns true if the method set m is equal to the method set n.
func (m methodSet) equals(n methodSet) bool {
	return m.contains(n) && n.contains(m)
}

// Methods returns a map of method type strings, indexed by method names.
func (t *itype) methods() methodSet {
	seen := map[*itype]bool{}
	var getMethods func(typ *itype) methodSet

	getMethods = func(typ *itype) methodSet {
		res := make(methodSet)

		if seen[typ] {
			// Stop the recursion, we have seen this type.
			return res
		}
		seen[typ] = true

		switch typ.cat {
		case linkedT:
			for k, v := range getMethods(typ.val) {
				res[k] = v
			}
		case interfaceT:
			// Get methods from recursive analysis of interface fields.
			for _, f := range typ.field {
				if f.typ.cat == funcT {
					res[f.name] = f.typ.TypeOf().String()
				} else {
					for k, v := range getMethods(f.typ) {
						res[k] = v
					}
				}
			}
		case valueT, errorT:
			// Get method from corresponding reflect.Type.
			for i := typ.TypeOf().NumMethod() - 1; i >= 0; i-- {
				m := typ.rtype.Method(i)
				res[m.Name] = m.Type.String()
			}
		case ptrT:
			if typ.val.cat == valueT {
				// Ptr receiver methods need to be found with the ptr type.
				typ.TypeOf() // Ensure the rtype exists.
				for i := typ.rtype.NumMethod() - 1; i >= 0; i-- {
					m := typ.rtype.Method(i)
					res[m.Name] = m.Type.String()
				}
			}
			for k, v := range getMethods(typ.val) {
				res[k] = v
			}
		case structT:
			for _, f := range typ.field {
				if !f.embed {
					continue
				}
				for k, v := range getMethods(f.typ) {
					res[k] = v
				}
			}
		}
		// Get all methods defined on this type.
		for _, m := range typ.method {
			res[m.ident] = m.typ.TypeOf().String()
		}
		return res
	}

	return getMethods(t)
}

// id returns a unique type identificator string.
func (t *itype) id() (res string) {
	// Prefer the wrapped type string over the rtype string.
	if t.cat == valueT && t.val != nil {
		return t.val.str
	}
	return t.str
}

// fixPossibleConstType returns the input type if it not a constant value,
// otherwise, it returns the default Go type corresponding to the
// constant.Value.
func fixPossibleConstType(t reflect.Type) (r reflect.Type) {
	cv, ok := reflect.New(t).Elem().Interface().(constant.Value)
	if !ok {
		return t
	}
	switch cv.Kind() {
	case constant.Bool:
		r = reflect.TypeOf(true)
	case constant.Int:
		r = reflect.TypeOf(0)
	case constant.String:
		r = reflect.TypeOf("")
	case constant.Float:
		r = reflect.TypeOf(float64(0))
	case constant.Complex:
		r = reflect.TypeOf(complex128(0))
	}
	return r
}

// zero instantiates and return a zero value object for the given type during execution.
func (t *itype) zero() (v reflect.Value, err error) {
	if t, err = t.finalize(); err != nil {
		return v, err
	}
	switch t.cat {
	case linkedT:
		v, err = t.val.zero()

	case arrayT, ptrT, structT, sliceT:
		v = reflect.New(t.frameType()).Elem()

	case valueT:
		v = reflect.New(t.rtype).Elem()

	default:
		v = zeroValues[t.cat]
	}
	return v, err
}

// fieldIndex returns the field index from name in a struct, or -1 if not found.
func (t *itype) fieldIndex(name string) int {
	switch t.cat {
	case linkedT, ptrT:
		return t.val.fieldIndex(name)
	}
	for i, field := range t.field {
		if name == field.name {
			return i
		}
	}
	return -1
}

// fieldSeq returns the field type from the list of field indexes.
func (t *itype) fieldSeq(seq []int) *itype {
	ft := t
	for _, i := range seq {
		if ft.cat == ptrT {
			ft = ft.val
		}
		ft = ft.field[i].typ
	}
	return ft
}

// lookupField returns a list of indices, i.e. a path to access a field in a struct object.
func (t *itype) lookupField(name string) []int {
	seen := map[*itype]bool{}
	var lookup func(*itype) []int
	tias := isStruct(t)

	lookup = func(typ *itype) []int {
		if seen[typ] {
			return nil
		}
		seen[typ] = true

		switch typ.cat {
		case linkedT, ptrT:
			return lookup(typ.val)
		}
		if fi := typ.fieldIndex(name); fi >= 0 {
			return []int{fi}
		}

		for i, f := range typ.field {
			switch f.typ.cat {
			case ptrT, structT, interfaceT, linkedT:
				if tias != isStruct(f.typ) {
					// Interface fields are not valid embedded struct fields.
					// Struct fields are not valid interface fields.
					break
				}
				if index2 := lookup(f.typ); len(index2) > 0 {
					return append([]int{i}, index2...)
				}
			}
		}

		return nil
	}

	return lookup(t)
}

// lookupBinField returns a structfield and a path to access an embedded binary field in a struct object.
func (t *itype) lookupBinField(name string) (s reflect.StructField, index []int, ok bool) {
	if t.cat == ptrT {
		return t.val.lookupBinField(name)
	}
	if !isStruct(t) {
		return
	}
	rt := t.TypeOf()
	for t.cat == valueT && rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return
	}
	s, ok = rt.FieldByName(name)
	if !ok {
		for i, f := range t.field {
			if f.embed {
				if s2, index2, ok2 := f.typ.lookupBinField(name); ok2 {
					index = append([]int{i}, index2...)
					return s2, index, ok2
				}
			}
		}
	}
	return s, index, ok
}

// MethodCallType returns a method function type without the receiver defined.
// The input type must be a method function type with the receiver as the first input argument.
func (t *itype) methodCallType() reflect.Type {
	it := []reflect.Type{}
	ni := t.rtype.NumIn()
	for i := 1; i < ni; i++ {
		it = append(it, t.rtype.In(i))
	}
	ot := []reflect.Type{}
	no := t.rtype.NumOut()
	for i := 0; i < no; i++ {
		ot = append(ot, t.rtype.Out(i))
	}
	return reflect.FuncOf(it, ot, t.rtype.IsVariadic())
}

func (t *itype) resolveAlias() *itype {
	for t.cat == linkedT {
		t = t.val
	}
	return t
}

// GetMethod returns a pointer to the method definition.
func (t *itype) getMethod(name string) *node {
	for _, m := range t.method {
		if name == m.ident {
			return m
		}
	}
	return nil
}

// LookupMethod returns a pointer to method definition associated to type t
// and the list of indices to access the right struct field, in case of an embedded method.
func (t *itype) lookupMethod(name string) (*node, []int) {
	return t.lookupMethod2(name, nil)
}

func (t *itype) lookupMethod2(name string, seen map[*itype]bool) (*node, []int) {
	if seen == nil {
		seen = map[*itype]bool{}
	}
	if seen[t] {
		return nil, nil
	}
	seen[t] = true
	if t.cat == ptrT {
		return t.val.lookupMethod2(name, seen)
	}
	var index []int
	m := t.getMethod(name)
	if m == nil {
		for i, f := range t.field {
			if f.embed {
				if n, index2 := f.typ.lookupMethod2(name, seen); n != nil {
					index = append([]int{i}, index2...)
					return n, index
				}
			}
		}
		if t.cat == linkedT || isInterfaceSrc(t) && t.val != nil {
			return t.val.lookupMethod2(name, seen)
		}
	}
	return m, index
}

// interfaceMethod returns type of method matching an interface method name (not as a concrete method).
func (t *itype) interfaceMethod(name string) *itype {
	return t.interfaceMethod2(name, nil)
}

func (t *itype) interfaceMethod2(name string, seen map[*itype]bool) *itype {
	if seen == nil {
		seen = map[*itype]bool{}
	}
	if seen[t] {
		return nil
	}
	seen[t] = true
	if t.cat == ptrT {
		return t.val.interfaceMethod2(name, seen)
	}
	for _, f := range t.field {
		if f.name == name && isInterface(t) {
			return f.typ
		}
		if !f.embed {
			continue
		}
		if typ := f.typ.interfaceMethod2(name, seen); typ != nil {
			return typ
		}
	}
	if t.cat == linkedT || isInterfaceSrc(t) && t.val != nil {
		return t.val.interfaceMethod2(name, seen)
	}
	return nil
}

// methodDepth returns a depth greater or equal to 0, or -1 if no match.
func (t *itype) methodDepth(name string) int {
	if m, lint := t.lookupMethod(name); m != nil {
		return len(lint)
	}
	if _, lint, _, ok := t.lookupBinMethod(name); ok {
		return len(lint)
	}
	return -1
}

// LookupBinMethod returns a method and a path to access a field in a struct object (the receiver).
func (t *itype) lookupBinMethod(name string) (m reflect.Method, index []int, isPtr, ok bool) {
	return t.lookupBinMethod2(name, nil)
}

func (t *itype) lookupBinMethod2(name string, seen map[*itype]bool) (m reflect.Method, index []int, isPtr, ok bool) {
	if seen == nil {
		seen = map[*itype]bool{}
	}
	if seen[t] {
		return
	}
	seen[t] = true
	if t.cat == ptrT {
		return t.val.lookupBinMethod2(name, seen)
	}
	for i, f := range t.field {
		if f.embed {
			if m2, index2, isPtr2, ok2 := f.typ.lookupBinMethod2(name, seen); ok2 {
				index = append([]int{i}, index2...)
				return m2, index, isPtr2, ok2
			}
		}
	}
	m, ok = t.TypeOf().MethodByName(name)
	if !ok {
		m, ok = reflect.PtrTo(t.TypeOf()).MethodByName(name)
		isPtr = ok
	}
	return m, index, isPtr, ok
}

func lookupFieldOrMethod(t *itype, name string) *itype {
	switch {
	case t.cat == valueT || t.cat == ptrT && t.val.cat == valueT:
		m, _, isPtr, ok := t.lookupBinMethod(name)
		if !ok {
			return nil
		}
		var recv *itype
		if t.rtype.Kind() != reflect.Interface {
			recv = t
			if isPtr && t.cat != ptrT && t.rtype.Kind() != reflect.Ptr {
				recv = ptrOf(t)
			}
		}
		return valueTOf(m.Type, withRecv(recv))
	case t.cat == interfaceT:
		seq := t.lookupField(name)
		if seq == nil {
			return nil
		}
		return t.fieldSeq(seq)
	default:
		n, _ := t.lookupMethod(name)
		if n == nil {
			return nil
		}
		return n.typ
	}
}

func exportName(s string) string {
	if canExport(s) {
		return s
	}
	return "X" + s
}

var (
	// TODO(mpl): generators.
	emptyInterfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	valueInterfaceType = reflect.TypeOf((*valueInterface)(nil)).Elem()
	constVal           = reflect.TypeOf((*constant.Value)(nil)).Elem()
)

type refTypeContext struct {
	defined map[string]*itype

	// refs keeps track of all the places (in the same type recursion) where the
	// type name (as key) is used as a field of another (or possibly the same) struct
	// type. Each of these fields will then live as an unsafe2.dummy type until the
	// whole recursion is fully resolved, and the type is fixed.
	refs map[string][]*itype

	// When we detect for the first time that we are in a recursive type (thanks to
	// defined), we keep track of the first occurrence of the type where the recursion
	// started, so we can restart the last step that fixes all the types from the same
	// "top-level" point.
	rect       *itype
	rebuilding bool
	slevel     int
}

// Clone creates a copy of the ref type context.
func (c *refTypeContext) Clone() *refTypeContext {
	return &refTypeContext{defined: c.defined, refs: c.refs, rebuilding: c.rebuilding}
}

func (c *refTypeContext) isComplete() bool {
	for _, t := range c.defined {
		if t.rtype == nil {
			return false
		}
	}
	return true
}

func (t *itype) fixDummy(typ reflect.Type) reflect.Type {
	if typ == unsafe2.DummyType {
		return t.rtype
	}
	switch typ.Kind() {
	case reflect.Array:
		return reflect.ArrayOf(typ.Len(), t.fixDummy(typ.Elem()))
	case reflect.Chan:
		return reflect.ChanOf(typ.ChanDir(), t.fixDummy(typ.Elem()))
	case reflect.Func:
		in := make([]reflect.Type, typ.NumIn())
		for i := range in {
			in[i] = t.fixDummy(typ.In(i))
		}
		out := make([]reflect.Type, typ.NumOut())
		for i := range out {
			out[i] = t.fixDummy(typ.Out(i))
		}
		return reflect.FuncOf(in, out, typ.IsVariadic())
	case reflect.Map:
		return reflect.MapOf(t.fixDummy(typ.Key()), t.fixDummy(typ.Elem()))
	case reflect.Ptr:
		return reflect.PtrTo(t.fixDummy(typ.Elem()))
	case reflect.Slice:
		return reflect.SliceOf(t.fixDummy(typ.Elem()))
	case reflect.Struct:
		fields := make([]reflect.StructField, typ.NumField())
		for i := range fields {
			fields[i] = typ.Field(i)
			fields[i].Type = t.fixDummy(fields[i].Type)
		}
		return reflect.StructOf(fields)
	}
	return typ
}

// RefType returns a reflect.Type representation from an interpreter type.
// In simple cases, reflect types are directly mapped from the interpreter
// counterpart.
// For recursive named struct or interfaces, as reflect does not permit to
// create a recursive named struct, a dummy type is set temporarily for each recursive
// field. When done, the dummy type fields are updated with the original reflect type
// pointer using unsafe. We thus obtain a usable recursive type definition, except
// for string representation, as created reflect types are still unnamed.
func (t *itype) refType(ctx *refTypeContext) reflect.Type {
	if ctx == nil {
		ctx = &refTypeContext{
			defined: map[string]*itype{},
			refs:    map[string][]*itype{},
		}
	}
	if t.incomplete || t.cat == nilT {
		var err error
		if t, err = t.finalize(); err != nil {
			panic(err)
		}
	}
	name := t.path + "/" + t.name

	if t.rtype != nil && !ctx.rebuilding {
		return t.rtype
	}
	if dt := ctx.defined[name]; dt != nil {
		// We get here when we are a struct field, and our type name has already been
		// seen at least once in one of our englobing structs. i.e. there's at least one
		// level of type recursion.
		if dt.rtype != nil {
			t.rtype = dt.rtype
			return dt.rtype
		}

		// The recursion has not been fully resolved yet.
		// To indicate that a rebuild is needed on the englobing struct,
		// return a dummy field type and create an empty entry.
		flds := ctx.refs[name]
		ctx.rect = dt

		// We know we are used as a field by someone, but we don't know by who
		// at this point in the code, so we just mark it as an empty *itype for now.
		// We'll complete the *itype in the caller.
		ctx.refs[name] = append(flds, (*itype)(nil))
		return unsafe2.DummyType
	}
	if isGeneric(t) {
		return reflect.TypeOf((*generic)(nil)).Elem()
	}
	switch t.cat {
	case linkedT:
		t.rtype = t.val.refType(ctx)
	case arrayT:
		t.rtype = reflect.ArrayOf(t.length, t.val.refType(ctx))
	case sliceT, variadicT:
		t.rtype = reflect.SliceOf(t.val.refType(ctx))
	case chanT:
		t.rtype = reflect.ChanOf(reflect.BothDir, t.val.refType(ctx))
	case chanRecvT:
		t.rtype = reflect.ChanOf(reflect.RecvDir, t.val.refType(ctx))
	case chanSendT:
		t.rtype = reflect.ChanOf(reflect.SendDir, t.val.refType(ctx))
	case errorT:
		t.rtype = reflect.TypeOf(new(error)).Elem()
	case funcT:
		variadic := false
		in := make([]reflect.Type, len(t.arg))
		out := make([]reflect.Type, len(t.ret))
		for i, v := range t.arg {
			in[i] = v.refType(ctx)
			variadic = v.cat == variadicT
		}
		for i, v := range t.ret {
			out[i] = v.refType(ctx)
		}
		t.rtype = reflect.FuncOf(in, out, variadic)
	case interfaceT:
		if len(t.field) == 0 {
			// empty interface, do not wrap it
			t.rtype = emptyInterfaceType
			break
		}
		t.rtype = valueInterfaceType
	case mapT:
		t.rtype = reflect.MapOf(t.key.refType(ctx), t.val.refType(ctx))
	case ptrT:
		rt := t.val.refType(ctx)
		if rt == unsafe2.DummyType && ctx.slevel > 1 {
			// We have a pointer to a recursive struct which is not yet fully computed.
			// Return it but do not yet store it in rtype, so the complete version can
			// be stored in future.
			return reflect.PtrTo(rt)
		}
		t.rtype = reflect.PtrTo(rt)
	case structT:
		if t.name != "" {
			ctx.defined[name] = t
		}
		ctx.slevel++
		var fields []reflect.StructField
		for _, f := range t.field {
			field := reflect.StructField{
				Name: exportName(f.name), Type: f.typ.refType(ctx),
				Tag: reflect.StructTag(f.tag), Anonymous: f.embed,
			}
			fields = append(fields, field)
			// Find any nil type refs that indicates a rebuild is needed on this field.
			for _, flds := range ctx.refs {
				for j, fld := range flds {
					if fld == nil {
						flds[j] = t
					}
				}
			}
		}
		ctx.slevel--
		type fixStructField struct {
			name  string
			index int
		}
		fieldFix := []fixStructField{} // Slice of field indices to fix for recursivity.
		t.rtype = reflect.StructOf(fields)
		if ctx.isComplete() {
			for _, s := range ctx.defined {
				for i := 0; i < s.rtype.NumField(); i++ {
					f := s.rtype.Field(i)
					if strings.HasSuffix(f.Type.String(), "unsafe2.dummy") {
						unsafe2.SetFieldType(s.rtype, i, ctx.rect.fixDummy(s.rtype.Field(i).Type))
						if name == s.path+"/"+s.name {
							fieldFix = append(fieldFix, fixStructField{s.name, i})
						}
						continue
					}
					if f.Type.Kind() == reflect.Func && strings.Contains(f.Type.String(), "unsafe2.dummy") {
						fieldFix = append(fieldFix, fixStructField{s.name, i})
					}
				}
			}
		}

		// The rtype has now been built, we can go back and rebuild
		// all the recursive types that relied on this type.
		// However, as we are keyed by type name, if two or more (recursive) fields at
		// the same depth level are of the same type, or a "variation" of the same type
		// (slice of, map of, etc), they "mask" each other, and only one
		// of them is in ctx.refs. That is why the code around here is a bit convoluted,
		// and we need both the loop above, around all the struct fields, and the loop
		// below, around the ctx.refs.
		for _, f := range ctx.refs[name] {
			for _, ff := range fieldFix {
				if ff.name == f.name {
					ftyp := f.field[ff.index].typ.refType(&refTypeContext{defined: ctx.defined, rebuilding: true})
					unsafe2.SetFieldType(f.rtype, ff.index, ftyp)
				}
			}
		}
	default:
		if z, _ := t.zero(); z.IsValid() {
			t.rtype = z.Type()
		}
	}
	return t.rtype
}

// TypeOf returns the reflection type of dynamic interpreter type t.
func (t *itype) TypeOf() reflect.Type {
	return t.refType(nil)
}

func (t *itype) frameType() (r reflect.Type) {
	var err error
	if t, err = t.finalize(); err != nil {
		panic(err)
	}
	switch t.cat {
	case linkedT:
		r = t.val.frameType()
	case arrayT:
		r = reflect.ArrayOf(t.length, t.val.frameType())
	case sliceT, variadicT:
		r = reflect.SliceOf(t.val.frameType())
	case interfaceT:
		if len(t.field) == 0 {
			// empty interface, do not wrap it
			r = emptyInterfaceType
			break
		}
		r = valueInterfaceType
	case mapT:
		r = reflect.MapOf(t.key.frameType(), t.val.frameType())
	case ptrT:
		r = reflect.PtrTo(t.val.frameType())
	default:
		r = t.TypeOf()
	}
	return r
}

func (t *itype) implements(it *itype) bool {
	if isBin(t) {
		// Note: in case of a valueInterfaceType, we
		// miss required data which will be available
		// later, so we optimistically return true to progress,
		// and additional checks will be hopefully performed at
		// runtime.
		if rt := it.TypeOf(); rt == valueInterfaceType {
			return true
		}
		return t.TypeOf().Implements(it.TypeOf())
	}
	return t.methods().contains(it.methods())
}

// defaultType returns the default type of an untyped type.
func (t *itype) defaultType(v reflect.Value, sc *scope) *itype {
	if !t.untyped {
		return t
	}

	typ := t
	// The default type can also be derived from a constant value.
	if v.IsValid() && v.Type().Implements(constVal) {
		switch v.Interface().(constant.Value).Kind() {
		case constant.String:
			typ = sc.getType("string")
		case constant.Bool:
			typ = sc.getType("bool")
		case constant.Int:
			switch t.cat {
			case int32T:
				typ = sc.getType("int32")
			default:
				typ = sc.getType("int")
			}
		case constant.Float:
			typ = sc.getType("float64")
		case constant.Complex:
			typ = sc.getType("complex128")
		}
	}
	if typ.untyped {
		switch t.cat {
		case stringT:
			typ = sc.getType("string")
		case boolT:
			typ = sc.getType("bool")
		case intT:
			typ = sc.getType("int")
		case float64T:
			typ = sc.getType("float64")
		case complex128T:
			typ = sc.getType("complex128")
		default:
			*typ = *t
			typ.untyped = false
		}
	}
	return typ
}

func (t *itype) isNil() bool { return t.cat == nilT }

func (t *itype) hasNil() bool {
	switch rt := t.TypeOf(); rt.Kind() {
	case reflect.UnsafePointer:
		return true
	case reflect.Slice, reflect.Ptr, reflect.Func, reflect.Interface, reflect.Map, reflect.Chan:
		return true
	case reflect.Struct:
		if rt == valueInterfaceType {
			return true
		}
	}
	return false
}

func (t *itype) elem() *itype {
	if t.cat == valueT {
		return valueTOf(t.rtype.Elem())
	}
	return t.val
}

func hasElem(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Ptr, reflect.Slice:
		return true
	}
	return false
}

func constToInt(c constant.Value) int {
	if constant.BitLen(c) > 64 {
		panic(fmt.Sprintf("constant %s overflows int64", c.ExactString()))
	}
	i, _ := constant.Int64Val(c)
	return int(i)
}

func constToString(v reflect.Value) string {
	c := v.Interface().(constant.Value)
	return constant.StringVal(c)
}

func wrappedType(n *node) *itype {
	if n.typ.cat != valueT {
		return nil
	}
	return n.typ.val
}

func isShiftNode(n *node) bool {
	switch n.action {
	case aShl, aShr, aShlAssign, aShrAssign:
		return true
	}
	return false
}

// chanElement returns the channel element type.
func chanElement(t *itype) *itype {
	switch t.cat {
	case linkedT:
		return chanElement(t.val)
	case chanT, chanSendT, chanRecvT:
		return t.val
	case valueT:
		return valueTOf(t.rtype.Elem(), withNode(t.node), withScope(t.scope))
	}
	return nil
}

func isBool(t *itype) bool { return t.TypeOf().Kind() == reflect.Bool }
func isChan(t *itype) bool { return t.TypeOf().Kind() == reflect.Chan }
func isFunc(t *itype) bool { return t.TypeOf().Kind() == reflect.Func }
func isMap(t *itype) bool  { return t.TypeOf().Kind() == reflect.Map }
func isPtr(t *itype) bool  { return t.TypeOf().Kind() == reflect.Ptr }

func isEmptyInterface(t *itype) bool {
	return t.cat == interfaceT && len(t.field) == 0
}

func isGeneric(t *itype) bool {
	return t.cat == funcT && t.node != nil && len(t.node.child) > 0 && len(t.node.child[0].child) > 0
}

func isNamedFuncSrc(t *itype) bool {
	return isFuncSrc(t) && t.node.anc.kind == funcDecl
}

func isFuncSrc(t *itype) bool {
	return t.cat == funcT || (t.cat == linkedT && isFuncSrc(t.val))
}

func isPtrSrc(t *itype) bool {
	return t.cat == ptrT || (t.cat == linkedT && isPtrSrc(t.val))
}

func isSendChan(t *itype) bool {
	rt := t.TypeOf()
	return rt.Kind() == reflect.Chan && rt.ChanDir() == reflect.SendDir
}

func isArray(t *itype) bool {
	if t.cat == nilT {
		return false
	}
	k := t.TypeOf().Kind()
	return k == reflect.Array || k == reflect.Slice
}

func isInterfaceSrc(t *itype) bool {
	return t.cat == interfaceT || (t.cat == linkedT && isInterfaceSrc(t.val))
}

func isInterfaceBin(t *itype) bool {
	return t.cat == valueT && t.rtype.Kind() == reflect.Interface || t.cat == errorT
}

func isInterface(t *itype) bool {
	return isInterfaceSrc(t) || t.TypeOf() == valueInterfaceType || t.TypeOf() != nil && t.TypeOf().Kind() == reflect.Interface
}

func isBin(t *itype) bool {
	switch t.cat {
	case valueT:
		return true
	case linkedT, ptrT:
		return isBin(t.val)
	default:
		return false
	}
}

func isStruct(t *itype) bool {
	// Test first for a struct category, because a recursive interpreter struct may be
	// represented by an interface{} at reflect level.
	switch t.cat {
	case structT:
		return true
	case linkedT, ptrT:
		return isStruct(t.val)
	case valueT:
		k := t.rtype.Kind()
		return k == reflect.Struct || (k == reflect.Ptr && t.rtype.Elem().Kind() == reflect.Struct)
	default:
		return false
	}
}

func isConstType(t *itype) bool {
	rt := t.TypeOf()
	return isBoolean(rt) || isString(rt) || isNumber(rt)
}

func isInt(t reflect.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

func isUint(t reflect.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

func isComplex(t reflect.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Complex64, reflect.Complex128:
		return true
	}
	return false
}

func isFloat(t reflect.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func isByteArray(t reflect.Type) bool {
	if t == nil {
		return false
	}
	k := t.Kind()
	return (k == reflect.Array || k == reflect.Slice) && t.Elem().Kind() == reflect.Uint8
}

func isFloat32(t reflect.Type) bool { return t != nil && t.Kind() == reflect.Float32 }
func isFloat64(t reflect.Type) bool { return t != nil && t.Kind() == reflect.Float64 }
func isNumber(t reflect.Type) bool {
	return isInt(t) || isFloat(t) || isComplex(t) || isConstantValue(t)
}
func isBoolean(t reflect.Type) bool       { return t != nil && t.Kind() == reflect.Bool }
func isString(t reflect.Type) bool        { return t != nil && t.Kind() == reflect.String }
func isConstantValue(t reflect.Type) bool { return t != nil && t.Implements(constVal) }
