package interp

import (
	"go/constant"
	"reflect"
)

const (
	notInFrame  = -1 // value of node.findex for literal values (not in frame)
	globalFrame = -1 // value of node.level for global symbols
)

func valueGenerator(n *node, i int) func(*frame) reflect.Value {
	switch n.level {
	case globalFrame:
		return func(f *frame) reflect.Value { return valueOf(f.root.data, i) }
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

func genValueBinMethodOnInterface(n *node, defaultGen func(*frame) reflect.Value) func(*frame) reflect.Value {
	if n == nil || n.child == nil || n.child[0] == nil ||
		n.child[0].child == nil || n.child[0].child[0] == nil {
		return defaultGen
	}
	c0 := n.child[0]
	if c0.child[1] == nil || c0.child[1].ident == "" {
		return defaultGen
	}
	value0 := genValue(c0.child[0])

	return func(f *frame) reflect.Value {
		v := value0(f)
		var nod *node

		for v.IsValid() {
			// Traverse interface indirections to find out concrete type.
			vi, ok := v.Interface().(valueInterface)
			if !ok {
				break
			}
			v = vi.value
			nod = vi.node
		}

		if nod == nil || nod.typ.rtype == nil {
			return defaultGen(f)
		}

		// Try to get the bin method, if it doesnt exist, fall back to
		// the default generator function.
		meth, ok := nod.typ.rtype.MethodByName(c0.child[1].ident)
		if !ok {
			return defaultGen(f)
		}

		return meth.Func
	}
}

func genValueRecvIndirect(n *node) func(*frame) reflect.Value {
	vr := genValueRecv(n)
	return func(f *frame) reflect.Value {
		v := vr(f)
		if vi, ok := v.Interface().(valueInterface); ok {
			return vi.value
		}
		return v.Elem()
	}
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

func genValueBinRecv(n *node, recv *receiver) func(*frame) reflect.Value {
	value := genValue(n)
	binValue := genValue(recv.node)

	v := func(f *frame) reflect.Value {
		if def, ok := value(f).Interface().(*node); ok {
			if def != nil && def.recv != nil && def.recv.val.IsValid() {
				return def.recv.val
			}
		}

		ival, _ := binValue(f).Interface().(valueInterface)
		return ival.value
	}

	fi := recv.index
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

func genValueAsFunctionWrapper(n *node) func(*frame) reflect.Value {
	value := genValue(n)
	typ := n.typ.TypeOf()

	return func(f *frame) reflect.Value {
		v := value(f)
		if v.IsNil() {
			return reflect.New(typ).Elem()
		}
		vn, ok := v.Interface().(*node)
		if ok && vn.rval.IsValid() && vn.rval.Type().Kind() == reflect.Func {
			// The node value is already a callable func, no need to wrap it.
			return vn.rval
		}
		return genFunctionWrapper(vn)(f)
	}
}

func genValueAs(n *node, t reflect.Type) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		v := value(f)
		switch v.Type().Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice, reflect.UnsafePointer:
			if v.IsNil() {
				return reflect.New(t).Elem()
			}
		}
		return v.Convert(t)
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
			i := n.sym.index
			if i < 0 {
				return genValue(n.sym.node)
			}
			if n.sym.global {
				return func(f *frame) reflect.Value { return f.root.data[i] }
			}
			return valueGenerator(n, i)
		}
		if n.findex == notInFrame {
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

func genDestValue(typ *itype, n *node) func(*frame) reflect.Value {
	convertLiteralValue(n, typ.TypeOf())
	switch {
	case isInterfaceSrc(typ) && !isEmptyInterface(typ):
		return genValueInterface(n)
	case isFuncSrc(typ) && (n.typ.cat == valueT || n.typ.cat == nilT):
		return genValueNode(n)
	case typ.cat == valueT && isFuncSrc(n.typ):
		return genFunctionWrapper(n)
	case isInterfaceBin(typ):
		return genInterfaceWrapper(n, typ.rtype)
	case n.kind == basicLit && n.val == nil:
		return func(*frame) reflect.Value { return reflect.New(typ.rtype).Elem() }
	case n.typ.untyped && isComplex(typ.TypeOf()):
		return genValueComplex(n)
	case n.typ.untyped && !typ.untyped:
		return genValueAs(n, typ.TypeOf())
	}
	return genValue(n)
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

	switch {
	case n.typ.TypeOf().Kind() == reflect.Ptr:
		// dereference array pointer, to support array operations on array pointer
		return func(f *frame) reflect.Value {
			return value(f).Elem()
		}
	case n.typ.val != nil && n.typ.val.cat == interfaceT:
		if len(n.typ.val.field) > 0 {
			return func(f *frame) reflect.Value {
				val := value(f)
				v := []valueInterface{}
				for i := 0; i < val.Len(); i++ {
					switch av := val.Index(i).Interface().(type) {
					case []valueInterface:
						v = append(v, av...)
					case valueInterface:
						v = append(v, av)
					default:
						panic(n.cfgErrorf("invalid type %v", val.Index(i).Type()))
					}
				}
				return reflect.ValueOf(v)
			}
		}
		// empty interface, do not wrap.
		fallthrough
	default:
		return func(f *frame) reflect.Value {
			// This is necessary to prevent changes in the returned
			// reflect.Value being reflected back to the value used
			// for the range expression.
			return reflect.ValueOf(value(f).Interface())
		}
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

		// empty interface, do not wrap.
		if nod != nil && isEmptyInterface(nod.typ) {
			return v
		}

		return reflect.ValueOf(valueInterface{nod, v})
	}
}

func getConcreteValue(val reflect.Value) reflect.Value {
	v := val
	for {
		vi, ok := v.Interface().(valueInterface)
		if !ok {
			break
		}
		v = vi.value
	}
	if v.NumMethod() > 0 {
		return v
	}
	if v.Type().Kind() != reflect.Struct {
		return v
	}
	// Search a concrete value in fields of an emulated interface.
	for i := v.NumField() - 1; i >= 0; i-- {
		vv := v.Field(i)
		if vv.Type().Kind() == reflect.Interface {
			vv = vv.Elem()
		}
		if vv.IsValid() {
			return vv
		}
	}
	return v
}

func zeroInterfaceValue() reflect.Value {
	n := &node{kind: basicLit, typ: &itype{cat: nilT, untyped: true, str: "nil"}}
	v := reflect.New(interf).Elem()
	return reflect.ValueOf(valueInterface{n, v})
}

func wantEmptyInterface(n *node) bool {
	return isEmptyInterface(n.typ) ||
		n.anc.action == aAssign && n.anc.typ.cat == interfaceT && len(n.anc.typ.field) == 0 ||
		n.anc.kind == returnStmt && n.anc.val.(*node).typ.ret[0].cat == interfaceT && len(n.anc.val.(*node).typ.ret[0].field) == 0
}

func genValueOutput(n *node, t reflect.Type) func(*frame) reflect.Value {
	value := genValue(n)
	switch {
	case n.anc.action == aAssign && n.anc.typ.cat == interfaceT:
		if len(n.anc.typ.field) == 0 {
			// empty interface, do not wrap
			return value
		}
		fallthrough
	case n.anc.kind == returnStmt && n.anc.val.(*node).typ.ret[0].cat == interfaceT:
		if nod, ok := n.anc.val.(*node); !ok || len(nod.typ.ret[0].field) == 0 {
			// empty interface, do not wrap
			return value
		}
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

func valueInterfaceValue(v reflect.Value) reflect.Value {
	for {
		vv, ok := v.Interface().(valueInterface)
		if !ok {
			break
		}
		v = vv.value
	}
	return v
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
		return valueInterfaceValue(v)
	}
}

func genValueNode(n *node) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		return reflect.ValueOf(&node{rval: value(f)})
	}
}

func vInt(v reflect.Value) (i int64) {
	if c := vConstantValue(v); c != nil {
		i, _ = constant.Int64Val(constant.ToInt(c))
		return i
	}
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
	return
}

func vUint(v reflect.Value) (i uint64) {
	if c := vConstantValue(v); c != nil {
		i, _ = constant.Uint64Val(constant.ToInt(c))
		return i
	}
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
	return
}

func vComplex(v reflect.Value) (c complex128) {
	if c := vConstantValue(v); c != nil {
		c = constant.ToComplex(c)
		rel, _ := constant.Float64Val(constant.Real(c))
		img, _ := constant.Float64Val(constant.Imag(c))
		return complex(rel, img)
	}
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
	return
}

func vFloat(v reflect.Value) (i float64) {
	if c := vConstantValue(v); c != nil {
		i, _ = constant.Float64Val(constant.ToFloat(c))
		return i
	}
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
	return
}

func vString(v reflect.Value) (s string) {
	if c := vConstantValue(v); c != nil {
		s = constant.StringVal(c)
		return s
	}
	return v.String()
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
