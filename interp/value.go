package interp

import (
	"go/constant"
	"reflect"
)

func valueGenerator(n *node, i int) func(*frame) reflect.Value {
	switch n.level {
	case 0:
		return func(f *frame) reflect.Value { return valueOf(f.data, i) }
	case 1:
		return func(f *frame) reflect.Value { return valueOf(f.anc.data, i) }
	case 2:
		return func(f *frame) reflect.Value { return valueOf(f.anc.anc.data, i) }
	default:
		return func(f *frame) reflect.Value {
			for level := n.level; level > 0; level-- {
				f = f.anc
			}
			return valueOf(f.data, i)
		}
	}
}

// valueOf safely recovers the ith element of data. This is necessary
// because a cancellation prior to any evaluation result may leave
// the frame's data empty.
func valueOf(data []reflect.Value, i int) reflect.Value {
	if i < len(data) {
		return data[i]
	}
	return reflect.Value{}
}

func genValueRecvIndirect(n *node) func(*frame) reflect.Value {
	v := genValueRecv(n)
	return func(f *frame) reflect.Value { return v(f).Elem() }
}

func genValueRecv(n *node) func(*frame) reflect.Value {
	v := genValue(n.recv.node)
	fi := n.recv.index

	if len(fi) == 0 {
		return v
	}

	return func(f *frame) reflect.Value {
		r := v(f)
		if r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		return r.FieldByIndex(fi)
	}
}

func genValueRecvInterfacePtr(n *node) func(*frame) reflect.Value {
	v := genValue(n.recv.node)
	fi := n.recv.index

	return func(f *frame) reflect.Value {
		r := v(f)
		r = r.Elem().Elem()

		if len(fi) == 0 {
			return r
		}

		if r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		return r.FieldByIndex(fi)
	}
}

func genValueAsFunctionWrapper(n *node) func(*frame) reflect.Value {
	value := genValue(n)
	typ := n.typ.TypeOf()

	return func(f *frame) reflect.Value {
		v := value(f)
		if v.IsNil() {
			return reflect.New(typ).Elem()
		}
		return genFunctionWrapper(v.Interface().(*node))(f)
	}
}

func genValueAs(n *node, t reflect.Type) func(*frame) reflect.Value {
	v := genValue(n)
	return func(f *frame) reflect.Value {
		return v(f).Convert(t)
	}
}

func genValue(n *node) func(*frame) reflect.Value {
	switch n.kind {
	case basicLit:
		convertConstantValue(n)
		v := n.rval
		if !v.IsValid() {
			v = reflect.New(interf).Elem()
		}
		return func(f *frame) reflect.Value { return v }
	case funcDecl:
		var v reflect.Value
		if w, ok := n.val.(reflect.Value); ok {
			v = w
		} else {
			v = reflect.ValueOf(n.val)
		}
		return func(f *frame) reflect.Value { return v }
	default:
		if n.rval.IsValid() {
			convertConstantValue(n)
			v := n.rval
			return func(f *frame) reflect.Value { return v }
		}
		if n.sym != nil {
			if n.sym.index < 0 {
				return genValue(n.sym.node)
			}
			i := n.sym.index
			if n.sym.global {
				return func(f *frame) reflect.Value {
					return n.interp.frame.data[i]
				}
			}
			return valueGenerator(n, i)
		}
		if n.findex < 0 {
			var v reflect.Value
			if w, ok := n.val.(reflect.Value); ok {
				v = w
			} else {
				v = reflect.ValueOf(n.val)
			}
			return func(f *frame) reflect.Value { return v }
		}
		return valueGenerator(n, n.findex)
	}
}

func genValueArray(n *node) func(*frame) reflect.Value {
	value := genValue(n)
	// dereference array pointer, to support array operations on array pointer
	if n.typ.TypeOf().Kind() == reflect.Ptr {
		return func(f *frame) reflect.Value {
			return value(f).Elem()
		}
	}
	return value
}

func genValueRangeArray(n *node) func(*frame) reflect.Value {
	value := genValue(n)
	// dereference array pointer, to support array operations on array pointer
	if n.typ.TypeOf().Kind() == reflect.Ptr {
		return func(f *frame) reflect.Value {
			return value(f).Elem()
		}
	}
	return func(f *frame) reflect.Value {
		// This is necessary to prevent changes in the returned
		// reflect.Value being reflected back to the value used
		// for the range expression.
		return reflect.ValueOf(value(f).Interface())
	}
}

func genValueInterfaceArray(n *node) func(*frame) reflect.Value {
	value := genValue(n)
	return func(f *frame) reflect.Value {
		vi := value(f).Interface().([]valueInterface)
		v := reflect.MakeSlice(reflect.TypeOf([]interface{}{}), len(vi), len(vi))
		for i, vv := range vi {
			v.Index(i).Set(vv.value)
		}

		return v
	}
}

func genValueInterface(n *node) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		v := value(f)
		nod := n
		for v.IsValid() {
			// traverse interface indirections to find out concrete type
			vi, ok := v.Interface().(valueInterface)
			if !ok {
				break
			}
			v = vi.value
			nod = vi.node
		}
		return reflect.ValueOf(valueInterface{nod, v})
	}
}

func zeroInterfaceValue() reflect.Value {
	n := &node{kind: basicLit, typ: &itype{cat: nilT, untyped: true}}
	v := reflect.New(interf).Elem()
	return reflect.ValueOf(valueInterface{n, v})
}

func genValueOutput(n *node, t reflect.Type) func(*frame) reflect.Value {
	value := genValue(n)
	switch {
	case n.anc.action == aAssign && n.anc.typ.cat == interfaceT:
		fallthrough
	case n.anc.kind == returnStmt && n.anc.val.(*node).typ.ret[0].cat == interfaceT:
		// The result of the builtin has to be returned as an interface type.
		// Wrap it in a valueInterface and return the dereferenced value.
		return func(f *frame) reflect.Value {
			d := value(f)
			v := reflect.New(t).Elem()
			d.Set(reflect.ValueOf(valueInterface{n, v}))
			return v
		}
	}
	return value
}

func genValueInterfaceValue(n *node) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		v := value(f)
		if v.Interface().(valueInterface).node == nil {
			// Uninitialized interface value, set it to a correct zero value.
			v.Set(zeroInterfaceValue())
			v = value(f)
		}
		return v.Interface().(valueInterface).value
	}
}

func genValueNode(n *node) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		return reflect.ValueOf(&node{rval: value(f)})
	}
}

func genValueRecursiveInterface(n *node, t reflect.Type) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		vv := value(f)
		v := reflect.New(t).Elem()
		toRecursive(v, vv)
		return v
	}
}

func toRecursive(dest, src reflect.Value) {
	if !src.IsValid() {
		return
	}

	switch dest.Kind() {
	case reflect.Map:
		v := reflect.MakeMapWithSize(dest.Type(), src.Len())
		for _, kv := range src.MapKeys() {
			vv := reflect.New(dest.Type().Elem()).Elem()
			toRecursive(vv, src.MapIndex(kv))
			vv.SetMapIndex(kv, vv)
		}
		dest.Set(v)
	case reflect.Slice:
		l := src.Len()
		v := reflect.MakeSlice(dest.Type(), l, l)
		for i := 0; i < l; i++ {
			toRecursive(v.Index(i), src.Index(i))
		}
		dest.Set(v)
	case reflect.Ptr:
		v := reflect.New(dest.Type().Elem()).Elem()
		s := src
		if s.Elem().Kind() != reflect.Struct { // In the case of *interface{}, we want *struct{}
			s = s.Elem()
		}
		toRecursive(v, s)
		dest.Set(v.Addr())
	default:
		dest.Set(src)
	}
}

func genValueRecursiveInterfacePtrValue(n *node) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		v := value(f)
		if v.IsZero() {
			return v
		}
		return v.Elem().Elem()
	}
}

func vInt(v reflect.Value) (i int64) {
	switch v.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i = v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		i = int64(v.Uint())
	case reflect.Float32, reflect.Float64:
		i = int64(v.Float())
	case reflect.Complex64, reflect.Complex128:
		i = int64(real(v.Complex()))
	}
	if v.Type().Implements(constVal) {
		c := v.Interface().(constant.Value)
		i, _ = constant.Int64Val(constant.ToInt(c))
	}
	return
}

func vUint(v reflect.Value) (i uint64) {
	switch v.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i = uint64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		i = v.Uint()
	case reflect.Float32, reflect.Float64:
		i = uint64(v.Float())
	case reflect.Complex64, reflect.Complex128:
		i = uint64(real(v.Complex()))
	}
	if v.Type().Implements(constVal) {
		c := v.Interface().(constant.Value)
		iv, _ := constant.Int64Val(constant.ToInt(c))
		i = uint64(iv)
	}
	return
}

func vComplex(v reflect.Value) (c complex128) {
	switch v.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		c = complex(float64(v.Int()), 0)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		c = complex(float64(v.Uint()), 0)
	case reflect.Float32, reflect.Float64:
		c = complex(v.Float(), 0)
	case reflect.Complex64, reflect.Complex128:
		c = v.Complex()
	}
	if v.Type().Implements(constVal) {
		con := v.Interface().(constant.Value)
		con = constant.ToComplex(con)
		rel, _ := constant.Float64Val(constant.Real(con))
		img, _ := constant.Float64Val(constant.Imag(con))
		c = complex(rel, img)
	}
	return
}

func vFloat(v reflect.Value) (i float64) {
	switch v.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		i = float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		i = v.Float()
	case reflect.Complex64, reflect.Complex128:
		i = real(v.Complex())
	}
	if v.Type().Implements(constVal) {
		c := v.Interface().(constant.Value)
		i, _ = constant.Float64Val(constant.ToFloat(c))
	}
	return
}

func vConstantValue(v reflect.Value) (c constant.Value) {
	if v.Type().Implements(constVal) {
		c = v.Interface().(constant.Value)
	}
	return
}

func genValueInt(n *node) func(*frame) (reflect.Value, int64) {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *frame) (reflect.Value, int64) { v := value(f); return v, v.Int() }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(f *frame) (reflect.Value, int64) { v := value(f); return v, int64(v.Uint()) }
	case reflect.Float32, reflect.Float64:
		return func(f *frame) (reflect.Value, int64) { v := value(f); return v, int64(v.Float()) }
	case reflect.Complex64, reflect.Complex128:
		if n.typ.untyped && n.rval.IsValid() && imag(n.rval.Complex()) == 0 {
			return func(f *frame) (reflect.Value, int64) { v := value(f); return v, int64(real(v.Complex())) }
		}
	}
	return nil
}

func genValueUint(n *node) func(*frame) (reflect.Value, uint64) {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *frame) (reflect.Value, uint64) { v := value(f); return v, uint64(v.Int()) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(f *frame) (reflect.Value, uint64) { v := value(f); return v, v.Uint() }
	case reflect.Float32, reflect.Float64:
		return func(f *frame) (reflect.Value, uint64) { v := value(f); return v, uint64(v.Float()) }
	case reflect.Complex64, reflect.Complex128:
		if n.typ.untyped && n.rval.IsValid() && imag(n.rval.Complex()) == 0 {
			return func(f *frame) (reflect.Value, uint64) { v := value(f); return v, uint64(real(v.Complex())) }
		}
	}
	return nil
}

func genValueFloat(n *node) func(*frame) (reflect.Value, float64) {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *frame) (reflect.Value, float64) { v := value(f); return v, float64(v.Int()) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(f *frame) (reflect.Value, float64) { v := value(f); return v, float64(v.Uint()) }
	case reflect.Float32, reflect.Float64:
		return func(f *frame) (reflect.Value, float64) { v := value(f); return v, v.Float() }
	case reflect.Complex64, reflect.Complex128:
		if n.typ.untyped && n.rval.IsValid() && imag(n.rval.Complex()) == 0 {
			return func(f *frame) (reflect.Value, float64) { v := value(f); return v, real(v.Complex()) }
		}
	}
	return nil
}

func genValueComplex(n *node) func(*frame) reflect.Value {
	vc := genComplex(n)
	return func(f *frame) reflect.Value { return reflect.ValueOf(vc(f)) }
}

func genComplex(n *node) func(*frame) complex128 {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *frame) complex128 { return complex(float64(value(f).Int()), 0) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(f *frame) complex128 { return complex(float64(value(f).Uint()), 0) }
	case reflect.Float32, reflect.Float64:
		return func(f *frame) complex128 { return complex(value(f).Float(), 0) }
	case reflect.Complex64, reflect.Complex128:
		return func(f *frame) complex128 { return value(f).Complex() }
	}
	return nil
}

func genValueString(n *node) func(*frame) (reflect.Value, string) {
	value := genValue(n)
	return func(f *frame) (reflect.Value, string) { v := value(f); return v, v.String() }
}
