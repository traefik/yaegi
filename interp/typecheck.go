package interp

import (
	"errors"
	"fmt"
	"go/constant"
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
func (check typecheck) op(p opPredicates, n, c *node, t reflect.Type) error {
	if pred := p[n.action]; pred != nil {
		if !pred(t) {
			return n.cfgErrorf("invalid operation: operator %v not defined on %s", n.action, c.typ.id())
		}
	} else {
		return n.cfgErrorf("invalid operation: unknown operator %v", n.action)
	}
	return nil
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

	if err := check.op(unaryOpPredicates, n, c0, t0); err != nil {
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
	if isShiftNode(n) {
		return check.shift(n)
	}

	_ = check.convertUntyped(c0, c1.typ)
	_ = check.convertUntyped(c1, c0.typ)

	if isComparisonNode(n) {
		return check.comparison(n)
	}

	if !c0.typ.equals(c1.typ) {
		return n.cfgErrorf("invalid operation: mismatched types %s and %s", c0.typ.id(), c1.typ.id())
	}

	t0 := c0.typ.TypeOf()
	if err := check.op(binaryOpPredicates, n, c0, t0); err != nil {
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

var errCantConvert = errors.New("cannot convert")

func (check typecheck) convertUntyped(n *node, typ *itype) error {
	if n.typ == nil || !n.typ.untyped || typ == nil {
		return nil
	}

	convErr := n.cfgErrorf("invalid operation: cannot convert %s to %s", n.typ.id(), typ.id())

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
		if !typ.isNil() {
			return convErr
		}
		return nil
	default:
		return convErr
	}

	n.rval, err = check.convertConst(n.rval, rtyp)
	if err != nil {
		if errors.Is(err, errCantConvert) {
			return n.cfgErrorf("invalid operation: cannot convert %s to %s", n.typ.id(), typ.id())
		}
		return n.cfgErrorf(err.Error())
	}
	n.typ = ityp
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
		k := c.Kind()
		c = constant.ToInt(c)
		i, ok := constant.Int64Val(c)
		if !ok {
			if k == constant.Float || k == constant.Complex {
				return v, fmt.Errorf("%s truncated to %s", c.ExactString(), t.Kind().String())
			}
			return v, errCantConvert
		}
		l := constant.BitLen(c)
		if l > bitlen[kind] {
			return v, fmt.Errorf("constant %s overflows %s", c.ExactString(), t.Kind().String())
		}
		v = reflect.ValueOf(i).Convert(t)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		k := c.Kind()
		c = constant.ToInt(c)
		i, ok := constant.Uint64Val(c)
		if !ok {
			if k == constant.Float || k == constant.Complex {
				return v, fmt.Errorf("%s truncated to %s", c.ExactString(), t.Kind().String())
			}
			return v, errCantConvert
		}
		l := constant.BitLen(c)
		if l > bitlen[kind] {
			return v, fmt.Errorf("constant %s overflows %s", c.ExactString(), t.Kind().String())
		}
		v = reflect.ValueOf(i).Convert(t)
	case reflect.Float32:
		f, ok := constant.Float32Val(constant.ToFloat(c))
		if !ok {
			return v, errCantConvert
		}
		v = reflect.ValueOf(f)
	case reflect.Float64:
		f, ok := constant.Float64Val(constant.ToFloat(c))
		if !ok {
			return v, errCantConvert
		}
		v = reflect.ValueOf(f)
	case reflect.Complex64:
		r, ok := constant.Float32Val(constant.Real(c))
		if !ok {
			return v, errCantConvert
		}
		i, ok := constant.Float32Val(constant.Imag(c))
		if !ok {
			return v, errCantConvert
		}
		v = reflect.ValueOf(complex(r, i)).Convert(t)
	case reflect.Complex128:
		r, ok := constant.Float64Val(constant.Real(c))
		if !ok {
			return v, errCantConvert
		}
		i, ok := constant.Float64Val(constant.Imag(c))
		if !ok {
			return v, errCantConvert
		}
		v = reflect.ValueOf(complex(r, i)).Convert(t)
	default:
		return v, errCantConvert
	}
	return v, nil
}
