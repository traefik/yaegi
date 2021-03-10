package interp

import (
	"fmt"
	"go/constant"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
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
	val         *itype        // Type of value element if chanT, chanSendT, chanRecvT, mapT, ptrT, aliasT, arrayT or variadicT
	recv        *itype        // Receiver type for funcT or nil
	arg         []*itype      // Argument types if funcT or nil
	ret         []*itype      // Return types if funcT or nil
	method      []*node       // Associated methods or nil
	name        string        // name of type within its package for a defined type
	path        string        // for a defined type, the package import path
	size        int           // Size of array if ArrayT
	rtype       reflect.Type  // Reflection type if ValueT, or nil
	incomplete  bool          // true if type must be parsed again (out of order declarations)
	recursive   bool          // true if the type has an element which refer to itself
	untyped     bool          // true for a literal value (string or number)
	sizedef     bool          // true if array size is computed from type definition
	isBinMethod bool          // true if the type refers to a bin method function
	node        *node         // root AST node of type definition
	scope       *scope        // type declaration scope (in case of re-parse incomplete type)
}

func untypedBool() *itype    { return &itype{cat: boolT, name: "bool", untyped: true} }
func untypedString() *itype  { return &itype{cat: stringT, name: "string", untyped: true} }
func untypedRune() *itype    { return &itype{cat: int32T, name: "int32", untyped: true} }
func untypedInt() *itype     { return &itype{cat: intT, name: "int", untyped: true} }
func untypedFloat() *itype   { return &itype{cat: float64T, name: "float64", untyped: true} }
func untypedComplex() *itype { return &itype{cat: complex128T, name: "complex128", untyped: true} }

// nodeType returns a type definition for the corresponding AST subtree.
func nodeType(interp *Interpreter, sc *scope, n *node) (*itype, error) {
	if n.typ != nil && !n.typ.incomplete {
		if n.kind == sliceExpr {
			n.typ.sizedef = false
		}
		return n.typ, nil
	}

	t := &itype{node: n, scope: sc}

	if n.anc.kind == typeSpec {
		name := n.anc.child[0].ident
		if sym := sc.sym[name]; sym != nil {
			// recover previously declared methods
			t.method = sym.typ.method
			t.path = sym.typ.path
			t.name = name
		}
	}

	var err error
	switch n.kind {
	case addressExpr, starExpr:
		t.cat = ptrT
		if t.val, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		t.incomplete = t.val.incomplete

	case arrayType:
		t.cat = arrayT
		c0 := n.child[0]
		if len(n.child) == 1 {
			// Array size is not defined.
			if t.val, err = nodeType(interp, sc, c0); err != nil {
				return nil, err
			}
			t.incomplete = t.val.incomplete
			break
		}
		// Array size is defined.
		switch v := c0.rval; {
		case v.IsValid():
			// Size if defined by a constant litteral value.
			if isConstantValue(v.Type()) {
				c := v.Interface().(constant.Value)
				t.size = constToInt(c)
			} else {
				t.size = int(v.Int())
			}
		case c0.kind == ellipsisExpr:
			// [...]T expression, get size from the length of composite array.
			t.size = arrayTypeLen(n.anc)
		case c0.kind == identExpr:
			sym, _, ok := sc.lookup(c0.ident)
			if !ok {
				t.incomplete = true
				break
			}
			// Size is defined by a symbol which must be a constant integer.
			if sym.kind != constSym {
				return nil, c0.cfgErrorf("non-constant array bound %q", c0.ident)
			}
			if sym.typ == nil || sym.typ.cat != intT || !sym.rval.IsValid() {
				t.incomplete = true
				break
			}
			if v, ok := sym.rval.Interface().(int); ok {
				t.size = v
				break
			}
			if c, ok := sym.rval.Interface().(constant.Value); ok {
				t.size = constToInt(c)
				break
			}
			t.incomplete = true
		default:
			// Size is defined by a numeric constant expression.
			if _, err = interp.cfg(c0, sc.pkgID); err != nil {
				return nil, err
			}
			v, ok := c0.rval.Interface().(constant.Value)
			if !ok {
				t.incomplete = true
				break
			}
			t.size = constToInt(v)
		}
		if t.val, err = nodeType(interp, sc, n.child[1]); err != nil {
			return nil, err
		}
		t.sizedef = true
		t.incomplete = t.incomplete || t.val.incomplete

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

		// Because an empty interface concrete type "mutates" as different values are
		// assigned to it, we need to make a new itype from scratch everytime a new
		// assignment is made, and not let different nodes (of the same variable) share the
		// same itype. Otherwise they would overwrite each other.
		if n.anc.kind == assignStmt && isInterface(n.anc.child[0].typ) && len(n.anc.child[0].typ.field) == 0 {
			// TODO(mpl): do the indexes properly for multiple assignments on the same line.
			// Also, maybe we should use nodeType to figure out dt.cat? but isn't it always
			// gonna be an interfaceT anyway?
			dt := new(itype)
			dt.cat = interfaceT
			val := new(itype)
			val.cat = t.cat
			dt.val = val
			// TODO(mpl): do the indexes properly for multiple assignments on the same line.
			// Also, maybe we should use nodeType to figure out dt.cat? but isn't it always
			// gonna be an interfaceT anyway?
			n.anc.child[0].typ = dt
			// TODO(mpl): not sure yet whether we should do that last step. It doesn't seem
			// to change anything either way though.
			// t = dt
			break
		}

		// If the node is to be assigned or returned, the node type is the destination type.
		dt := t

		switch a := n.anc; {
		case a.kind == defineStmt && len(a.child) > a.nleft+a.nright:
			if dt, err = nodeType(interp, sc, a.child[a.nleft]); err != nil {
				return nil, err
			}
		case a.kind == returnStmt:
			dt = sc.def.typ.ret[childPos(n)]
		}
		if isInterface(dt) {
			dt.val = t
		}
		t = dt

	case callExpr:
		if interp.isBuiltinCall(n) {
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
						t.untyped = true
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
						t = &itype{cat: valueT, rtype: floatType, untyped: true, scope: sc}
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
				t = &itype{cat: ptrT, val: t, incomplete: t.incomplete, scope: sc}
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
					t = &itype{cat: valueT, rtype: rt.Out(0), scope: sc}
				}
			default:
				if len(t.ret) == 1 {
					t = t.ret[0]
				}
			}
		}

	case compositeLitExpr:
		t, err = nodeType(interp, sc, n.child[0])

	case chanType:
		t.cat = chanT
		if t.val, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		t.incomplete = t.val.incomplete

	case chanTypeRecv:
		t.cat = chanRecvT
		if t.val, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		t.incomplete = t.val.incomplete

	case chanTypeSend:
		t.cat = chanSendT
		if t.val, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		t.incomplete = t.val.incomplete

	case ellipsisExpr:
		t.cat = variadicT
		if t.val, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		t.incomplete = t.val.incomplete

	case funcLit:
		t, err = nodeType(interp, sc, n.child[2])

	case funcType:
		t.cat = funcT
		// Handle input parameters
		for _, arg := range n.child[0].child {
			cl := len(arg.child) - 1
			typ, err := nodeType(interp, sc, arg.child[cl])
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
				typ, err := nodeType(interp, sc, ret.child[cl])
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

	case identExpr:
		sym, _, found := sc.lookup(n.ident)
		if !found {
			// retry with the filename, in case ident is a package name.
			baseName := filepath.Base(interp.fset.Position(n.pos).Filename)
			ident := filepath.Join(n.ident, baseName)
			sym, _, found = sc.lookup(ident)
			if !found {
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
		case arrayT, mapT:
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
		for _, field := range n.child[0].child {
			if len(field.child) == 1 {
				typ, err := nodeType(interp, sc, field.child[0])
				if err != nil {
					return nil, err
				}
				t.field = append(t.field, structField{name: fieldName(field.child[0]), embed: true, typ: typ})
				incomplete = incomplete || typ.incomplete
			} else {
				typ, err := nodeType(interp, sc, field.child[1])
				if err != nil {
					return nil, err
				}
				t.field = append(t.field, structField{name: field.child[0].ident, typ: typ})
				incomplete = incomplete || typ.incomplete
			}
		}
		t.incomplete = incomplete

	case landExpr, lorExpr:
		t.cat = boolT

	case mapType:
		t.cat = mapT
		if t.key, err = nodeType(interp, sc, n.child[0]); err != nil {
			return nil, err
		}
		if t.val, err = nodeType(interp, sc, n.child[1]); err != nil {
			return nil, err
		}
		t.incomplete = t.key.incomplete || t.val.incomplete

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
				t.cat = valueT
				t.rtype = v.Type()
				if isBinType(v) {
					// A bin type is encoded as a pointer on a typed nil value.
					t.rtype = t.rtype.Elem()
				}
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
				t = &itype{cat: valueT, rtype: bm.Type, recv: lt, isBinMethod: true, scope: sc}
			} else if ti := lt.lookupField(name); len(ti) > 0 {
				t = lt.fieldSeq(ti)
			} else if bs, _, ok := lt.lookupBinField(name); ok {
				t = &itype{cat: valueT, rtype: bs.Type, scope: sc}
			} else {
				err = lt.node.cfgErrorf("undefined selector %s", name)
			}
		}

	case sliceExpr:
		t, err = nodeType(interp, sc, n.child[0])
		if t.cat == ptrT {
			t = t.val
		}
		if err == nil && t.size != 0 {
			t1 := *t
			t1.size = 0
			t1.sizedef = false
			t1.rtype = nil
			t = &t1
		}

	case structType:
		t.cat = structT
		var incomplete bool
		if sname := typeName(n); sname != "" {
			if sym, _, found := sc.lookup(sname); found && sym.kind == typeSym {
				sym.typ = t
			}
		}
		for _, c := range n.child[0].child {
			switch {
			case len(c.child) == 1:
				typ, err := nodeType(interp, sc, c.child[0])
				if err != nil {
					return nil, err
				}
				t.field = append(t.field, structField{name: fieldName(c.child[0]), embed: true, typ: typ})
				incomplete = incomplete || typ.incomplete
			case len(c.child) == 2 && c.child[1].kind == basicLit:
				tag := vString(c.child[1].rval)
				typ, err := nodeType(interp, sc, c.child[0])
				if err != nil {
					return nil, err
				}
				t.field = append(t.field, structField{name: fieldName(c.child[0]), embed: true, typ: typ, tag: tag})
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
					t.field = append(t.field, structField{name: d.ident, typ: typ, tag: tag})
				}
			}
		}
		t.incomplete = incomplete

	default:
		err = n.cfgErrorf("type definition not implemented: %s", n.kind)
	}

	if err == nil && t.cat == nilT && !t.incomplete {
		err = n.cfgErrorf("use of untyped nil %s", t.name)
	}

	return t, err
}

func (interp *Interpreter) isBuiltinCall(n *node) bool {
	if n.kind != callExpr {
		return false
	}
	s := interp.universe.sym[n.child[0].ident]
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
	case aliasT, arrayT, chanT, chanRecvT, chanSendT, ptrT:
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
				return &itype{cat: variadicT, val: &itype{cat: valueT, rtype: t.rtype.In(i).Elem()}}
			}
			return &itype{cat: valueT, rtype: t.rtype.In(i)}
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
			return &itype{cat: valueT, rtype: t.rtype.Out(i)}
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
	case arrayT, chanT, chanRecvT, chanSendT, ptrT:
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
		// if alias types are not identical, it is not assignable.
		return false
	}
	if t.isNil() && o.hasNil() || o.isNil() && t.hasNil() {
		return true
	}

	if t.TypeOf().AssignableTo(o.TypeOf()) {
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
			for i := typ.rtype.NumMethod() - 1; i >= 0; i-- {
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
	if t.name != "" {
		if t.path != "" {
			return t.path + "." + t.name
		}
		return t.name
	}
	switch t.cat {
	case nilT:
		res = "nil"
	case arrayT:
		if t.size == 0 {
			res = "[]" + t.val.id()
		} else {
			res = "[" + strconv.Itoa(t.size) + "]" + t.val.id()
		}
	case chanT:
		res = "chan " + t.val.id()
	case chanSendT:
		res = "chan<- " + t.val.id()
	case chanRecvT:
		res = "<-chan " + t.val.id()
	case funcT:
		res = "func("
		for i, t := range t.arg {
			if i > 0 {
				res += ","
			}
			res += t.id()
		}
		res += ")("
		for i, t := range t.ret {
			if i > 0 {
				res += ","
			}
			res += t.id()
		}
		res += ")"
	case interfaceT:
		res = "interface{"
		for _, t := range t.field {
			res += t.name + " " + t.typ.id() + ";"
		}
		res += "}"
	case mapT:
		res = "map[" + t.key.id() + "]" + t.val.id()
	case ptrT:
		res = "*" + t.val.id()
	case structT:
		res = "struct{"
		for _, t := range t.field {
			res += t.name + " " + t.typ.id() + ";"
		}
		res += "}"
	case valueT:
		res = ""
		if t.rtype.PkgPath() != "" {
			res += t.rtype.PkgPath() + "."
		}
		res += t.rtype.Name()
	}
	return res
}

// zero instantiates and return a zero value object for the given type during execution.
func (t *itype) zero() (v reflect.Value, err error) {
	if t, err = t.finalize(); err != nil {
		return v, err
	}
	switch t.cat {
	case aliasT:
		v, err = t.val.zero()

	case arrayT, ptrT, structT:
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
	switch t.cat {
	case aliasT, ptrT:
		return t.val.lookupField(name)
	}
	if fi := t.fieldIndex(name); fi >= 0 {
		return []int{fi}
	}

	for i, f := range t.field {
		switch f.typ.cat {
		case ptrT, structT, interfaceT, aliasT:
			if index2 := f.typ.lookupField(name); len(index2) > 0 {
				return append([]int{i}, index2...)
			}
		}
	}

	return nil
}

// lookupBinField returns a structfield and a path to access an embedded binary field in a struct object.
func (t *itype) lookupBinField(name string) (s reflect.StructField, index []int, ok bool) {
	if t.cat == ptrT {
		return t.val.lookupBinField(name)
	}
	if !isStruct(t) {
		return
	}
	rt := t.rtype
	if t.cat == valueT && rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
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
	}
	return m, index
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
				recv = &itype{cat: ptrT, val: t}
			}
		}
		return &itype{cat: valueT, rtype: m.Type, recv: recv}
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

// RefType returns a reflect.Type representation from an interpreter type.
// In simple cases, reflect types are directly mapped from the interpreter
// counterpart.
// For recursive named struct or interfaces, as reflect does not permit to
// create a recursive named struct, an interface{} is returned in place to
// avoid infinitely nested structs.
func (t *itype) refType(defined map[string]*itype, wrapRecursive bool) reflect.Type {
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
	if wrapRecursive && t.recursive {
		return interf
	}
	if t.rtype != nil {
		return t.rtype
	}
	if defined[name] != nil && defined[name].rtype != nil {
		return defined[name].rtype
	}
	if t.val != nil && t.val.cat == structT && t.val.rtype == nil && hasRecursiveStruct(t.val, copyDefined(defined)) {
		// Replace reference to self (direct or indirect) by an interface{} to handle
		// recursive types with reflect.
		typ := *t.val
		t.val = &typ
		t.val.rtype = interf
		recursive = true
	}
	switch t.cat {
	case aliasT:
		t.rtype = t.val.refType(defined, wrapRecursive)
	case arrayT, variadicT:
		if t.sizedef {
			t.rtype = reflect.ArrayOf(t.size, t.val.refType(defined, wrapRecursive))
		} else {
			t.rtype = reflect.SliceOf(t.val.refType(defined, wrapRecursive))
		}
	case chanT:
		t.rtype = reflect.ChanOf(reflect.BothDir, t.val.refType(defined, wrapRecursive))
	case chanRecvT:
		t.rtype = reflect.ChanOf(reflect.RecvDir, t.val.refType(defined, wrapRecursive))
	case chanSendT:
		t.rtype = reflect.ChanOf(reflect.SendDir, t.val.refType(defined, wrapRecursive))
	case errorT:
		t.rtype = reflect.TypeOf(new(error)).Elem()
	case funcT:
		if t.name != "" {
			defined[name] = t // TODO(marc): make sure that key is name and not t.name.
		}
		variadic := false
		in := make([]reflect.Type, len(t.arg))
		out := make([]reflect.Type, len(t.ret))
		for i, v := range t.arg {
			in[i] = v.refType(defined, true)
			variadic = v.cat == variadicT
		}
		for i, v := range t.ret {
			out[i] = v.refType(defined, true)
		}
		t.rtype = reflect.FuncOf(in, out, variadic)
	case interfaceT:
		t.rtype = interf
	case mapT:
		t.rtype = reflect.MapOf(t.key.refType(defined, wrapRecursive), t.val.refType(defined, wrapRecursive))
	case ptrT:
		t.rtype = reflect.PtrTo(t.val.refType(defined, wrapRecursive))
	case structT:
		if t.name != "" {
			// Check against local t.name and not name to catch recursive type definitions.
			if defined[t.name] != nil {
				recursive = true
			}
			defined[t.name] = t
		}
		var fields []reflect.StructField
		// TODO(mpl): make Anonymous work for recursive types too. Maybe not worth the
		// effort, and we're better off just waiting for
		// https://github.com/golang/go/issues/39717 to land.
		for _, f := range t.field {
			field := reflect.StructField{
				Name: exportName(f.name), Type: f.typ.refType(defined, wrapRecursive),
				Tag: reflect.StructTag(f.tag), Anonymous: (f.embed && !recursive),
			}
			fields = append(fields, field)
		}
		if recursive && wrapRecursive {
			t.rtype = interf
		} else {
			t.rtype = reflect.StructOf(fields)
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
	return t.refType(map[string]*itype{}, false)
}

func (t *itype) frameType() (r reflect.Type) {
	var err error
	if t, err = t.finalize(); err != nil {
		panic(err)
	}
	switch t.cat {
	case aliasT:
		r = t.val.frameType()
	case arrayT, variadicT:
		if t.sizedef {
			r = reflect.ArrayOf(t.size, t.val.frameType())
		} else {
			r = reflect.SliceOf(t.val.frameType())
		}
	case funcT:
		r = reflect.TypeOf((*node)(nil))
	case interfaceT:
		if len(t.field) == 0 {
			// empty interface, do not wrap it
			r = reflect.TypeOf((*interface{})(nil)).Elem()
			break
		}
		r = reflect.TypeOf((*valueInterface)(nil)).Elem()
	case ptrT:
		r = reflect.PtrTo(t.val.frameType())
	default:
		r = t.TypeOf()
	}
	return r
}

func (t *itype) implements(it *itype) bool {
	if t.cat == valueT {
		return t.TypeOf().Implements(it.TypeOf())
	}
	return t.methods().contains(it.methods())
}

// defaultType returns the default type of an untyped type.
func (t *itype) defaultType(v reflect.Value) *itype {
	if !t.untyped {
		return t
	}
	// The default type can also be derived from a constant value.
	if v.IsValid() && t.TypeOf().Implements(constVal) {
		switch v.Interface().(constant.Value).Kind() {
		case constant.String:
			t = untypedString()
		case constant.Bool:
			t = untypedBool()
		case constant.Int:
			t = untypedInt()
		case constant.Float:
			t = untypedFloat()
		case constant.Complex:
			t = untypedComplex()
		}
	}
	typ := *t
	typ.untyped = false
	return &typ
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

func copyDefined(m map[string]*itype) map[string]*itype {
	n := make(map[string]*itype, len(m))
	for k, v := range m {
		n[k] = v
	}
	return n
}

// hasRecursiveStruct determines if a struct is a recursion or a recursion
// intermediate. A recursion intermediate is a struct that contains a recursive
// struct.
func hasRecursiveStruct(t *itype, defined map[string]*itype) bool {
	if len(defined) == 0 {
		return false
	}

	typ := t
	for typ != nil {
		if typ.cat != structT {
			typ = typ.val
			continue
		}

		if defined[typ.path+"/"+typ.name] != nil {
			return true
		}
		defined[typ.path+"/"+typ.name] = typ

		for _, f := range typ.field {
			if hasRecursiveStruct(f.typ, copyDefined(defined)) {
				return true
			}
		}
		return false
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
		return &itype{cat: valueT, rtype: t.rtype.Elem(), node: t.node, scope: t.scope}
	}
	return nil
}

func isBool(t *itype) bool { return t.TypeOf().Kind() == reflect.Bool }
func isChan(t *itype) bool { return t.TypeOf().Kind() == reflect.Chan }
func isFunc(t *itype) bool { return t.TypeOf().Kind() == reflect.Func }
func isMap(t *itype) bool  { return t.TypeOf().Kind() == reflect.Map }
func isPtr(t *itype) bool  { return t.TypeOf().Kind() == reflect.Ptr }

func isSendChan(t *itype) bool {
	rt := t.TypeOf()
	return rt.Kind() == reflect.Chan && rt.ChanDir() == reflect.SendDir
}

func isArray(t *itype) bool {
	k := t.TypeOf().Kind()
	return k == reflect.Array || k == reflect.Slice
}

func isInterfaceSrc(t *itype) bool {
	return t.cat == interfaceT || (t.cat == aliasT && isInterfaceSrc(t.val))
}

func isInterfaceBin(t *itype) bool {
	return t.cat == valueT && t.rtype.Kind() == reflect.Interface
}

func isInterface(t *itype) bool {
	return isInterfaceSrc(t) || t.TypeOf() != nil && t.TypeOf().Kind() == reflect.Interface
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
