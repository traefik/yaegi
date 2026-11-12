package vm

// Native slice views. A slice that comes from native memory and whose
// elements are scalars (strings, numbers, bools) is kept in the VM as a
// reflect.Value of kind Slice, a view of the native array, and not copied
// into a new []Value. Writes through it reach the native array, as in Go:
// req.Header["X"][0] = v, the loop `for _, vs := range req.Header { vs[i] =
// ... }`, url.Values["k"][0] = v, tls.Config.NextProtos[0] = v. Before, the
// VM worked on a copy and these writes were silently lost.
//
// The operations on slices accept a view: index load and store (indexAddr
// gives a reflect pointer to the element), len, cap, slicing (OpSlice),
// append, copy, clear, conversion to an array pointer, comparison with nil.
// Passed back to native code, a view is the native slice itself
// (toNativeW). A view is never nil: a nil native slice converts to the nil
// Value, as before.
//
// Reflect on a variable that holds a view (decoders of the AOT copies, the
// program's own reflect): the twin changes the header (SetLen, SetCap,
// Grow, Set, SetZero) on a settable copy and stores the new view back into
// the variable (RV.setNat). With opaque handles, a pointer to such a
// variable given to native code is a pointer to a shadow of the header
// (boxAs).
//
// Byte slices are not views: they stay a Go []byte (bytes.go). Slices of
// other element types (structs, pointers, slices, interfaces) are still
// copied: their elements need a conversion that a view cannot do in place.

import (
	"go/types"
	"math"
	"reflect"
	"unsafe"
)

// scalarKind reports whether native elements of kind k convert to a VM
// value without a type: a bool, a number or a string. Bytes are []byte
// (bytes.go), not views.
func scalarKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return true
	}
	return false
}

// isNatView reports whether rv is a native slice view: a slice of scalars.
func isNatView(rv reflect.Value) bool {
	return rv.Kind() == reflect.Slice && scalarKind(rv.Type().Elem().Kind())
}

// natViewable reports whether rv, a native slice (not nil) converted to the
// interpreted slice type with element type et, can stay a view.
func natViewable(rv reflect.Value, et types.Type) bool {
	if !scalarKind(rv.Type().Elem().Kind()) {
		return false
	}
	b, ok := et.Underlying().(*types.Basic)
	return ok && b.Kind() != types.UnsafePointer
}

// natView returns a view of the native slice rv: a slice header of its own,
// with the same array, length and capacity. rv itself may be the header
// stored in native memory (a struct field such as tls.Config.NextProtos):
// the view must not change when native code later stores another slice
// there, as a Go variable that holds a copy of the field does not. A slice
// read through an unexported field is rebuilt without the read-only flag
// of reflect, so that the VM can write its elements, as it can write the
// elements of a []Value copy.
func natView(rv reflect.Value) reflect.Value {
	if rv.CanInterface() {
		return rv.Slice3(0, rv.Len(), rv.Cap())
	}
	p := reflect.New(rv.Type())
	*(*sliceHeader)(p.UnsafePointer()) = sliceHeader{rv.UnsafePointer(), rv.Len(), rv.Cap()}
	return p.Elem().Slice3(0, rv.Len(), rv.Cap())
}

// sliceHeader is the memory layout of a Go slice.
type sliceHeader struct {
	data     unsafe.Pointer
	len, cap int
}

// natScalar returns the VM value of the native scalar e (scalarKind).
func natScalar(e reflect.Value) Value {
	switch e.Kind() {
	case reflect.String:
		return Value{R: e.String()}
	case reflect.Bool:
		return Value{N: b2i(e.Bool())}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Value{N: e.Int()}
	case reflect.Float32, reflect.Float64:
		return Value{N: fv(e.Float())}
	case reflect.Complex64, reflect.Complex128:
		return Value{R: e.Complex()}
	}
	return Value{N: int64(e.Uint())}
}

// setNatScalar stores the VM value v into the native scalar e (scalarKind),
// which must be settable.
func setNatScalar(e reflect.Value, v Value) {
	switch e.Kind() {
	case reflect.String:
		e.SetString(valStr(v))
	case reflect.Bool:
		e.SetBool(v.N != 0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.SetInt(v.N)
	case reflect.Float32, reflect.Float64:
		e.SetFloat(math.Float64frombits(uint64(v.N)))
	case reflect.Complex64, reflect.Complex128:
		e.SetComplex(cplx(v))
	default:
		e.SetUint(uint64(v.N))
	}
}

// natViewOf returns the native slice view held by r, if any.
func natViewOf(r any) (reflect.Value, bool) {
	rv, ok := r.(reflect.Value)
	if !ok || !rv.IsValid() || !isNatView(rv) {
		return reflect.Value{}, false
	}
	return rv, true
}

// viewValue returns the VM value of the native slice s, the result of an
// operation on views: the nil Value for a nil slice.
func viewValue(s reflect.Value) Value {
	if s.IsNil() {
		return Value{}
	}
	return Value{R: s}
}

// appendNat implements append(s, t...) when s or t is a native slice view.
// An append within the capacity of a view writes into the native array,
// as in Go; reflect.Append grows the array as the append of gc. ok is false
// when neither s nor t is a view.
func appendNat(s, t any) (_ Value, ok bool) {
	sv, sok := natViewOf(s)
	tv, tok := natViewOf(t)
	switch {
	case sok:
		switch t := t.(type) {
		case reflect.Value:
			if !tok {
				return Value{}, false
			}
			if tv.Type() != sv.Type() {
				tv = tv.Convert(sv.Type())
			}
			return viewValue(reflect.AppendSlice(sv, tv)), true
		case []Value:
			xs := make([]reflect.Value, len(t))
			et := sv.Type().Elem()
			for i := range t {
				xs[i] = reflect.New(et).Elem()
				setNatScalar(xs[i], t[i])
			}
			return viewValue(reflect.Append(sv, xs...)), true
		case nil:
			return Value{R: sv}, true
		}
		return Value{}, false
	case tok:
		switch s := s.(type) {
		case nil:
			// append(nil, view...): a new native array, as large as gc
			// makes it.
			return viewValue(reflect.AppendSlice(reflect.Zero(tv.Type()), tv)), true
		case []Value:
			for i := range tv.Len() {
				s = append(s, natScalar(tv.Index(i)))
			}
			return Value{R: s}, true
		}
	}
	return Value{}, false
}

// copyNat implements copy(dst, src) when dst or src is a native slice view.
// ok is false when neither is a view.
func copyNat(dst, src any) (n int, ok bool) {
	dv, dok := natViewOf(dst)
	sv, sok := natViewOf(src)
	if dok {
		curMachine().hostWriteIn(dv)
	}
	switch {
	case dok && sok:
		if sv.Type() != dv.Type() {
			sv = sv.Convert(dv.Type())
		}
		return reflect.Copy(dv, sv), true
	case dok:
		s, _ := src.([]Value)
		n = min(dv.Len(), len(s))
		for i := range n {
			setNatScalar(dv.Index(i), s[i])
		}
		return n, true
	case sok:
		d, _ := dst.([]Value)
		n = min(len(d), sv.Len())
		for i := range n {
			d[i] = natScalar(sv.Index(i))
		}
		return n, true
	}
	return 0, false
}
