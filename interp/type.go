package interp

import (
	"reflect"
	"strconv"
)

// tcat defines interpreter type categories
type tcat uint

// Types for go language
const (
	nilT tcat = iota
	aliasT
	arrayT
	binT
	binPkgT
	boolT
	builtinT
	chanT
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

// structField type defines a field in a struct
type structField struct {
	name  string
	tag   string
	embed bool
	typ   *itype
}

// itype defines the internal representation of types in the interpreter
type itype struct {
	cat         tcat          // Type category
	field       []structField // Array of struct fields if structT or interfaceT
	key         *itype        // Type of key element if MapT or nil
	val         *itype        // Type of value element if chanT, mapT, ptrT, aliasT, arrayT or variadicT
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

// nodeType returns a type definition for the corresponding AST subtree
func nodeType(interp *Interpreter, sc *scope, n *node) (*itype, error) {
	if n.typ != nil && !n.typ.incomplete {
		if n.kind == sliceExpr {
			n.typ.sizedef = false
		}
		return n.typ, nil
	}

	var t = &itype{node: n, scope: sc}

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
		if len(n.child) > 1 {
			switch {
			case n.child[0].rval.IsValid():
				// constant size
				t.size = int(n.child[0].rval.Int())
			case n.child[0].kind == ellipsisExpr:
				// [...]T expression
				t.size = arrayTypeLen(n.anc)
			default:
				if sym, _, ok := sc.lookup(n.child[0].ident); ok {
					// Resolve symbol to get size value
					if sym.typ != nil && sym.typ.cat == intT {
						if v, ok := sym.rval.Interface().(int); ok {
							t.size = v
						} else {
							t.incomplete = true
						}
					} else {
						t.incomplete = true
					}
				} else {
					// Evaluate constant array size expression
					if _, err = interp.cfg(n.child[0], sc.pkgID); err != nil {
						return nil, err
					}
					t.incomplete = true
				}
			}
			if t.val, err = nodeType(interp, sc, n.child[1]); err != nil {
				return nil, err
			}
			t.sizedef = true
			t.incomplete = t.incomplete || t.val.incomplete
		} else {
			if t.val, err = nodeType(interp, sc, n.child[0]); err != nil {
				return nil, err
			}
			t.incomplete = t.val.incomplete
		}

	case basicLit:
		switch v := n.rval.Interface().(type) {
		case bool:
			t.cat = boolT
			t.name = "bool"
		case byte:
			t.cat = uint8T
			t.name = "uint8"
			t.untyped = true
		case complex64:
			t.cat = complex64T
			t.name = "complex64"
		case complex128:
			t.cat = complex128T
			t.name = "complex128"
			t.untyped = true
		case float32:
			t.cat = float32T
			t.name = "float32"
			t.untyped = true
		case float64:
			t.cat = float64T
			t.name = "float64"
			t.untyped = true
		case int:
			t.cat = intT
			t.name = "int"
			t.untyped = true
		case uint:
			t.cat = uintT
			t.name = "uint"
			t.untyped = true
		case rune:
			t.cat = int32T
			t.name = "int32"
			t.untyped = true
		case string:
			t.cat = stringT
			t.name = "string"
			t.untyped = true
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
			case "complex":
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
						t = &itype{cat: valueT, rtype: complexType, scope: sc}
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
			case "real", "imag":
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
			case "cap", "copy", "len":
				t = sc.getType("int")
			case "append", "make":
				t, err = nodeType(interp, sc, n.child[1])
			case "new":
				t, err = nodeType(interp, sc, n.child[1])
				t = &itype{cat: ptrT, val: t, incomplete: t.incomplete, scope: sc}
			case "recover":
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
		if sym, _, found := sc.lookup(n.ident); found {
			t = sym.typ
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
		} else {
			t.incomplete = true
			sc.sym[n.ident] = &symbol{kind: typeSym, typ: t}
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
		if lt, err = nodeType(interp, sc, n.child[0]); err != nil {
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
				if isBinType(v) { // a bin type is encoded as a pointer on nil value
					t.rtype = t.rtype.Elem()
				}
			} else {
				err = n.cfgErrorf("undefined selector %s.%s", lt.path, name)
				panic(err)
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
				t = &itype{cat: valueT, rtype: bm.Type, isBinMethod: true, scope: sc}
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
				tag := c.child[1].rval.String()
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
					tag = c.lastChild().rval.String()
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

// struct name returns the name of a struct type
func typeName(n *node) string {
	if n.anc.kind == typeSpec {
		return n.anc.child[0].ident
	}
	return ""
}

// fieldName returns an implicit struct field name according to node kind
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
// full type name. It allows to asses a type recursive status.
func (t *itype) referTo(name string, seen map[*itype]bool) bool {
	if t.path+"/"+t.name == name {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.cat {
	case aliasT, arrayT, chanT, ptrT:
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

// isComplete returns true if type definition is complete.
func (t *itype) isComplete() bool { return isComplete(t, map[string]bool{}) }

func isComplete(t *itype, visited map[string]bool) bool {
	if t.incomplete {
		return false
	}
	name := t.path + "/" + t.name
	if visited[name] {
		return !t.incomplete
	}
	if t.name != "" {
		visited[name] = true
	}
	switch t.cat {
	case aliasT, arrayT, chanT, ptrT:
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
	res := make(methodSet)
	switch t.cat {
	case interfaceT:
		// Get methods from recursive analysis of interface fields
		for _, f := range t.field {
			if f.typ.cat == funcT {
				res[f.name] = f.typ.TypeOf().String()
			} else {
				for k, v := range f.typ.methods() {
					res[k] = v
				}
			}
		}
	case valueT, errorT:
		// Get method from corresponding reflect.Type
		for i := t.rtype.NumMethod() - 1; i >= 0; i-- {
			m := t.rtype.Method(i)
			res[m.Name] = m.Type.String()
		}
	case ptrT:
		// Consider only methods where receiver is a pointer to type t
		for _, m := range t.val.method {
			if m.child[0].child[0].lastChild().typ.cat == ptrT {
				res[m.ident] = m.typ.TypeOf().String()
			}
		}
		for k, v := range t.val.methods() {
			res[k] = v
		}
	case structT:
		for _, f := range t.field {
			for k, v := range f.typ.methods() {
				res[k] = v
			}
		}
	}
	for _, m := range t.method {
		res[m.ident] = m.typ.TypeOf().String()
	}
	return res
}

// id returns a unique type identificator string
func (t *itype) id() (res string) {
	if t.name != "" {
		return t.path + "." + t.name
	}
	switch t.cat {
	case arrayT:
		res = "[" + strconv.Itoa(t.size) + "]" + t.val.id()
	case chanT:
		res = "<-" + t.val.id()
	case funcT:
		res = "func("
		for _, t := range t.arg {
			res += t.id() + ","
		}
		res += ")("
		for _, t := range t.ret {
			res += t.id() + ","
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
		res = t.rtype.PkgPath() + "." + t.rtype.Name()
	}
	return res
}

// zero instantiates and return a zero value object for the given type during execution
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

// fieldIndex returns the field index from name in a struct, or -1 if not found
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

// fieldSeq returns the field type from the list of field indexes
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

// lookupField returns a list of indices, i.e. a path to access a field in a struct object
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

// lookupBinField returns a structfield and a path to access an embedded binary field in a struct object
func (t *itype) lookupBinField(name string) (s reflect.StructField, index []int, ok bool) {
	if t.cat == ptrT {
		return t.val.lookupBinField(name)
	}
	if !isStruct(t) {
		return
	}
	s, ok = t.TypeOf().FieldByName(name)
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
func (t *itype) lookupBinMethod(name string) (m reflect.Method, index []int, isPtr bool, ok bool) {
	if t.cat == ptrT {
		return t.val.lookupBinMethod(name)
	}
	m, ok = t.TypeOf().MethodByName(name)
	if !ok {
		m, ok = reflect.PtrTo(t.TypeOf()).MethodByName(name)
		isPtr = ok
	}
	if !ok {
		for i, f := range t.field {
			if f.embed {
				if m2, index2, isPtr2, ok2 := f.typ.lookupBinMethod(name); ok2 {
					index = append([]int{i}, index2...)
					return m2, index, isPtr2, ok2
				}
			}
		}
	}
	return m, index, isPtr, ok
}

func exportName(s string) string {
	if canExport(s) {
		return s
	}
	return "X" + s
}

var interf = reflect.TypeOf((*interface{})(nil)).Elem()

// RefType returns a reflect.Type representation from an interpereter type.
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
	if t.val != nil && defined[t.val.path+"/"+t.val.name] != nil && t.val.rtype == nil {
		// Replace reference to self (direct or indirect) by an interface{} to handle
		// recursive types with reflect.
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
	case errorT:
		t.rtype = reflect.TypeOf(new(error)).Elem()
	case funcT:
		if t.name != "" {
			defined[name] = t
		}
		in := make([]reflect.Type, len(t.arg))
		out := make([]reflect.Type, len(t.ret))
		for i, v := range t.arg {
			in[i] = v.refType(defined, true)
		}
		for i, v := range t.ret {
			out[i] = v.refType(defined, true)
		}
		t.rtype = reflect.FuncOf(in, out, false)
	case interfaceT:
		t.rtype = interf
	case mapT:
		t.rtype = reflect.MapOf(t.key.refType(defined, wrapRecursive), t.val.refType(defined, wrapRecursive))
	case ptrT:
		t.rtype = reflect.PtrTo(t.val.refType(defined, wrapRecursive))
	case structT:
		if t.name != "" {
			if defined[name] != nil {
				recursive = true
			}
			defined[name] = t
		}
		var fields []reflect.StructField
		for _, f := range t.field {
			field := reflect.StructField{Name: exportName(f.name), Type: f.typ.refType(defined, wrapRecursive), Tag: reflect.StructTag(f.tag)}
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
		r = reflect.TypeOf((*valueInterface)(nil)).Elem()
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

func defRecvType(n *node) *itype {
	if n.kind != funcDecl || len(n.child[0].child) == 0 {
		return nil
	}
	if r := n.child[0].child[0].lastChild(); r != nil {
		return r.typ
	}
	return nil
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
	case chanT:
		return t.val
	case valueT:
		return &itype{cat: valueT, rtype: t.rtype.Elem(), node: t.node, scope: t.scope}
	}
	return nil
}

func isBool(t *itype) bool { return t.TypeOf().Kind() == reflect.Bool }
func isChan(t *itype) bool { return t.TypeOf().Kind() == reflect.Chan }
func isMap(t *itype) bool  { return t.TypeOf().Kind() == reflect.Map }

func isInterfaceSrc(t *itype) bool {
	return t.cat == interfaceT || (t.cat == aliasT && isInterfaceSrc(t.val))
}

func isInterface(t *itype) bool {
	return isInterfaceSrc(t) || t.TypeOf().Kind() == reflect.Interface
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
func isNumber(t reflect.Type) bool  { return isInt(t) || isFloat(t) || isComplex(t) }
func isString(t reflect.Type) bool  { return t != nil && t.Kind() == reflect.String }
