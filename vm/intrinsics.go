package vm

// Go implementations of bodyless (assembly / linkname) functions of internal
// packages, so that
// those packages can be interpreted from source.
//
// The unsafe builtins (unsafe.String, Slice, StringData, SliceData) are
// compiled in unsafe.go.

import (
	"bytes"
	"reflect"
	"strings"
)

func valBytes(v Value) []byte {
	if b, ok := v.R.([]byte); ok {
		return b
	}
	s, _ := v.R.([]Value)
	b := make([]byte, len(s))
	for i := range s {
		b[i] = byte(s[i].N)
	}
	return b
}

func valStr(v Value) string { s, _ := v.R.(string); return s }

var goIntrinsics = map[string]func(a []Value) Value{
	"internal/abi.NoEscape": func(a []Value) Value { return a[0] },
	"internal/abi.Escape":   func(a []Value) Value { return a[0] },
	"internal/bytealg.IndexByteString": func(a []Value) Value {
		return Value{N: int64(strings.IndexByte(valStr(a[0]), byte(a[1].N)))}
	},
	"internal/bytealg.IndexByte": func(a []Value) Value {
		if b, ok := a[0].R.([]byte); ok {
			return Value{N: int64(bytes.IndexByte(b, byte(a[1].N)))}
		}
		s, _ := a[0].R.([]Value)
		for i := range s {
			if byte(s[i].N) == byte(a[1].N) {
				return Value{N: int64(i)}
			}
		}
		return Value{N: -1}
	},
	"internal/bytealg.IndexString": func(a []Value) Value {
		return Value{N: int64(strings.Index(valStr(a[0]), valStr(a[1])))}
	},
	"internal/bytealg.Index": func(a []Value) Value {
		return Value{N: int64(bytes.Index(valBytes(a[0]), valBytes(a[1])))}
	},
	"internal/bytealg.CountString": func(a []Value) Value {
		return Value{N: int64(strings.Count(valStr(a[0]), string(rune(byte(a[1].N)))))}
	},
	"internal/bytealg.Count": func(a []Value) Value {
		if b, ok := a[0].R.([]byte); ok {
			return Value{N: int64(bytes.Count(b, []byte{byte(a[1].N)}))}
		}
		s, _ := a[0].R.([]Value)
		n := 0
		for i := range s {
			if byte(s[i].N) == byte(a[1].N) {
				n++
			}
		}
		return Value{N: int64(n)}
	},
	"internal/bytealg.Compare": func(a []Value) Value {
		return Value{N: int64(bytes.Compare(valBytes(a[0]), valBytes(a[1])))}
	},
	"internal/bytealg.abigen_runtime_cmpstring": func(a []Value) Value {
		return Value{N: int64(strings.Compare(valStr(a[0]), valStr(a[1])))}
	},
	"internal/bytealg.MakeNoZero": func(a []Value) Value {
		return Value{R: make([]Value, a[0].N)}
	},
	// maps.clone(m any) any is linked to the runtime. maps.Clone calls it
	// with a non-nil map in an interface.
	"maps.clone": func(a []Value) Value {
		iv, _ := a[0].R.(*Iface)
		if iv == nil {
			return a[0]
		}
		var out Value
		switch mp := iv.V.R.(type) {
		case *MapV:
			// Map elements are values that the VM never changes in place:
			// a shallow copy of both tables is enough.
			c := newMapV(len(mp.M))
			for k, v := range mp.M {
				c.M[k] = v
			}
			if mp.K != nil {
				c.K = make(map[any]Value, len(mp.K))
				for k, v := range mp.K {
					c.K[k] = v
				}
			}
			out = Value{R: c}
		case reflect.Value: // map of a native named type (http.Header{})
			c := reflect.MakeMapWithSize(mp.Type(), mp.Len())
			for it := mp.MapRange(); it.Next(); {
				c.SetMapIndex(it.Key(), it.Value())
			}
			out = Value{R: c}
		default:
			return a[0]
		}
		return Value{R: &Iface{T: iv.T, V: out}}
	},
}
