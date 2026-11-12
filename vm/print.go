package vm

import (
	"go/types"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"
)

// This file formats the arguments of the print and println builtins like the
// gc runtime does (runtime/print.go): floats in the shortest form for their
// size, pointers, maps, chans and funcs as hex addresses, slices as
// [len/cap]address and interfaces as (type,data). Addresses of non-nil values
// are real Go addresses of the VM objects, so they differ from gc, but nil
// values print 0x0 exactly as in gc, and non-nil values never print 0x0.

// printOut returns where print and println write: Machine.PrintOut, or
// the standard error like the gc runtime (SetStdio, else os.Stderr).
func (m *Machine) printOut() io.Writer {
	if m.PrintOut != nil {
		return m.PrintOut
	}
	if m.stderr != nil {
		return m.stderr
	}
	return os.Stderr
}

// writePrint writes one print or println call to w, in a single write. The
// arguments are vals[idx[i]], or vals[i] when idx is nil: OpPrintln reads the
// register file directly, without a copy of the arguments.
func writePrint(w io.Writer, vals []Value, idx []int32, ts []types.Type, ln bool) {
	var sb strings.Builder
	for i, t := range ts {
		if ln && i > 0 {
			sb.WriteByte(' ')
		}
		var v Value
		if idx != nil {
			v = vals[idx[i]]
		} else {
			v = vals[i]
		}
		sb.WriteString(formatPrint(v, t))
	}
	if ln {
		sb.WriteByte('\n')
	}
	io.WriteString(w, sb.String())
}

func printHex(p uintptr) string { return "0x" + strconv.FormatUint(uint64(p), 16) }

// formatPrint formats one argument of print or println.
func formatPrint(v Value, t types.Type) string {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Kind() == types.Float32:
			return strconv.FormatFloat(float64(float32(f64(v))), 'g', -1, 32)
		case u.Info()&types.IsFloat != 0:
			return strconv.FormatFloat(f64(v), 'g', -1, 64)
		case u.Kind() == types.Complex64:
			return strconv.FormatComplex(cplx(v), 'g', -1, 64)
		case u.Info()&types.IsComplex != 0:
			return strconv.FormatComplex(cplx(v), 'g', -1, 128)
		}
		return formatBasic(v, t)
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature:
		return printHex(refAddr(v.R))
	case *types.Slice:
		return formatPrintSlice(v.R)
	case *types.Interface:
		return formatPrintIface(v.R)
	}
	return formatBasic(v, t)
}

// efaceData returns the data word of the interface value x. It is 0 only
// when x is nil or holds a nil pointer-shaped value (pointer, map, chan,
// func). For other values it is the address of the boxed copy. So it is the
// real address for the reference shapes of the VM (*Value, *byte, *MapV,
// *Closure, chan Value, native maps and funcs...), and it is never 0 for a
// non-nil value of a shape that this file does not know.
func efaceData(x any) uintptr {
	return uintptr(efaceWord(&x))
}

// refAddr returns the Go address behind a VM reference value (pointer, map,
// chan, func), or 0 for nil.
func refAddr(r any) uintptr {
	switch p := r.(type) {
	case reflect.Value:
		return valueAddr(p)
	case badAddr, roByte, *hdrView, *hdrWord, *byteView:
		return addrOf(Value{R: p})
	}
	return efaceData(r)
}

// valueAddr returns the data word that gc stores when the native value rv
// is put in an interface: the address behind a reference value (0 for nil),
// and for a value that is not pointer-shaped (an int, a string, a struct of
// several fields...) the address of a copy, which is never 0.
func valueAddr(rv reflect.Value) uintptr {
	if !rv.IsValid() {
		return 0
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return rv.Pointer()
	case reflect.Interface:
		if rv.IsNil() {
			return 0
		}
		return valueAddr(rv.Elem())
	case reflect.Struct:
		if rv.NumField() == 1 && directIface(rv.Type().Field(0).Type) {
			return valueAddr(rv.Field(0))
		}
	case reflect.Array:
		if rv.Len() == 1 && directIface(rv.Type().Elem()) {
			return valueAddr(rv.Index(0))
		}
	}
	if rv.CanAddr() {
		return rv.UnsafeAddr()
	}
	if rv.CanInterface() {
		return efaceData(rv.Interface())
	}
	return efaceData(rv) // a boxed copy of the reflect.Value: not 0
}

// directIface reports whether gc stores a value of type t directly in the
// data word of an interface (IsDirectIface in cmd/compile): pointer-shaped
// types, and structs and arrays of exactly one such element.
func directIface(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Struct:
		return t.NumField() == 1 && directIface(t.Field(0).Type)
	case reflect.Array:
		return t.Len() == 1 && directIface(t.Elem())
	}
	return false
}

// vmIfaceData returns the data word of an interpreted value v of dynamic
// type t, like valueAddr does for native values. ok is false when t is not
// pointer-shaped: gc then stores the address of a copy.
func vmIfaceData(v Value, t types.Type) (p uintptr, ok bool) {
	switch u := t.Underlying().(type) {
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature:
		return refAddr(v.R), true
	case *types.Basic:
		if u.Kind() == types.UnsafePointer {
			return addrOf(v), true
		}
	case *types.Struct:
		if u.NumFields() == 1 {
			return oneElemData(v, u.Field(0).Type())
		}
	case *types.Array:
		if u.Len() == 1 {
			return oneElemData(v, u.Elem())
		}
	}
	return 0, false
}

// oneElemData is vmIfaceData for a struct or an array of one element of
// type et.
func oneElemData(v Value, et types.Type) (uintptr, bool) {
	switch x := v.R.(type) {
	case []Value:
		if len(x) == 1 {
			return vmIfaceData(x[0], et)
		}
	case reflect.Value:
		if x.IsValid() && directIface(x.Type()) {
			return valueAddr(x), true
		}
	}
	return 0, false
}

func formatPrintSlice(r any) string {
	var n, c int
	var p uintptr
	switch s := r.(type) {
	case nil:
	case []Value:
		n, c, p = len(s), cap(s), uintptr(unsafe.Pointer(unsafe.SliceData(s)))
	case []byte:
		n, c, p = len(s), cap(s), uintptr(unsafe.Pointer(unsafe.SliceData(s)))
	default:
		rv, ok := r.(reflect.Value)
		if !ok {
			rv = reflect.ValueOf(r)
		}
		switch {
		case !rv.IsValid(): // an empty reflect.Value: a nil slice
		case rv.Kind() == reflect.Slice:
			n, c, p = rv.Len(), rv.Cap(), rv.Pointer()
		default:
			// Do not print an unknown shape like a nil slice.
			unsupported("print of a slice held as %T", r)
		}
	}
	return "[" + strconv.Itoa(n) + "/" + strconv.Itoa(c) + "]" + printHex(p)
}

// formatPrintIface prints (type,data) like printeface and printiface. Here
// "type" is the address of the type descriptor of the dynamic type (the VM
// *RType, or the native *rtype), and "data" is 0x0 for a nil pointer-shaped
// value (a typed nil), else the address of the value.
func formatPrintIface(r any) string {
	var tp, dp uintptr
	switch iv := r.(type) {
	case nil:
	case *Iface:
		if iv == nil {
			break
		}
		tp = uintptr(unsafe.Pointer(iv.T))
		if iv.T.T == nil { // native dynamic type
			dp = refAddr(iv.V.R)
			break
		}
		if a, ok := vmIfaceData(iv.V, iv.T.T); ok {
			dp = a
		} else {
			dp = uintptr(unsafe.Pointer(&iv.V)) // the boxed payload
		}
	case reflect.Value:
		if iv.IsValid() && iv.Kind() == reflect.Interface {
			iv = iv.Elem() // the dynamic value (invalid for a nil interface)
		}
		if iv.IsValid() {
			tp = reflect.ValueOf(iv.Type()).Pointer() // the native *rtype
			dp = valueAddr(iv)
		}
	default:
		w := (*[2]uintptr)(unsafe.Pointer(&r)) // the eface words of a native value
		tp, dp = w[0], w[1]
	}
	return "(" + printHex(tp) + "," + printHex(dp) + ")"
}

// String conversions with runes. The interpreter (OpRune2Str, OpStr2Runes,
// OpRunes2Str) and the twin reflect (twin_value.go) share these helpers.

// isRuneSlice reports whether t (an underlying type) is []rune, that is a
// slice of a type whose underlying type is int32.
func isRuneSlice(t types.Type) bool {
	s, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	b := basic(s.Elem())
	return b != nil && b.Kind() == types.Int32
}

// intToStr converts an integer to a string like Go: values that are not
// valid code points (negative, too large, or a surrogate half) give "�".
// The integer is not cut to 32 bits first: string(int64(1<<32 + 'A')) is
// "�", not "A". An unsigned value above MaxInt64 is negative here, so it
// gives "�" too.
func intToStr(n int64) string {
	if n < 0 || n > utf8.MaxRune {
		return string(utf8.RuneError)
	}
	return string(rune(n))
}

// strToRuneVals converts a string to the VM representation of a []rune.
func strToRuneVals(s string) []Value {
	out := make([]Value, 0, utf8.RuneCountInString(s))
	for _, r := range s {
		out = append(out, Value{N: int64(r)})
	}
	return out
}

// strToRunes converts a string to a []rune (stored as []Value).
func strToRunes(v Value) Value {
	s, _ := v.R.(string)
	return Value{R: strToRuneVals(s)}
}

// runesString converts n runes, given by at, to a string. Invalid runes
// give "�".
func runesString(n int, at func(i int) rune) string {
	var sb strings.Builder
	sb.Grow(n) // at least one byte per rune
	for i := 0; i < n; i++ {
		sb.WriteRune(at(i)) // "�" for an invalid rune
	}
	return sb.String()
}

// runesToStr converts a []rune to a string. Invalid runes give "�".
func runesToStr(v Value) Value {
	switch rs := v.R.(type) {
	case []Value:
		return Value{R: runesString(len(rs), func(i int) rune { return rune(rs[i].N) })}
	case []rune:
		return Value{R: string(rs)}
	case reflect.Value:
		if rs.IsValid() && rs.Kind() == reflect.Slice {
			return Value{R: runesString(rs.Len(), func(i int) rune { return rune(rs.Index(i).Int()) })}
		}
	}
	return Value{R: ""}
}
