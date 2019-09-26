package interp

import (
	"reflect"
)

func valueGenerator(n *node, i int) func(*frame) reflect.Value {
	switch n.level {
	case 0:
		return func(f *frame) reflect.Value { return f.data[i] }
	case 1:
		return func(f *frame) reflect.Value { return f.anc.data[i] }
	case 2:
		return func(f *frame) reflect.Value { return f.anc.anc.data[i] }
	default:
		return func(f *frame) reflect.Value {
			for level := n.level; level > 0; level-- {
				f = f.anc
			}
			return f.data[i]
		}
	}
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

func genValueAsFunctionWrapper(n *node) func(*frame) reflect.Value {
	v := genValue(n)
	return func(f *frame) reflect.Value {
		return genFunctionWrapper(v(f).Interface().(*node))(f)
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
		v := n.rval
		if !v.IsValid() {
			v = reflect.New(reflect.TypeOf((*interface{})(nil)).Elem()).Elem()
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
	case rvalueExpr:
		v := n.rval
		return func(f *frame) reflect.Value { return v }
	default:
		if n.rval.IsValid() {
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

func genValueInterfacePtr(n *node) func(*frame) reflect.Value {
	value := genValue(n)
	it := reflect.TypeOf((*interface{})(nil)).Elem()

	return func(f *frame) reflect.Value {
		v := reflect.New(it).Elem()
		v.Set(value(f))
		return v.Addr()
	}
}

func genValueInterface(n *node) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		return reflect.ValueOf(valueInterface{n, value(f)})
	}
}

func genValueInterfaceValue(n *node) func(*frame) reflect.Value {
	value := genValue(n)

	return func(f *frame) reflect.Value {
		return value(f).Interface().(valueInterface).value
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
	}
	return nil
}

func genValueComplex(n *node) func(*frame) (reflect.Value, complex128) {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *frame) (reflect.Value, complex128) { v := value(f); return v, complex(float64(v.Int()), 0) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(f *frame) (reflect.Value, complex128) { v := value(f); return v, complex(float64(v.Uint()), 0) }
	case reflect.Float32, reflect.Float64:
		return func(f *frame) (reflect.Value, complex128) { v := value(f); return v, complex(v.Float(), 0) }
	case reflect.Complex64, reflect.Complex128:
		return func(f *frame) (reflect.Value, complex128) { v := value(f); return v, v.Complex() }
	}
	return nil
}

func genValueString(n *node) func(*frame) (reflect.Value, string) {
	value := genValue(n)
	return func(f *frame) (reflect.Value, string) { v := value(f); return v, v.String() }
}
