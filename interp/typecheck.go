package interp

import (
	"errors"
	"go/constant"
	"go/token"
	"math"
	"reflect"
)

type opPredicates map[action]func(reflect.Type) bool

// typecheck handles all type checking following "go/types" logic.
//
// Due to variant type systems (itype vs reflect.Type) a single
// type system should used, namely reflect.Type with exception
// of the untyped flag on itype.
type typecheck struct {
	scope *scope
}

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
	if n.typ == nil {
		return n.cfgErrorf("invalid type in %s", context)
	}
	if n.typ.untyped {
		if typ == nil || isInterface(typ) {
			if typ == nil && n.typ.cat == nilT {
				return n.cfgErrorf("use of untyped nil in %s", context)
			}
			typ = n.typ.defaultType(n.rval, check.scope)
		}
		if err := check.convertUntyped(n, typ); err != nil {
			return err
		}
	}

	if typ == nil {
		return nil
	}

	if !n.typ.assignableTo(typ) && typ.str != "*unsafe2.dummy" {
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
			dest.typ = dest.typ.defaultType(src.rval, check.scope)
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
		case starExpr:
			c0 = c0.child[0]
			continue
		case indexExpr, sliceExpr:
			c := c0.child[0]
			if isArray(c.typ) || isMap(c.typ) {
				c0 = c
				found = true
				continue
			}
		case compositeLitExpr, identExpr:
			found = true
			continue
		}
		return n.cfgErrorf("invalid operation: cannot take address of %s [kind: %s]", c0.typ.id(), kinds[c0.kind])
	}
	return nil
}

// starExpr type checks a star expression on a variable.
func (check typecheck) starExpr(n *node) error {
	if n.typ.TypeOf().Kind() != reflect.Ptr {
		return n.cfgErrorf("invalid operation: cannot indirect %q", n.name())
	}
	return nil
}

var unaryOpPredicates = opPredicates{
	aInc:    isNumber,
	aDec:    isNumber,
	aPos:    isNumber,
	aNeg:    isNumber,
	aBitNot: isInt,
	aNot:    isBoolean,
}

// unaryExpr type checks a unary expression.
func (check typecheck) unaryExpr(n *node) error {
	c0 := n.child[0]
	if isBlank(c0) {
		return n.cfgErrorf("cannot use _ as value")
	}
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

	return check.op(unaryOpPredicates, n.action, n, c0, t0)
}

// shift type checks a shift binary expression.
func (check typecheck) shift(n *node) error {
	c0, c1 := n.child[0], n.child[1]
	t0, t1 := c0.typ.TypeOf(), c1.typ.TypeOf()

	var v0 constant.Value
	if c0.typ.untyped && c0.rval.IsValid() {
		v0 = constant.ToInt(c0.rval.Interface().(constant.Value))
		c0.rval = reflect.ValueOf(v0)
	}

	if !(c0.typ.untyped && v0 != nil && v0.Kind() == constant.Int || isInt(t0)) {
		return n.cfgErrorf("invalid operation: shift of type %v", c0.typ.id())
	}

	switch {
	case c1.typ.untyped:
		if err := check.convertUntyped(c1, check.scope.getType("uint")); err != nil {
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
	t0, t1 := n.child[0].typ, n.child[1].typ

	if !t0.assignableTo(t1) && !t1.assignableTo(t0) {
		return n.cfgErrorf("invalid operation: mismatched types %s and %s", t0.id(), t1.id())
	}

	ok := false

	if !isInterface(t0) && !isInterface(t1) && !t0.isNil() && !t1.isNil() && t0.untyped == t1.untyped && t0.id() != t1.id() && !typeDefined(t0, t1) {
		// Non interface types must be really equals.
		return n.cfgErrorf("invalid operation: mismatched types %s and %s", t0.id(), t1.id())
	}

	switch n.action {
	case aEqual, aNotEqual:
		ok = t0.comparable() && t1.comparable() || t0.isNil() && t1.hasNil() || t1.isNil() && t0.hasNil()
	case aLower, aLowerEqual, aGreater, aGreaterEqual:
		ok = t0.ordered() && t1.ordered()
	}
	if !ok {
		typ := t0
		if typ.isNil() {
			typ = t1
		}
		return n.cfgErrorf("invalid operation: operator %v not defined on %s", n.action, typ.id())
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

	if isBlank(c0) || isBlank(c1) {
		return n.cfgErrorf("cannot use _ as value")
	}

	a := n.action
	if isAssignAction(a) {
		a--
	}

	if isShiftAction(a) {
		return check.shift(n)
	}

	switch n.action {
	case aAdd:
		if n.typ == nil {
			break
		}
		// Catch mixing string and number for "+" operator use.
		k, k0, k1 := isNumber(n.typ.TypeOf()), isNumber(c0.typ.TypeOf()), isNumber(c1.typ.TypeOf())
		if k != k0 || k != k1 {
			return n.cfgErrorf("cannot use type %s as type %s in assignment", c0.typ.id(), n.typ.id())
		}
	case aRem:
		if zeroConst(c1) {
			return n.cfgErrorf("invalid operation: division by zero")
		}
	case aQuo:
		if zeroConst(c1) {
			return n.cfgErrorf("invalid operation: division by zero")
		}
		if c0.rval.IsValid() && c1.rval.IsValid() {
			// Avoid constant conversions below to ensure correct constant integer quotient.
			return nil
		}
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

	return check.op(binaryOpPredicates, a, n, c0, t0)
}

func zeroConst(n *node) bool {
	return n.typ.untyped && constant.Sign(n.rval.Interface().(constant.Value)) == 0
}

func (check typecheck) index(n *node, max int) error {
	if err := check.convertUntyped(n, check.scope.getType("int")); err != nil {
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
func (check typecheck) arrayLitExpr(child []*node, typ *itype) error {
	cat := typ.cat
	length := typ.length
	typ = typ.val
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
		case cat == arrayT && index >= length:
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

// structLitExpr type checks a struct composite literal expression.
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

// structBinLitExpr type checks a struct composite literal expression on a binary type.
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

			if err := check.assignment(val, valueTOf(field.Type), "struct literal"); err != nil {
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

		if err := check.assignment(c, valueTOf(field.Type), "struct literal"); err != nil {
			return err
		}
	}
	if len(child) < typ.NumField() {
		return child[len(child)-1].cfgErrorf("too few values in struct literal")
	}
	return nil
}

// sliceExpr type checks a slice expression.
func (check typecheck) sliceExpr(n *node) error {
	for _, c := range n.child {
		if isBlank(c) {
			return n.cfgErrorf("cannot use _ as value")
		}
	}

	c, child := n.child[0], n.child[1:]

	t := c.typ.TypeOf()
	var low, high, max *node
	if len(child) >= 1 {
		if n.action == aSlice {
			low = child[0]
		} else {
			high = child[0]
		}
	}
	if len(child) >= 2 {
		if n.action == aSlice {
			high = child[1]
		} else {
			max = child[1]
		}
	}
	if len(child) == 3 && n.action == aSlice {
		max = child[2]
	}

	l := -1
	valid := false
	switch t.Kind() {
	case reflect.String:
		valid = true
		if c.rval.IsValid() {
			l = len(vString(c.rval))
		}
		if max != nil {
			return max.cfgErrorf("invalid operation: 3-index slice of string")
		}
	case reflect.Array:
		valid = true
		l = t.Len()
		// TODO(marc): check addressable status of array object (i.e. composite arrays are not).
	case reflect.Slice:
		valid = true
	case reflect.Ptr:
		if t.Elem().Kind() == reflect.Array {
			valid = true
			l = t.Elem().Len()
		}
	}
	if !valid {
		return c.cfgErrorf("cannot slice type %s", c.typ.id())
	}

	var ind [3]int64
	for i, nod := range []*node{low, high, max} {
		x := int64(-1)
		switch {
		case nod != nil:
			max := -1
			if l >= 0 {
				max = l + 1
			}
			if err := check.index(nod, max); err != nil {
				return err
			}
			if nod.rval.IsValid() {
				x = vInt(nod.rval)
			}
		case i == 0:
			x = 0
		case l >= 0:
			x = int64(l)
		}
		ind[i] = x
	}

	for i, x := range ind[:len(ind)-1] {
		if x <= 0 {
			continue
		}
		for _, y := range ind[i+1:] {
			if y < 0 || x <= y {
				continue
			}
			return n.cfgErrorf("invalid index values, must be low <= high <= max")
		}
	}
	return nil
}

// typeAssertionExpr type checks a type assert expression.
func (check typecheck) typeAssertionExpr(n *node, typ *itype) error {
	// TODO(nick): This type check is not complete and should be revisited once
	// https://github.com/golang/go/issues/39717 lands. It is currently impractical to
	// type check Named types as they cannot be asserted.

	if rt := n.typ.TypeOf(); rt.Kind() != reflect.Interface && rt != valueInterfaceType {
		return n.cfgErrorf("invalid type assertion: non-interface type %s on left", n.typ.id())
	}
	ims := n.typ.methods()
	if len(ims) == 0 {
		// Empty interface must be a dynamic check.
		return nil
	}

	if isInterface(typ) {
		// Asserting to an interface is a dynamic check as we must look to the
		// underlying struct.
		return nil
	}

	for name := range ims {
		im := lookupFieldOrMethod(n.typ, name)
		tm := lookupFieldOrMethod(typ, name)
		if im == nil {
			// This should not be possible.
			continue
		}
		if tm == nil {
			// Lookup for non-exported methods is impossible
			// for bin types, ignore them as they can't be used
			// directly by the interpreted programs.
			if !token.IsExported(name) && isBin(typ) {
				continue
			}
			return n.cfgErrorf("impossible type assertion: %s does not implement %s (missing %v method)", typ.id(), n.typ.id(), name)
		}
		if tm.recv != nil && tm.recv.TypeOf().Kind() == reflect.Ptr && typ.TypeOf().Kind() != reflect.Ptr {
			return n.cfgErrorf("impossible type assertion: %s does not implement %s as %q method has a pointer receiver", typ.id(), n.typ.id(), name)
		}

		if im.cat != funcT || tm.cat != funcT {
			// It only makes sense to compare in/out parameter types if both types are functions.
			continue
		}

		err := n.cfgErrorf("impossible type assertion: %s does not implement %s", typ.id(), n.typ.id())
		if im.numIn() != tm.numIn() || im.numOut() != tm.numOut() {
			return err
		}
		for i := 0; i < im.numIn(); i++ {
			if !im.in(i).equals(tm.in(i)) {
				return err
			}
		}
		for i := 0; i < im.numOut(); i++ {
			if !im.out(i).equals(tm.out(i)) {
				return err
			}
		}
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
			n.rval = reflect.ValueOf(constant.MakeString(string(rune(codepoint))))
			ok = true
		}

	case n.typ.convertibleTo(typ):
		ok = true
	}
	if !ok {
		return n.cfgErrorf("cannot convert expression of type %s to type %s", n.typ.id(), typ.id())
	}
	if !n.typ.untyped || c == nil {
		return nil
	}
	if isInterface(typ) || !isConstType(typ) {
		typ = n.typ.defaultType(n.rval, check.scope)
	}
	return check.convertUntyped(n, typ)
}

type param struct {
	nod *node
	typ *itype
}

func (p param) Type() *itype {
	if p.typ != nil {
		return p.typ
	}
	return p.nod.typ
}

// unpackParams unpacks child parameters into a slice of param.
// If there is only 1 child and it is a callExpr with an n-value return,
// the return types are returned, otherwise the original child nodes are
// returned with nil typ.
func (check typecheck) unpackParams(child []*node) (params []param) {
	if len(child) == 1 && isCall(child[0]) && child[0].child[0].typ.numOut() > 1 {
		c0 := child[0]
		ftyp := child[0].child[0].typ
		for i := 0; i < ftyp.numOut(); i++ {
			params = append(params, param{nod: c0, typ: ftyp.out(i)})
		}
		return params
	}

	for _, c := range child {
		params = append(params, param{nod: c})
	}
	return params
}

var builtinFuncs = map[string]struct {
	args     int
	variadic bool
}{
	bltnAppend:  {args: 1, variadic: true},
	bltnCap:     {args: 1, variadic: false},
	bltnClose:   {args: 1, variadic: false},
	bltnComplex: {args: 2, variadic: false},
	bltnImag:    {args: 1, variadic: false},
	bltnCopy:    {args: 2, variadic: false},
	bltnDelete:  {args: 2, variadic: false},
	bltnLen:     {args: 1, variadic: false},
	bltnMake:    {args: 1, variadic: true},
	bltnNew:     {args: 1, variadic: false},
	bltnPanic:   {args: 1, variadic: false},
	bltnPrint:   {args: 0, variadic: true},
	bltnPrintln: {args: 0, variadic: true},
	bltnReal:    {args: 1, variadic: false},
	bltnRecover: {args: 0, variadic: false},
}

func (check typecheck) builtin(name string, n *node, child []*node, ellipsis bool) error {
	fun := builtinFuncs[name]
	if ellipsis && name != bltnAppend {
		return n.cfgErrorf("invalid use of ... with builtin %s", name)
	}

	var params []param
	nparams := len(child)
	switch name {
	case bltnMake, bltnNew:
		// Special param handling
	default:
		params = check.unpackParams(child)
		nparams = len(params)
	}

	if nparams < fun.args {
		return n.cfgErrorf("not enough arguments in call to %s", name)
	} else if !fun.variadic && nparams > fun.args {
		return n.cfgErrorf("too many arguments for %s", name)
	}

	switch name {
	case bltnAppend:
		typ := params[0].Type()
		t := typ.TypeOf()
		if t == nil || t.Kind() != reflect.Slice {
			return params[0].nod.cfgErrorf("first argument to append must be slice; have %s", typ.id())
		}

		if nparams == 1 {
			return nil
		}
		// Special case append([]byte, "test"...) is allowed.
		t1 := params[1].Type()
		if nparams == 2 && ellipsis && t.Elem().Kind() == reflect.Uint8 && t1.TypeOf().Kind() == reflect.String {
			if t1.untyped {
				return check.convertUntyped(params[1].nod, check.scope.getType("string"))
			}
			return nil
		}

		fun := &node{
			typ: &itype{
				cat: funcT,
				arg: []*itype{
					typ,
					{cat: variadicT, val: valueTOf(t.Elem())},
				},
				ret: []*itype{typ},
			},
			ident: "append",
		}
		return check.arguments(n, child, fun, ellipsis)
	case bltnCap, bltnLen:
		typ := arrayDeref(params[0].Type())
		ok := false
		switch typ.TypeOf().Kind() {
		case reflect.Array, reflect.Slice, reflect.Chan:
			ok = true
		case reflect.String, reflect.Map:
			ok = name == bltnLen
		}
		if !ok {
			return params[0].nod.cfgErrorf("invalid argument for %s", name)
		}
	case bltnClose:
		p := params[0]
		typ := p.Type()
		t := typ.TypeOf()
		if t.Kind() != reflect.Chan {
			return p.nod.cfgErrorf("invalid operation: non-chan type %s", p.nod.typ.id())
		}
		if t.ChanDir() == reflect.RecvDir {
			return p.nod.cfgErrorf("invalid operation: cannot close receive-only channel")
		}
	case bltnComplex:
		var err error
		p0, p1 := params[0], params[1]
		typ0, typ1 := p0.Type(), p1.Type()
		switch {
		case typ0.untyped && !typ1.untyped:
			err = check.convertUntyped(p0.nod, typ1)
		case !typ0.untyped && typ1.untyped:
			err = check.convertUntyped(p1.nod, typ0)
		case typ0.untyped && typ1.untyped:
			fltType := untypedFloat(nil)
			err = check.convertUntyped(p0.nod, fltType)
			if err != nil {
				break
			}
			err = check.convertUntyped(p1.nod, fltType)
		}
		if err != nil {
			return err
		}

		// check we have the correct types after conversion.
		typ0, typ1 = p0.Type(), p1.Type()
		if !typ0.equals(typ1) {
			return n.cfgErrorf("invalid operation: mismatched types %s and %s", typ0.id(), typ1.id())
		}
		if !isFloat(typ0.TypeOf()) {
			return n.cfgErrorf("invalid operation: arguments have type %s, expected floating-point", typ0.id())
		}
	case bltnImag, bltnReal:
		p := params[0]
		typ := p.Type()
		if typ.untyped {
			if err := check.convertUntyped(p.nod, untypedComplex(nil)); err != nil {
				return err
			}
		}
		typ = p.Type()
		if !isComplex(typ.TypeOf()) {
			return p.nod.cfgErrorf("invalid argument type %s for %s", typ.id(), name)
		}
	case bltnCopy:
		typ0, typ1 := params[0].Type(), params[1].Type()
		var t0, t1 reflect.Type
		if t := typ0.TypeOf(); t.Kind() == reflect.Slice {
			t0 = t.Elem()
		}

		switch t := typ1.TypeOf(); t.Kind() {
		case reflect.String:
			t1 = reflect.TypeOf(byte(1))
		case reflect.Slice:
			t1 = t.Elem()
		}

		if t0 == nil || t1 == nil {
			return n.cfgErrorf("copy expects slice arguments")
		}
		if !reflect.DeepEqual(t0, t1) {
			return n.cfgErrorf("arguments to copy have different element types %s and %s", typ0.id(), typ1.id())
		}
	case bltnDelete:
		typ := params[0].Type()
		if typ.TypeOf().Kind() != reflect.Map {
			return params[0].nod.cfgErrorf("first argument to delete must be map; have %s", typ.id())
		}
		ktyp := params[1].Type()
		if typ.key != nil && !ktyp.assignableTo(typ.key) {
			return params[1].nod.cfgErrorf("cannot use %s as type %s in delete", ktyp.id(), typ.key.id())
		}
	case bltnMake:
		var min int
		switch child[0].typ.TypeOf().Kind() {
		case reflect.Slice:
			min = 2
		case reflect.Map, reflect.Chan:
			min = 1
		default:
			return child[0].cfgErrorf("cannot make %s; type must be slice, map, or channel", child[0].typ.id())
		}
		if nparams < min {
			return n.cfgErrorf("not enough arguments in call to make")
		} else if nparams > min+1 {
			return n.cfgErrorf("too many arguments for make")
		}

		var sizes []int
		for _, c := range child[1:] {
			if err := check.index(c, -1); err != nil {
				return err
			}
			if c.rval.IsValid() {
				sizes = append(sizes, int(vInt(c.rval)))
			}
		}
		for len(sizes) == 2 && sizes[0] > sizes[1] {
			return n.cfgErrorf("len larger than cap in make")
		}

	case bltnPanic:
		return check.assignment(params[0].nod, check.scope.getType("interface{}"), "argument to panic")
	case bltnPrint, bltnPrintln:
		for _, param := range params {
			if param.typ != nil {
				continue
			}

			if err := check.assignment(param.nod, nil, "argument to "+name); err != nil {
				return err
			}
		}
	case bltnRecover, bltnNew:
		// Nothing to do.
	default:
		return n.cfgErrorf("unsupported builtin %s", name)
	}
	return nil
}

// arrayDeref returns A if typ is *A, otherwise typ.
func arrayDeref(typ *itype) *itype {
	if typ.cat == valueT && typ.TypeOf().Kind() == reflect.Ptr {
		t := typ.TypeOf()
		if t.Elem().Kind() == reflect.Array {
			return valueTOf(t.Elem())
		}
		return typ
	}

	if typ.cat == ptrT && typ.val.cat == arrayT {
		return typ.val
	}
	return typ
}

// arguments type checks the call expression arguments.
func (check typecheck) arguments(n *node, child []*node, fun *node, ellipsis bool) error {
	params := check.unpackParams(child)
	l := len(child)
	if ellipsis {
		if !fun.typ.isVariadic() {
			return n.cfgErrorf("invalid use of ..., corresponding parameter is non-variadic")
		}
		if len(params) > l {
			return child[0].cfgErrorf("cannot use ... with %d-valued %s", child[0].child[0].typ.numOut(), child[0].child[0].typ.id())
		}
	}

	var cnt int
	for i, param := range params {
		ellip := i == l-1 && ellipsis
		if err := check.argument(param, fun.typ, cnt, l, ellip); err != nil {
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

func (check typecheck) argument(p param, ftyp *itype, i, l int, ellipsis bool) error {
	atyp := getArg(ftyp, i)
	if atyp == nil {
		return p.nod.cfgErrorf("too many arguments")
	}

	if p.typ == nil && isCall(p.nod) && p.nod.child[0].typ.numOut() != 1 {
		if l == 1 {
			return p.nod.cfgErrorf("cannot use %s as type %s", p.nod.child[0].typ.id(), getArgsID(ftyp))
		}
		return p.nod.cfgErrorf("cannot use %s as type %s", p.nod.child[0].typ.id(), atyp.id())
	}

	if ellipsis {
		if i != ftyp.numIn()-1 {
			return p.nod.cfgErrorf("can only use ... with matching parameter")
		}
		t := p.Type().TypeOf()
		if t.Kind() != reflect.Slice || !(valueTOf(t.Elem())).assignableTo(atyp) {
			return p.nod.cfgErrorf("cannot use %s as type %s", p.nod.typ.id(), (sliceOf(atyp)).id())
		}
		return nil
	}

	if p.typ != nil {
		if !p.typ.assignableTo(atyp) {
			return p.nod.cfgErrorf("cannot use %s as type %s", p.nod.child[0].typ.id(), getArgsID(ftyp))
		}
		return nil
	}
	return check.assignment(p.nod, atyp, "")
}

func getArg(ftyp *itype, i int) *itype {
	l := ftyp.numIn()
	switch {
	case ftyp.isVariadic() && i >= l-1:
		arg := ftyp.in(l - 1).val
		return arg
	case i < l:
		return ftyp.in(i)
	case ftyp.cat == valueT && i < ftyp.rtype.NumIn():
		return valueTOf(ftyp.rtype.In(i))
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
		ityp = n.typ.defaultType(n.rval, check.scope)
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
