package interp

import (
	"fmt"
	"go/constant"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"

	"github.com/traefik/yaegi/internal/unsafe2"
)

// tcat defines interpreter type categories.
type tcat uint

// Types for go language.
const (
	nilT tcat = iota
	aliasT
	arrayT
	binT
	binPkgT
	boolT
	builtinT
	chanT
	chanSendT
	chanRecvT
	complex64T
	complex128T
	errorT
	float32T
	float64T
	funcT
	interfaceT
	intT
	int8T
	int16T
	int32T
	int64T
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
	aliasT:      "aliasT",
	arrayT:      "arrayT",
	binT:        "binT",
	binPkgT:     "binPkgT",
	boolT:       "boolT",
	builtinT:    "builtinT",
	chanT:       "chanT",
	complex64T:  "complex64T",
	complex128T: "complex128T",
	errorT:      "errorT",
	float32T:    "float32",
	float64T:    "float64T",
	funcT:       "funcT",
	interfaceT:  "interfaceT",
	intT:        "intT",
	int8T:       "int8T",
	int16T:      "int16T",
	int32T:      "int32T",
	int64T:      "int64T",
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
	mu          *sync.Mutex
	cat         tcat          // Type category
	field       []structField // Array of struct fields if structT or interfaceT
	key         *itype        // Type of key element if MapT or nil
	val         *itype        // Type of value element if chanT, chanSendT, chanRecvT, mapT, ptrT, aliasT, arrayT, sliceT or variadicT
	recv        *itype        // Receiver type for funcT or nil
	arg         []*itype      // Argument types if funcT or nil
	ret         []*itype      // Return types if funcT or nil
	ptr         *itype        // Pointer to this type. Might be nil
	method      []*node       // Associated methods or nil
	name        string        // name of type within its package for a defined type
	path        string        // for a defined type, the package import path
	length      int           // length of array if ArrayT
	rtype       reflect.Type  // Reflection type if ValueT, or nil
	incomplete  bool          // true if type must be parsed again (out of order declarations)
	recursive   bool          // true if the type has an element which refer to itself
	untyped     bool          // true for a literal value (string or number)
	isBinMethod bool          // true if the type refers to a bin method function
	node        *node         // root AST node of type definition
	scope       *scope        // type declaration scope (in case of re-parse incomplete type)
	str         string        // String representation of the type
}

func untypedBool() *itype {
	return &itype{cat: boolT, name: "bool", untyped: true, str: "untyped bool"}
}

func untypedString() *itype {
	return &itype{cat: stringT, name: "string", untyped: true, str: "untyped string"}
}

func untypedRune() *itype {
	return &itype{cat: int32T, name: "int32", untyped: true, str: "untyped rune"}
}

func untypedInt() *itype {
	return &itype{cat: intT, name: "int", untyped: true, str: "untyped int"}
}

func untypedFloat() *itype {
	return &itype{cat: float64T, name: "float64", untyped: true, str: "untyped float"}
}

func untypedComplex() *itype {
	return &itype{cat: complex128T, name: "complex128", untyped: true, str: "untyped complex"}
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
	t := &itype{cat: aliasT, val: val, path: path, name: name, str: str}
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
func interfaceOf(fields []structField, opts ...itypeOption) *itype {
	str := "interface{}"
	if len(fields) > 0 {
		str = "interface { " + methodsTypeString(fields) + "}"
	}
	t := &itype{cat: interfaceT, field: fields, str: str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// structOf returns a struct type with the given fields.
func structOf(fields []structField, opts ...itypeOption) *itype {
	str := "struct {}"
	if len(fields) > 0 {
		str = "struct { " + fieldsTypeString(fields) + "}"
	}
	t := &itype{cat: structT, field: fields, str: str}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// nodeType returns a type definition for the corresponding AST subtree.
func nodeType(interp *Interpreter, sc *scope, n *node) (*itype, error) {
	if n.typ != nil && !n.typ.incomplete {
		return n.typ, nil
	}
	if sname := typeName(n); sname != "" {
		if sym, _, found := sc.lookup(sname); found && sym.kind == typeSym && sym.typ != nil && sym.typ.isComplete() {
			return sym.typ, nil
		}
	}

	t := &itype{node: n, scope: sc}

	var err error
	switch n.kind {
	case addressExpr, starExpr:
		val, err := nodeType(interp, sc, n.child[0])
		if err != nil {
			return nil, err
		}
		t = ptrOf(val, withNode(n), withScope(sc))
		t.incomplete = val.incomplete

	case arrayType:
		c0 := n.child[0]
		if len(n.child) == 1 {
			val, err := nodeType(interp, sc, c0)
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
			// Size if defined by a constant litteral value.
			if isConstantValue(v.Type()) {
				c := v.Interface().(constant.Value)
				length = constToInt(c)
			} else {
				length = int(v.Int())
			}
		case c0.kind == ellipsisExpr:
			// [...]T expression, get size from the length of composite array.
			length = arrayTypeLen(n.anc)
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
			if _, err = interp.cfg(c0, sc.pkgID, sc.pkgName); err != nil {
				return nil, err
			}
			v, ok := c0.rval.Interface().(constant.Value)
			if !ok {
				incomplete = true
				break
			}
			length = constToInt(v)
		}
		val, err := nodeType(interp, sc, n.child[1])
		if err != nil {
			return nil, err
		}
		t = arrayOf(val, length, withNode(n), withScope(sc))
		t.incomplete = incomplete || val.incomplete

	case basicLit:
		switch v := n.rval.Interface().(type) {
		case bool:
			n.rval = reflect.ValueOf(constant.MakeBool(v))
			t = untypedBool()
		case rune:
			// It is impossible to work out rune const literals in AST
			// with the correct type so we must make the const type here.
			n.rval = reflect.ValueOf(constant.MakeInt64(int64(v)))
			t = untypedRune()
		case constant.Value:
			switch v.Kind() {
			case constant.Bool:
				t = untypedBool()
			case constant.String:
				t = untypedString()
			case constant.Int:
				t = untypedInt()
			case constant.Float:
				t = untypedFloat()
			case constant.Complex:
				t = untypedComplex()
			default:
				err = n.cfgErrorf("missing support for type %v", n.rval)
			}
		default:
			err = n.cfgErrorf("missing support for type %T: %v", v, n.rval)
		}

	case unaryExpr:
		t, err = nodeType(interp, sc, n.child[0])

	case binaryExpr:
		// Get type of first operand.
		if t, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		// For operators other than shift, get the type from the 2nd operand if the first is untyped.
		if t.untyped && !isShiftNode(n) {
			var t1 *itype
			t1, err = nodeType(interp, sc, n.child[1])
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
			if dt, err = nodeType(interp, sc, a.child[a.nleft]); err != nil {
				return nil, err
			}

		case a.kind == returnStmt:
			dt = sc.def.typ.ret[childPos(n)]
		}

		if isInterfaceSrc(dt) {
			dt.val = t
		}
		t = dt

	case callExpr:
		if isBuiltinCall(n, sc) {
			// Builtin types are special and may depend from their input arguments.
			t.cat = builtinT
			switch n.child[0].ident {
			case bltnComplex:
				var nt0, nt1 *itype
				if nt0, err = nodeType(interp, sc, n.child[1]); err != nil {
					return nil, err
				}
				if nt1, err = nodeType(interp, sc, n.child[2]); err != nil {
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
						t = untypedComplex()
					case nt0.untyped && isFloat32(t1) || nt1.untyped && isFloat32(t0):
						t = sc.getType("complex64")
					case nt0.untyped && isFloat64(t1) || nt1.untyped && isFloat64(t0):
						t = sc.getType("complex128")
					default:
						err = n.cfgErrorf("invalid types %s and %s", t0.Kind(), t1.Kind())
					}
					if nt0.untyped && nt1.untyped {
						t = untypedComplex()
					}
				}
			case bltnReal, bltnImag:
				if t, err = nodeType(interp, sc, n.child[1]); err != nil {
					return nil, err
				}
				if !t.incomplete {
					switch k := t.TypeOf().Kind(); {
					case k == reflect.Complex64:
						t = sc.getType("float32")
					case k == reflect.Complex128:
						t = sc.getType("float64")
					case t.untyped && isNumber(t.TypeOf()):
						t = valueTOf(floatType, withUntyped(true), withScope(sc))
					default:
						err = n.cfgErrorf("invalid complex type %s", k)
					}
				}
			case bltnCap, bltnCopy, bltnLen:
				t = sc.getType("int")
			case bltnAppend, bltnMake:
				t, err = nodeType(interp, sc, n.child[1])
			case bltnNew:
				t, err = nodeType(interp, sc, n.child[1])
				incomplete := t.incomplete
				t = ptrOf(t, withScope(sc))
				t.incomplete = incomplete
			case bltnRecover:
				t = sc.getType("interface{}")
			}
			if err != nil {
				return nil, err
			}
		} else {
			if t, err = nodeType(interp, sc, n.child[0]); err != nil {
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
		t, err = nodeType(interp, sc, n.child[0])

	case chanType, chanTypeRecv, chanTypeSend:
		dir := chanSendRecv
		switch n.kind {
		case chanTypeRecv:
			dir = chanRecv
		case chanTypeSend:
			dir = chanSend
		}
		val, err := nodeType(interp, sc, n.child[0])
		if err != nil {
			return nil, err
		}
		t = chanOf(val, dir, withNode(n), withScope(sc))
		t.incomplete = val.incomplete

	case ellipsisExpr:
		t.cat = variadicT
		if t.val, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		t.str = "..." + t.val.str
		t.incomplete = t.val.incomplete

	case funcLit:
		t, err = nodeType(interp, sc, n.child[2])

	case funcType:
		var incomplete bool
		// Handle input parameters
		args := make([]*itype, 0, len(n.child[0].child))
		for _, arg := range n.child[0].child {
			cl := len(arg.child) - 1
			typ, err := nodeType(interp, sc, arg.child[cl])
			if err != nil {
				return nil, err
			}
			args = append(args, typ)
			for i := 1; i < cl; i++ {
				// Several arguments may be factorized on the same field type
				args = append(args, typ)
			}
			incomplete = incomplete || typ.incomplete
		}

		var rets []*itype
		if len(n.child) == 2 {
			// Handle returned values
			for _, ret := range n.child[1].child {
				cl := len(ret.child) - 1
				typ, err := nodeType(interp, sc, ret.child[cl])
				if err != nil {
					return nil, err
				}
				rets = append(rets, typ)
				for i := 1; i < cl; i++ {
					// Several arguments may be factorized on the same field type
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
				t.name = n.ident
				t.path = sc.pkgName
				t.incomplete = true
				sc.sym[n.ident] = &symbol{kind: typeSym, typ: t}
				break
			}
		}
		t = sym.typ
		if t.incomplete && t.cat == aliasT && t.val != nil && t.val.cat != nilT {
			t.incomplete = false
		}
		if t.incomplete && t.node != n {
			m := t.method
			if t, err = nodeType(interp, sc, t.node); err != nil {
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
		if lt, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		if lt.incomplete {
			t.incomplete = true
			break
		}
		switch lt.cat {
		case arrayT, mapT, sliceT, variadicT:
			t = lt.val
		}

	case interfaceType:
		t.cat = interfaceT
		var incomplete bool
		if sname := typeName(n); sname != "" {
			if sym, _, found := sc.lookup(sname); found && sym.kind == typeSym {
				sym.typ = t
			}
		}
		fields := make([]structField, 0, len(n.child[0].child))
		for _, field := range n.child[0].child {
			f0 := field.child[0]
			if len(field.child) == 1 {
				if f0.ident == "error" {
					// Unwrap error interface inplace rather than embedding it, because
					// "error" is lower case which may cause problems with reflect for method lookup.
					typ := errorMethodType(sc)
					fields = append(fields, structField{name: "Error", typ: typ})
					continue
				}
				typ, err := nodeType(interp, sc, f0)
				if err != nil {
					return nil, err
				}
				fields = append(fields, structField{name: fieldName(f0), embed: true, typ: typ})
				incomplete = incomplete || typ.incomplete
				continue
			}
			typ, err := nodeType(interp, sc, field.child[1])
			if err != nil {
				return nil, err
			}
			fields = append(fields, structField{name: f0.ident, typ: typ})
			incomplete = incomplete || typ.incomplete
		}
		*t = *interfaceOf(fields, withNode(n), withScope(sc))
		t.incomplete = incomplete

	case landExpr, lorExpr:
		t = sc.getType("bool")

	case mapType:
		key, err := nodeType(interp, sc, n.child[0])
		if err != nil {
			return nil, err
		}
		val, err := nodeType(interp, sc, n.child[1])
		if err != nil {
			return nil, err
		}
		t = mapOf(key, val, withNode(n), withScope(sc))
		t.incomplete = key.incomplete || val.incomplete

	case parenExpr:
		t, err = nodeType(interp, sc, n.child[0])

	case selectorExpr:
		// Resolve the left part of selector, then lookup the right part on it
		var lt *itype

		// If we are in a list of func parameters, and we are a selector on a binPkgT, but
		// one of the other parameters has the same name as the pkg name, in the list of
		// symbols we would find the other parameter instead of the pkg because it comes
		// first when looking up in the stack of scopes. So in that case we force the
		// lookup directly in the root scope to shortcircuit that issue.
		var localScope *scope
		localScope = sc
		if n.anc != nil && len(n.anc.child) > 1 && n.anc.child[1] == n &&
			// This check is weaker than what we actually want to know, i.e. whether
			// n.anc.child[0] is a variable, but it seems at this point in the run we have no
			// way of knowing that yet (typ is nil, so there's no typ.cat yet).
			n.anc.child[0].kind == identExpr {
			for {
				if localScope.level == 0 {
					break
				}
				localScope = localScope.anc
			}
		}

		if lt, err = nodeType(interp, localScope, n.child[0]); err != nil {
			return nil, err
		}

		if lt.incomplete {
			t.incomplete = true
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
				t, err = nodeType(interp, sc, m.child[2])
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
		t, err = nodeType(interp, sc, n.child[0])
		if err != nil {
			return nil, err
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
		t.cat = structT
		var (
			methods    []*node
			incomplete bool
		)
		if sname := typeName(n); sname != "" {
			if sym, _, found := sc.lookup(sname); found && sym.kind == typeSym {
				methods = sym.typ.method
				sym.typ = t
			}
		}
		fields := make([]structField, 0, len(n.child[0].child))
		for _, c := range n.child[0].child {
			switch {
			case len(c.child) == 1:
				typ, err := nodeType(interp, sc, c.child[0])
				if err != nil {
					return nil, err
				}
				fields = append(fields, structField{name: fieldName(c.child[0]), embed: true, typ: typ})
				incomplete = incomplete || typ.incomplete
			case len(c.child) == 2 && c.child[1].kind == basicLit:
				tag := vString(c.child[1].rval)
				typ, err := nodeType(interp, sc, c.child[0])
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
				typ, err := nodeType(interp, sc, c.child[l-1])
				if err != nil {
					return nil, err
				}
				incomplete = incomplete || typ.incomplete
				for _, d := range c.child[:l-1] {
					fields = append(fields, structField{name: d.ident, typ: typ, tag: tag})
				}
			}
		}
		*t = *structOf(fields, withNode(n), withScope(sc))
		t.method = methods // Recover the symbol methods.
		t.incomplete = incomplete

	default:
		err = n.cfgErrorf("type definition not implemented: %s", n.kind)
	}

	if err == nil && t.cat == nilT && !t.incomplete {
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

// ReferTo returns true if the type contains a reference to a
// full type name. It allows to assess a type recursive status.
func (t *itype) referTo(name string, seen map[*itype]bool) bool {
	if t.path+"/"+t.name == name {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.cat {
	case aliasT, arrayT, chanT, chanRecvT, chanSendT, ptrT, sliceT, variadicT:
		return t.val.referTo(name, seen)
	case funcT:
		for _, a := range t.arg {
			if a.referTo(name, seen) {
				return true
			}
		}
		for _, a := range t.ret {
			if a.referTo(name, seen) {
				return true
			}
		}
	case mapT:
		return t.key.referTo(name, seen) || t.val.referTo(name, seen)
	case structT, interfaceT:
		for _, f := range t.field {
			if f.typ.referTo(name, seen) {
				return true
			}
		}
	}
	return false
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
			if t.recv != nil {
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
	}
	return 1
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

// IsRecursive returns true if type is recursive.
// Only a named struct or interface can be recursive.
func (t *itype) isRecursive() bool {
	if t.name == "" {
		return false
	}
	switch t.cat {
	case structT, interfaceT:
		for _, f := range t.field {
			if f.typ.referTo(t.path+"/"+t.name, map[*itype]bool{}) {
				return true
			}
		}
	}
	return false
}

func (t *itype) isIndirectRecursive() bool {
	return t.isRecursive() || t.val != nil && t.val.isIndirectRecursive()
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
	case aliasT:
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
	if t.cat == aliasT && o.cat == aliasT {
		// If alias types are not identical, it is not assignable.
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

	if t.isBinMethod && isFunc(o) {
		// TODO (marc): check that t without receiver as first parameter is equivalent to o.
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
		case aliasT:
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
	case aliasT:
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
	case aliasT, ptrT:
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

	lookup = func(typ *itype) []int {
		if seen[typ] {
			return nil
		}
		seen[typ] = true

		switch typ.cat {
		case aliasT, ptrT:
			return lookup(typ.val)
		}
		if fi := typ.fieldIndex(name); fi >= 0 {
			return []int{fi}
		}

		for i, f := range typ.field {
			switch f.typ.cat {
			case ptrT, structT, interfaceT, aliasT:
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
	for t.cat == aliasT {
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
	if t.cat == ptrT {
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
		if t.cat == aliasT || isInterfaceSrc(t) && t.val != nil {
			return t.val.lookupMethod(name)
		}
	}
	return m, index
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
	if t.cat == ptrT {
		return t.val.lookupBinMethod(name)
	}
	for i, f := range t.field {
		if f.embed {
			if m2, index2, isPtr2, ok2 := f.typ.lookupBinMethod(name); ok2 {
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
	interf   = reflect.TypeOf((*interface{})(nil)).Elem()
	constVal = reflect.TypeOf((*constant.Value)(nil)).Elem()
)

type fieldRebuild struct {
	typ *itype
	idx int
}

type refTypeContext struct {
	defined    map[string]*itype
	refs       map[string][]fieldRebuild
	rebuilding bool
}

// Clone creates a copy if the ref type context without the `needsRebuild` set.
func (c *refTypeContext) Clone() *refTypeContext {
	return &refTypeContext{defined: c.defined, refs: c.refs, rebuilding: c.rebuilding}
}

// RefType returns a reflect.Type representation from an interpreter type.
// In simple cases, reflect types are directly mapped from the interpreter
// counterpart.
// For recursive named struct or interfaces, as reflect does not permit to
// create a recursive named struct, a nil type is set temporarily for each recursive
// field. When done, the nil type fields are updated with the original reflect type
// pointer using unsafe. We thus obtain a usable recursive type definition, except
// for string representation, as created reflect types are still unnamed.
func (t *itype) refType(ctx *refTypeContext) reflect.Type {
	if ctx == nil {
		ctx = &refTypeContext{
			defined: map[string]*itype{},
			refs:    map[string][]fieldRebuild{},
		}
	}
	if t.incomplete || t.cat == nilT {
		var err error
		if t, err = t.finalize(); err != nil {
			panic(err)
		}
	}
	recursive := false
	name := t.path + "/" + t.name
	// Predefined types from universe or runtime may have a nil scope.
	if t.scope != nil {
		if st := t.scope.sym[t.name]; st != nil {
			// Update the type recursive status. Several copies of type
			// may exist per symbol, as a new type is created at each GTA
			// pass (several needed due to out of order declarations), and
			// a node can still point to a previous copy.
			st.typ.recursive = st.typ.recursive || st.typ.isRecursive()
			recursive = st.typ.isRecursive()
			// It is possible that t.recursive is not inline with st.typ.recursive
			// which will break recursion detection. Set it here to make sure it
			// is correct.
			t.recursive = recursive
		}
	}
	if t.rtype != nil && !ctx.rebuilding {
		return t.rtype
	}
	if dt := ctx.defined[name]; dt != nil {
		if dt.rtype != nil {
			t.rtype = dt.rtype
			return dt.rtype
		}

		// To indicate that a rebuild is needed on the nearest struct
		// field, create an entry with a nil type.
		flds := ctx.refs[name]
		ctx.refs[name] = append(flds, fieldRebuild{})
		return unsafe2.DummyType
	}
	switch t.cat {
	case aliasT:
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
		t.rtype = interf
	case mapT:
		t.rtype = reflect.MapOf(t.key.refType(ctx), t.val.refType(ctx))
	case ptrT:
		t.rtype = reflect.PtrTo(t.val.refType(ctx))
	case structT:
		if t.name != "" {
			ctx.defined[name] = t
		}
		var fields []reflect.StructField
		for i, f := range t.field {
			fctx := ctx.Clone()
			field := reflect.StructField{
				Name: exportName(f.name), Type: f.typ.refType(fctx),
				Tag: reflect.StructTag(f.tag), Anonymous: (f.embed && !recursive),
			}
			fields = append(fields, field)
			// Find any nil type refs that indicates a rebuild is needed on this field.
			for _, flds := range ctx.refs {
				for j, fld := range flds {
					if fld.typ == nil {
						flds[j] = fieldRebuild{typ: t, idx: i}
					}
				}
			}
		}
		t.rtype = reflect.StructOf(fields)

		// The rtype has now been built, we can go back and rebuild
		// all the recursive types that relied on this type.
		for _, f := range ctx.refs[name] {
			ftyp := f.typ.field[f.idx].typ.refType(&refTypeContext{defined: ctx.defined, rebuilding: true})
			unsafe2.SwapFieldType(f.typ.rtype, f.idx, ftyp)
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
	case aliasT:
		r = t.val.frameType()
	case arrayT:
		r = reflect.ArrayOf(t.length, t.val.frameType())
	case sliceT, variadicT:
		r = reflect.SliceOf(t.val.frameType())
	case funcT:
		r = reflect.TypeOf((*node)(nil))
	case interfaceT:
		if len(t.field) == 0 {
			// empty interface, do not wrap it
			r = reflect.TypeOf((*interface{})(nil)).Elem()
			break
		}
		r = reflect.TypeOf((*valueInterface)(nil)).Elem()
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
	switch t.TypeOf().Kind() {
	case reflect.UnsafePointer:
		return true
	case reflect.Slice, reflect.Ptr, reflect.Func, reflect.Interface, reflect.Map, reflect.Chan:
		return true
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

func defRecvType(n *node) *itype {
	if n.kind != funcDecl || len(n.child[0].child) == 0 {
		return nil
	}
	if r := n.child[0].child[0].lastChild(); r != nil {
		return r.typ
	}
	return nil
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
	case aliasT:
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

func isFuncSrc(t *itype) bool {
	return t.cat == funcT || (t.cat == aliasT && isFuncSrc(t.val))
}

func isPtrSrc(t *itype) bool {
	return t.cat == ptrT || (t.cat == aliasT && isPtrSrc(t.val))
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
	return t.cat == interfaceT || (t.cat == aliasT && isInterfaceSrc(t.val))
}

func isInterfaceBin(t *itype) bool {
	return t.cat == valueT && t.rtype.Kind() == reflect.Interface || t.cat == errorT
}

func isInterface(t *itype) bool {
	return isInterfaceSrc(t) || t.TypeOf() != nil && t.TypeOf().Kind() == reflect.Interface
}

func isBin(t *itype) bool {
	switch t.cat {
	case valueT:
		return true
	case aliasT, ptrT:
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
	case aliasT, ptrT:
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
