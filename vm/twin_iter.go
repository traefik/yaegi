package vm

// Go-level twin of package reflect: iterators (Go 1.23+ / 1.26 API) and
// native-only entry points.

import (
	"iter"
	"reflect"
	"unsafe"
)

func (vt *vtype) Methods() iter.Seq[RMethod] {
	return func(yield func(RMethod) bool) {
		for i := 0; i < vt.NumMethod(); i++ {
			if !yield(vt.Method(i)) {
				return
			}
		}
	}
}

func (vt *vtype) Fields() iter.Seq[StructField] {
	return func(yield func(StructField) bool) {
		for i := 0; i < vt.NumField(); i++ {
			if !yield(vt.Field(i)) {
				return
			}
		}
	}
}

func (vt *vtype) Ins() iter.Seq[RT] {
	return func(yield func(RT) bool) {
		for i := 0; i < vt.NumIn(); i++ {
			if !yield(vt.In(i)) {
				return
			}
		}
	}
}

func (vt *vtype) Outs() iter.Seq[RT] {
	return func(yield func(RT) bool) {
		for i := 0; i < vt.NumOut(); i++ {
			if !yield(vt.Out(i)) {
				return
			}
		}
	}
}

// Fields mirrors reflect.Value.Fields.
func (v RV) Fields() iter.Seq2[StructField, RV] {
	return func(yield func(StructField, RV) bool) {
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if !yield(t.Field(i), v.Field(i)) {
				return
			}
		}
	}
}

// Methods mirrors reflect.Value.Methods.
func (v RV) Methods() iter.Seq2[RMethod, RV] {
	return func(yield func(RMethod, RV) bool) {
		t := v.Type()
		for i := 0; i < v.NumMethod(); i++ {
			if !yield(t.Method(i), v.Method(i)) {
				return
			}
		}
	}
}

// Seq mirrors reflect.Value.Seq.
func (v RV) Seq() iter.Seq[RV] {
	if n, ok := v.native(); ok && v.p == nil {
		return func(yield func(RV) bool) {
			for x := range n.Seq() {
				if !yield(natRV(x)) {
					return
				}
			}
		}
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(yield func(RV) bool) {
			for i := int64(0); i < v.Int(); i++ {
				if !yield(scalarOf(v.Type(), func(r RV) { r.SetInt(i) })) {
					return
				}
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(yield func(RV) bool) {
			for i := uint64(0); i < v.Uint(); i++ {
				if !yield(scalarOf(v.Type(), func(r RV) { r.SetUint(i) })) {
					return
				}
			}
		}
	case reflect.Array, reflect.Slice, reflect.String:
		return func(yield func(RV) bool) {
			for i := 0; i < v.Len(); i++ {
				if !yield(ValueOf(i)) {
					return
				}
			}
		}
	case reflect.Map:
		return func(yield func(RV) bool) {
			it := v.MapRange()
			for it.Next() {
				if !yield(it.Key()) {
					return
				}
			}
		}
	case reflect.Chan:
		return func(yield func(RV) bool) {
			for {
				x, ok := v.Recv()
				if !ok || !yield(x) {
					return
				}
			}
		}
	case reflect.Func:
		if v.Type().CanSeq() {
			return func(yield func(RV) bool) {
				rf := MakeFunc(v.Type().In(0), func(in []RV) []RV {
					return []RV{ValueOf(yield(in[0]))}
				})
				v.Call([]RV{rf})
			}
		}
	}
	panic("reflect: " + v.Type().String() + " cannot produce iter.Seq[Value]")
}

// Seq2 mirrors reflect.Value.Seq2.
func (v RV) Seq2() iter.Seq2[RV, RV] {
	if n, ok := v.native(); ok && v.p == nil {
		return func(yield func(RV, RV) bool) {
			for k, x := range n.Seq2() {
				if !yield(natRV(k), natRV(x)) {
					return
				}
			}
		}
	}
	switch v.Kind() {
	case reflect.Array, reflect.Slice:
		return func(yield func(RV, RV) bool) {
			for i := 0; i < v.Len(); i++ {
				if !yield(ValueOf(i), v.Index(i)) {
					return
				}
			}
		}
	case reflect.String:
		return func(yield func(RV, RV) bool) {
			for i, r := range v.String() {
				if !yield(ValueOf(i), ValueOf(r)) {
					return
				}
			}
		}
	case reflect.Map:
		return func(yield func(RV, RV) bool) {
			it := v.MapRange()
			for it.Next() {
				if !yield(it.Key(), it.Value()) {
					return
				}
			}
		}
	case reflect.Func:
		if v.Type().CanSeq2() {
			return func(yield func(RV, RV) bool) {
				rf := MakeFunc(v.Type().In(0), func(in []RV) []RV {
					return []RV{ValueOf(yield(in[0], in[1]))}
				})
				v.Call([]RV{rf})
			}
		}
	}
	panic("reflect: " + v.Type().String() + " cannot produce iter.Seq2[Value, Value]")
}

// SliceAt mirrors reflect.SliceAt (native types only).
func SliceAt(t RT, p unsafe.Pointer, n int) RV {
	if !t.isNat() {
		unsupported("vm reflect: SliceAt on interpreted type %s", t)
	}
	return natRV(reflect.SliceAt(t.nat, p, n))
}

// SetPointer mirrors reflect.Value.SetPointer (native values only).
func (v RV) SetPointer(x unsafe.Pointer) {
	if n, ok := v.native(); ok && v.p == nil {
		n.SetPointer(x)
		return
	}
	unsupported("vm reflect: SetPointer on interpreted value")
}

// InterfaceData mirrors reflect.Value.InterfaceData (native values only).
func (v RV) InterfaceData() [2]uintptr {
	unsupported("vm reflect: InterfaceData")
	return [2]uintptr{}
}
