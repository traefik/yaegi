package interp

import (
	"reflect"
)

func valueGenerator(n *Node, i int) func(*Frame) reflect.Value {
	switch n.level {
	case 0:
		return func(f *Frame) reflect.Value { return f.data[i] }
	case 1:
		return func(f *Frame) reflect.Value { return f.anc.data[i] }
	case 2:
		return func(f *Frame) reflect.Value { return f.anc.anc.data[i] }
	default:
		return func(f *Frame) reflect.Value {
			for level := n.level; level > 0; level-- {
				f = f.anc
			}
			return f.data[i]
		}
	}
}

func genValueRecv(n *Node) func(*Frame) reflect.Value {
	v := genValue(n.recv.node)
	fi := n.recv.index

	if len(fi) == 0 {
		return v
	}

	return func(f *Frame) reflect.Value {
		r := v(f)
		if r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		return r.FieldByIndex(fi)
	}
}

func genValueAs(n *Node, t reflect.Type) func(*Frame) reflect.Value {
	v := genValue(n)
	return func(f *Frame) reflect.Value {
		return v(f).Convert(t)
	}
}

func genValue(n *Node) func(*Frame) reflect.Value {
	switch n.kind {
	case BasicLit, FuncDecl:
		var v reflect.Value
		if w, ok := n.val.(reflect.Value); ok {
			v = w
		} else {
			v = reflect.ValueOf(n.val)
		}
		return func(f *Frame) reflect.Value { return v }
	case Rvalue:
		v := n.rval
		return func(f *Frame) reflect.Value { return v }
	default:
		if n.rval.IsValid() {
			v := n.rval
			return func(f *Frame) reflect.Value { return v }
		}
		if n.sym != nil {
			if n.sym.index < 0 {
				return genValue(n.sym.node)
			}
			i := n.sym.index
			if n.sym.global {
				return func(f *Frame) reflect.Value {
					return n.interp.Frame.data[i]
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
			return func(f *Frame) reflect.Value { return v }
		}
		return valueGenerator(n, n.findex)
	}
}

func genValueInterface(n *Node) func(*Frame) reflect.Value {
	value := genValue(n)

	return func(f *Frame) reflect.Value {
		return reflect.ValueOf(valueInterface{n, value(f)})
	}
}

func genValueInterfaceValue(n *Node) func(*Frame) reflect.Value {
	value := genValue(n)

	return func(f *Frame) reflect.Value {
		return value(f).Interface().(valueInterface).value
	}
}

func genValueInt(n *Node) func(*Frame) int64 {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *Frame) int64 { return value(f).Int() }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(f *Frame) int64 { return int64(value(f).Uint()) }
	case reflect.Float32, reflect.Float64:
		return func(f *Frame) int64 { return int64(value(f).Float()) }
	}
	return nil
}

func genValueUint(n *Node) func(*Frame) uint64 {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *Frame) uint64 { return uint64(value(f).Int()) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(f *Frame) uint64 { return value(f).Uint() }
	case reflect.Float32, reflect.Float64:
		return func(f *Frame) uint64 { return uint64(value(f).Float()) }
	}
	return nil
}

func genValueFloat(n *Node) func(*Frame) float64 {
	value := genValue(n)

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f *Frame) float64 { return float64(value(f).Int()) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(f *Frame) float64 { return float64(value(f).Uint()) }
	case reflect.Float32, reflect.Float64:
		return func(f *Frame) float64 { return value(f).Float() }
	}
	return nil
}

func genValueString(n *Node) func(*Frame) string {
	value := genValue(n)
	return func(f *Frame) string { return value(f).String() }
}
