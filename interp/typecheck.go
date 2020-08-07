package interp

import (
	"errors"
	"go/constant"
	"math"
	"reflect"
)

type opPredicates map[action]func(reflect.Type) bool

// typecheck handles all type checking following "go/types" logic.
//
// Due to variant type systems (itype vs reflect.Type) a single
// type system should used, namely reflect.Type with exception
// of the untyped flag on itype.
type typecheck struct{}

// op type checks an expression against a set of expression predicates.
func (check typecheck) op(p opPredicates, a action, n, c *node, t reflect.Type) error {
	if pred := p[a]; pred != nil {
		if !pred(t) {
			return n.cfgErrorf("invalid operation: operator %v not defined on %s", n.action, c.typ.id())
		}
	} else {
		return n.cfgErrorf("invalid operation: unknown operator %v", n.action)
	}
	return nil
}

// assignment checks if n can be assigned to typ.
//
// Use typ == nil to indicate assignment to an untyped blank identifier.
func (check typecheck) assignment(n *node, typ *itype, context string) error {
	if n.typ.untyped {
		if typ == nil || isInterface(typ) {
			if typ == nil && n.typ.cat == nilT {
				return n.cfgErrorf("use of untyped nil in %s", context)
			}
			typ = n.typ.defaultType()
		}
		if err := check.convertUntyped(n, typ); err != nil {
			return err
		}
	}

	if typ == nil {
		return nil
	}

	if !n.typ.assignableTo(typ) {
		if context == "" {
			return n.cfgErrorf("cannot use type %s as type %s", n.typ.id(), typ.id())
		}
		return n.cfgErrorf("cannot use type %s as type %s in %s", n.typ.id(), typ.id(), context)
	}
	return nil
}

// assignExpr type checks an assign expression.
//
// This is done per pair of assignments.
func (check typecheck) assignExpr(n, dest, src *node) error {
	if n.action == aAssign {
		isConst := n.anc.kind == constDecl
		if !isConst {
			// var operations must be typed
			dest.typ = dest.typ.defaultType()
		}

		return check.assignment(src, dest.typ, "assignment")
	}

	// assignment operations.
	if n.nleft > 1 || n.nright > 1 {
		return n.cfgErrorf("assignment operation %s requires single-valued expressions", n.action)
	}

	return check.binaryExpr(n)
}

// addressExpr type checks a unary address expression.
func (check typecheck) addressExpr(n *node) error {
	c0 := n.child[0]
	found := false
	for !found {
		switch c0.kind {
		case parenExpr:
			c0 = c0.child[0]
			continue
		case selectorExpr:
			c0 = c0.child[1]
			continue
		case indexExpr:
			c := c0.child[0]
			if isArray(c.typ) || isMap(c.typ) {
				c0 = c
				continue
			}
		case compositeLitExpr, identExpr:
			found = true
			continue
		}
		return n.cfgErrorf("invalid operation: cannot take address of %s", c0.typ.id())
	}
	return nil
}

var unaryOpPredicates = opPredicates{
	aPos:    isNumber,
	aNeg:    isNumber,
	aBitNot: isInt,
	aNot:    isBoolean,
}

// unaryExpr type checks a unary expression.
func (check typecheck) unaryExpr(n *node) error {
	c0 := n.child[0]
	t0 := c0.typ.TypeOf()

	if n.action == aRecv {
		if !isChan(c0.typ) {
			return n.cfgErrorf("invalid operation: cannot receive from non-channel %s", c0.typ.id())
		}
		if isSendChan(c0.typ) {
			return n.cfgErrorf("invalid operation: cannot receive from send-only channel %s", c0.typ.id())
		}
		return nil
	}

	if err := check.op(unaryOpPredicates, n.action, n, c0, t0); err != nil {
		return err
	}
	return nil
}

// shift type checks a shift binary expression.
func (check typecheck) shift(n *node) error {
	c0, c1 := n.child[0], n.child[1]
	t0, t1 := c0.typ.TypeOf(), c1.typ.TypeOf()

	var v0 constant.Value
	if c0.typ.untyped {
		v0 = constant.ToInt(c0.rval.Interface().(constant.Value))
		c0.rval = reflect.ValueOf(v0)
	}

	if !(c0.typ.untyped && v0 != nil && v0.Kind() == constant.Int || isInt(t0)) {
		return n.cfgErrorf("invalid operation: shift of type %v", c0.typ.id())
	}

	switch {
	case c1.typ.untyped:
		if err := check.convertUntyped(c1, &itype{cat: uintT, name: "uint"}); err != nil {
			return n.cfgErrorf("invalid operation: shift count type %v, must be integer", c1.typ.id())
		}
	case isInt(t1):
		// nothing to do
	default:
		return n.cfgErrorf("invalid operation: shift count type %v, must be integer", c1.typ.id())
	}
	return nil
}

// comparison type checks a comparison binary expression.
func (check typecheck) comparison(n *node) error {
	c0, c1 := n.child[0], n.child[1]

	if !c0.typ.assignableTo(c1.typ) && !c1.typ.assignableTo(c0.typ) {
		return n.cfgErrorf("invalid operation: mismatched types %s and %s", c0.typ.id(), c1.typ.id())
	}

	ok := false
	switch n.action {
	case aEqual, aNotEqual:
		ok = c0.typ.comparable() && c1.typ.comparable() || c0.typ.isNil() && c1.typ.hasNil() || c1.typ.isNil() && c0.typ.hasNil()
	case aLower, aLowerEqual, aGreater, aGreaterEqual:
		ok = c0.typ.ordered() && c1.typ.ordered()
	}
	if !ok {
		typ := c0.typ
		if typ.isNil() {
			typ = c1.typ
		}
		return n.cfgErrorf("invalid operation: operator %v not defined on %s", n.action, typ.id(), ".")
	}
	return nil
}

var binaryOpPredicates = opPredicates{
	aAdd: func(typ reflect.Type) bool { return isNumber(typ) || isString(typ) },
	aSub: isNumber,
	aMul: isNumber,
	aQuo: isNumber,
	aRem: isInt,

	aAnd:    isInt,
	aOr:     isInt,
	aXor:    isInt,
	aAndNot: isInt,

	aLand: isBoolean,
	aLor:  isBoolean,
}

// binaryExpr type checks a binary expression.
func (check typecheck) binaryExpr(n *node) error {
	c0, c1 := n.child[0], n.child[1]
	a := n.action
	if isAssignAction(a) {
		a--
	}

	if isShiftAction(a) {
		return check.shift(n)
	}

	_ = check.convertUntyped(c0, c1.typ)
	_ = check.convertUntyped(c1, c0.typ)

	if isComparisonAction(a) {
		return check.comparison(n)
	}

	if !c0.typ.equals(c1.typ) {
		return n.cfgErrorf("invalid operation: mismatched types %s and %s", c0.typ.id(), c1.typ.id())
	}

	t0 := c0.typ.TypeOf()
	if err := check.op(binaryOpPredicates, a, n, c0, t0); err != nil {
		return err
	}

	switch n.action {
	case aQuo, aRem:
		if (c0.typ.untyped || isInt(t0)) && c1.typ.untyped && constant.Sign(c1.rval.Interface().(constant.Value)) == 0 {
			return n.cfgErrorf("invalid operation: division by zero")
		}
	}
	return nil
}

func (check typecheck) index(n *node, max int) error {
	if err := check.convertUntyped(n, &itype{cat: intT, name: "int"}); err != nil {
		return err
	}

	if !isInt(n.typ.TypeOf()) {
		return n.cfgErrorf("index %s must be integer", n.typ.id())
	}

	if !n.rval.IsValid() || max < 1 {
		return nil
	}

	if int(vInt(n.rval)) >= max {
		return n.cfgErrorf("index %s is out of bounds", n.typ.id())
	}

	return nil
}

// arrayLitExpr type checks an array composite literal expression.
func (check typecheck) arrayLitExpr(child []*node, typ *itype, length int) error {
	visited := make(map[int]bool, len(child))
	index := 0
	for _, c := range child {
		n := c
		switch {
		case c.kind == keyValueExpr:
			if err := check.index(c.child[0], length); err != nil {
				return c.cfgErrorf("index %s must be integer constant", c.child[0].typ.id())
			}
			n = c.child[1]
			index = int(vInt(c.child[0].rval))
		case length > 0 && index >= length:
			return c.cfgErrorf("index %d is out of bounds (>= %d)", index, length)
		}

		if visited[index] {
			return n.cfgErrorf("duplicate index %d in array or slice literal", index)
		}
		visited[index] = true
		index++

		if err := check.assignment(n, typ, "array or slice literal"); err != nil {
			return err
		}
	}
	return nil
}

// mapLitExpr type checks an map composite literal expression.
func (check typecheck) mapLitExpr(child []*node, ktyp, vtyp *itype) error {
	visited := make(map[interface{}]bool, len(child))
	for _, c := range child {
		if c.kind != keyValueExpr {
			return c.cfgErrorf("missing key in map literal")
		}

		key, val := c.child[0], c.child[1]
		if err := check.assignment(key, ktyp, "map literal"); err != nil {
			return err
		}

		if key.rval.IsValid() {
			kval := key.rval.Interface()
			if visited[kval] {
				return c.cfgErrorf("duplicate key %s in map literal", kval)
			}
			visited[kval] = true
		}

		if err := check.assignment(val, vtyp, "map literal"); err != nil {
			return err
		}
	}
	return nil
}

// structLitExpr type checks an struct composite literal expression.
func (check typecheck) structLitExpr(child []*node, typ *itype) error {
	if len(child) == 0 {
		return nil
	}

	if child[0].kind == keyValueExpr {
		// All children must be keyValueExpr
		visited := make([]bool, len(typ.field))
		for _, c := range child {
			if c.kind != keyValueExpr {
				return c.cfgErrorf("mixture of field:value and value elements in struct literal")
			}

			key, val := c.child[0], c.child[1]
			name := key.ident
			if name == "" {
				return c.cfgErrorf("invalid field name %s in struct literal", key.typ.id())
			}
			i := typ.fieldIndex(name)
			if i < 0 {
				return c.cfgErrorf("unknown field %s in struct literal", name)
			}
			field := typ.field[i]

			if err := check.assignment(val, field.typ, "struct literal"); err != nil {
				return err
			}

			if visited[i] {
				return c.cfgErrorf("duplicate field name %s in struct literal", name)
			}
			visited[i] = true
		}
		return nil
	}

	// No children can be keyValueExpr
	for i, c := range child {
		if c.kind == keyValueExpr {
			return c.cfgErrorf("mixture of field:value and value elements in struct literal")
		}

		if i >= len(typ.field) {
			return c.cfgErrorf("too many values in struct literal")
		}
		field := typ.field[i]
		// TODO(nick): check if this field is not exported and in a different package.

		if err := check.assignment(c, field.typ, "struct literal"); err != nil {
			return err
		}
	}
	if len(child) < len(typ.field) {
		return child[len(child)-1].cfgErrorf("too few values in struct literal")
	}
	return nil
}

// structBinLitExpr type checks an struct composite literal expression on a binary type.
func (check typecheck) structBinLitExpr(child []*node, typ reflect.Type) error {
	if len(child) == 0 {
		return nil
	}

	if child[0].kind == keyValueExpr {
		// All children must be keyValueExpr
		visited := make(map[string]bool, typ.NumField())
		for _, c := range child {
			if c.kind != keyValueExpr {
				return c.cfgErrorf("mixture of field:value and value elements in struct literal")
			}

			key, val := c.child[0], c.child[1]
			name := key.ident
			if name == "" {
				return c.cfgErrorf("invalid field name %s in struct literal", key.typ.id())
			}
			field, ok := typ.FieldByName(name)
			if !ok {
				return c.cfgErrorf("unknown field %s in struct literal", name)
			}

			if err := check.assignment(val, &itype{cat: valueT, rtype: field.Type}, "struct literal"); err != nil {
				return err
			}

			if visited[field.Name] {
				return c.cfgErrorf("duplicate field name %s in struct literal", name)
			}
			visited[field.Name] = true
		}
		return nil
	}

	// No children can be keyValueExpr
	for i, c := range child {
		if c.kind == keyValueExpr {
			return c.cfgErrorf("mixture of field:value and value elements in struct literal")
		}

		if i >= typ.NumField() {
			return c.cfgErrorf("too many values in struct literal")
		}
		field := typ.Field(i)
		if !canExport(field.Name) {
			return c.cfgErrorf("implicit assignment to unexported field %s in %s literal", field.Name, typ)
		}

		if err := check.assignment(c, &itype{cat: valueT, rtype: field.Type}, "struct literal"); err != nil {
			return err
		}
	}
	if len(child) < typ.NumField() {
		return child[len(child)-1].cfgErrorf("too few values in struct literal")
	}
	return nil
}

// conversion type checks the conversion of n to typ.
func (check typecheck) conversion(n *node, typ *itype) error {
	var c constant.Value
	if n.rval.IsValid() {
		if con, ok := n.rval.Interface().(constant.Value); ok {
			c = con
		}
	}

	var ok bool
	switch {
	case c != nil && isConstType(typ):
		switch t := typ.TypeOf(); {
		case representableConst(c, t):
			ok = true
		case isInt(n.typ.TypeOf()) && isString(t):
			codepoint := int64(-1)
			if i, ok := constant.Int64Val(c); ok {
				codepoint = i
			}
			n.rval = reflect.ValueOf(constant.MakeString(string(codepoint)))
			ok = true
		}

	case n.typ.convertibleTo(typ):
		ok = true
	}
	if !ok {
		return n.cfgErrorf("cannot convert expression of type %s to type %s", n.typ.id(), typ.id())
	}

	if n.typ.untyped {
		if isInterface(typ) || c != nil && !isConstType(typ) {
			typ = n.typ.defaultType()
		}
		if err := check.convertUntyped(n, typ); err != nil {
			return err
		}
	}
	return nil
}

// arguments type checks the call expression arguments.
func (check typecheck) arguments(n *node, child []*node, fun *node, ellipsis bool) error {
	l := len(child)
	if ellipsis {
		if !fun.typ.isVariadic() {
			return n.cfgErrorf("invalid use of ..., corresponding parameter is non-variadic")
		}
		if len(child) == 1 && isCall(child[0]) && child[0].child[0].typ.numOut() > 1 {
			return child[0].cfgErrorf("cannot use ... with %d-valued %s", child[0].child[0].typ.numOut(), child[0].child[0].typ.id())
		}
	}

	if len(child) == 1 && isCall(child[0]) && child[0].child[0].typ.numOut() > 1 {
		// Handle the case of unpacking a n-valued function into the params.
		c := child[0].child[0]
		l := c.typ.numOut()
		if l < fun.typ.numIn() {
			return child[0].cfgErrorf("not enough arguments in call to %s", fun.name())
		}
		for i := 0; i < l; i++ {
			arg := getArg(fun.typ, i)
			if arg == nil {
				return child[0].cfgErrorf("too many arguments")
			}
			if !c.typ.out(i).assignableTo(arg) {
				return child[0].cfgErrorf("cannot use %s as type %s", c.typ.id(), getArgsID(fun.typ))
			}
		}
		return nil
	}

	var cnt int
	if fun.kind == selectorExpr && fun.typ.cat == valueT && fun.recv != nil && !isInterface(fun.recv.node.typ) {
		// If this is a bin call, and we have a receiver and the receiver is
		// not an interface, then the first input is the receiver, so skip it.
		cnt++
	}
	for _, arg := range child {
		ellip := cnt == l-1 && ellipsis
		if err := check.argument(arg, fun.typ, cnt, ellip); err != nil {
			return err
		}
		cnt++
	}

	if fun.typ.isVariadic() {
		cnt++
	}
	if cnt < fun.typ.numIn() {
		return n.cfgErrorf("not enough arguments in call to %s", fun.name())
	}
	return nil
}

func (check typecheck) argument(n *node, ftyp *itype, i int, ellipsis bool) error {
	typ := getArg(ftyp, i)
	if typ == nil {
		return n.cfgErrorf("too many arguments")
	}

	if isCall(n) && n.child[0].typ.numOut() != 1 {
		return n.cfgErrorf("cannot use %s as type %s", n.child[0].typ.id(), typ.id())
	}

	if ellipsis {
		if i != ftyp.numIn()-1 {
			return n.cfgErrorf("can only use ... with matching parameter")
		}
		t := n.typ.TypeOf()
		if t.Kind() != reflect.Slice || !(&itype{cat: valueT, rtype: t.Elem()}).assignableTo(typ) {
			return n.cfgErrorf("cannot use %s as type %s", n.typ.id(), (&itype{cat: arrayT, val: typ}).id())
		}
		return nil
	}

	err := check.assignment(n, typ, "")
	return err
}

func getArg(ftyp *itype, i int) *itype {
	l := ftyp.numIn()
	switch {
	case ftyp.isVariadic() && i >= l-1:
		arg := ftyp.in(l - 1).val
		return arg
	case i < l:
		return ftyp.in(i)
	default:
		return nil
	}
}

func getArgsID(ftyp *itype) string {
	res := "("
	for i, arg := range ftyp.arg {
		if i > 0 {
			res += ","
		}
		res += arg.id()
	}
	res += ")"
	return res
}

var errCantConvert = errors.New("cannot convert")

func (check typecheck) convertUntyped(n *node, typ *itype) error {
	if n.typ == nil || !n.typ.untyped || typ == nil {
		return nil
	}

	convErr := n.cfgErrorf("cannot convert %s to %s", n.typ.id(), typ.id())

	ntyp, ttyp := n.typ.TypeOf(), typ.TypeOf()
	if typ.untyped {
		// Both n and target are untyped.
		nkind, tkind := ntyp.Kind(), ttyp.Kind()
		if isNumber(ntyp) && isNumber(ttyp) {
			if nkind < tkind {
				n.typ = typ
			}
		} else if nkind != tkind {
			return convErr
		}
		return nil
	}

	var (
		ityp *itype
		rtyp reflect.Type
		err  error
	)
	switch {
	case typ.isNil() && n.typ.isNil():
		n.typ = typ
		return nil
	case isNumber(ttyp) || isString(ttyp) || isBoolean(ttyp):
		ityp = typ
		rtyp = ttyp
	case isInterface(typ):
		if n.typ.isNil() {
			return nil
		}
		if len(n.typ.methods()) > 0 { // untyped cannot be set to iface
			return convErr
		}
		ityp = n.typ.defaultType()
		rtyp = ntyp

	case isArray(typ) || isMap(typ) || isChan(typ) || isFunc(typ) || isPtr(typ):
		// TODO(nick): above we are acting on itype, but really it is an rtype check. This is not clear which type
		// 		 	   plain we are in. Fix this later.
		if !n.typ.isNil() {
			return convErr
		}
		return nil
	default:
		return convErr
	}

	if err := check.representable(n, rtyp); err != nil {
		return err
	}
	n.rval, err = check.convertConst(n.rval, rtyp)
	if err != nil {
		if errors.Is(err, errCantConvert) {
			return convErr
		}
		return n.cfgErrorf(err.Error())
	}
	n.typ = ityp
	return nil
}

func (check typecheck) representable(n *node, t reflect.Type) error {
	if !n.rval.IsValid() {
		// TODO(nick): This should be an error as the const is in the frame which is undesirable.
		return nil
	}
	c, ok := n.rval.Interface().(constant.Value)
	if !ok {
		// TODO(nick): This should be an error as untyped strings and bools should be constant.Values.
		return nil
	}

	if !representableConst(c, t) {
		typ := n.typ.TypeOf()
		if isNumber(typ) && isNumber(t) {
			// numeric conversion : error msg
			//
			// integer -> integer : overflows
			// integer -> float   : overflows (actually not possible)
			// float   -> integer : truncated
			// float   -> float   : overflows
			//
			if !isInt(typ) && isInt(t) {
				return n.cfgErrorf("%s truncated to %s", c.ExactString(), t.Kind().String())
			}
			return n.cfgErrorf("%s overflows %s", c.ExactString(), t.Kind().String())
		}
		return n.cfgErrorf("cannot convert %s to %s", c.ExactString(), t.Kind().String())
	}
	return nil
}

func (check typecheck) convertConst(v reflect.Value, t reflect.Type) (reflect.Value, error) {
	if !v.IsValid() {
		// TODO(nick): This should be an error as the const is in the frame which is undesirable.
		return v, nil
	}
	c, ok := v.Interface().(constant.Value)
	if !ok {
		// TODO(nick): This should be an error as untyped strings and bools should be constant.Values.
		return v, nil
	}

	kind := t.Kind()
	switch kind {
	case reflect.Bool:
		v = reflect.ValueOf(constant.BoolVal(c))
	case reflect.String:
		v = reflect.ValueOf(constant.StringVal(c))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, _ := constant.Int64Val(constant.ToInt(c))
		v = reflect.ValueOf(i).Convert(t)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		i, _ := constant.Uint64Val(constant.ToInt(c))
		v = reflect.ValueOf(i).Convert(t)
	case reflect.Float32:
		f, _ := constant.Float32Val(constant.ToFloat(c))
		v = reflect.ValueOf(f)
	case reflect.Float64:
		f, _ := constant.Float64Val(constant.ToFloat(c))
		v = reflect.ValueOf(f)
	case reflect.Complex64:
		r, _ := constant.Float32Val(constant.Real(c))
		i, _ := constant.Float32Val(constant.Imag(c))
		v = reflect.ValueOf(complex(r, i)).Convert(t)
	case reflect.Complex128:
		r, _ := constant.Float64Val(constant.Real(c))
		i, _ := constant.Float64Val(constant.Imag(c))
		v = reflect.ValueOf(complex(r, i)).Convert(t)
	default:
		return v, errCantConvert
	}
	return v, nil
}

var bitlen = [...]int{
	reflect.Int:     64,
	reflect.Int8:    8,
	reflect.Int16:   16,
	reflect.Int32:   32,
	reflect.Int64:   64,
	reflect.Uint:    64,
	reflect.Uint8:   8,
	reflect.Uint16:  16,
	reflect.Uint32:  32,
	reflect.Uint64:  64,
	reflect.Uintptr: 64,
}

func representableConst(c constant.Value, t reflect.Type) bool {
	switch {
	case isInt(t):
		x := constant.ToInt(c)
		if x.Kind() != constant.Int {
			return false
		}
		switch t.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if _, ok := constant.Int64Val(x); !ok {
				return false
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			if _, ok := constant.Uint64Val(x); !ok {
				return false
			}
		default:
			return false
		}
		return constant.BitLen(x) <= bitlen[t.Kind()]
	case isFloat(t):
		x := constant.ToFloat(c)
		if x.Kind() != constant.Float {
			return false
		}
		switch t.Kind() {
		case reflect.Float32:
			f, _ := constant.Float32Val(x)
			return !math.IsInf(float64(f), 0)
		case reflect.Float64:
			f, _ := constant.Float64Val(x)
			return !math.IsInf(f, 0)
		default:
			return false
		}
	case isComplex(t):
		x := constant.ToComplex(c)
		if x.Kind() != constant.Complex {
			return false
		}
		switch t.Kind() {
		case reflect.Complex64:
			r, _ := constant.Float32Val(constant.Real(x))
			i, _ := constant.Float32Val(constant.Imag(x))
			return !math.IsInf(float64(r), 0) && !math.IsInf(float64(i), 0)
		case reflect.Complex128:
			r, _ := constant.Float64Val(constant.Real(x))
			i, _ := constant.Float64Val(constant.Imag(x))
			return !math.IsInf(r, 0) && !math.IsInf(i, 0)
		default:
			return false
		}
	case isString(t):
		return c.Kind() == constant.String
	case isBoolean(t):
		return c.Kind() == constant.Bool
	default:
		return false
	}
}

func isShiftAction(a action) bool {
	switch a {
	case aShl, aShr, aShlAssign, aShrAssign:
		return true
	}
	return false
}

func isComparisonAction(a action) bool {
	switch a {
	case aEqual, aNotEqual, aGreater, aGreaterEqual, aLower, aLowerEqual:
		return true
	}
	return false
}
