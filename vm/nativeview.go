package vm

import (
	"go/types"
	"reflect"
	"unsafe"
)

// A value of a native type (a type of a package that runs natively, such as
// syscall.RawSockaddrAny) lives in native memory with the layout of gc. So
// (*T)(unsafe.Pointer(&v)) can be the same memory seen as a T, as in gc, when
// both types are plain data (no pointer, string, slice, map, chan, func or
// interface word: a write through one view can never make a pointer the
// garbage collector does not know) and T fits inside the value: its size is
// not larger, and the address of the value is aligned for T. The socket
// address types of package syscall and golang.org/x/sys/unix are used this
// way ((*syscall.RawSockaddrInet4)(unsafe.Pointer(&rsa)) for a
// RawSockaddrAny).
//
// A native T is the native value at the same address (reflect.NewAt); an
// interpreted T of plain data is a view of the bytes of the native value
// (byteview.go). In the same way, a native T of plain data over real bytes
// (a Go []byte: (*unix.RawSockaddrInet4)(unsafe.Pointer(&b[i])), as
// golang.org/x/net and x/sys parse socket messages) is the native value at
// the address of the bytes, inside their array (nativeBytesView). Everything
// else stays refused ("needs byte-level memory").
// Only the public reflect API is used: no layout of the runtime.

// nativeView returns p, a pointer to a native value, seen as a *to, or false
// when p does not point to a native value or to is its type. It refuses
// (unsupported) a native value that to cannot view.
func (m *Machine) nativeView(p Value, from, to types.Type) (Value, bool) {
	var base unsafe.Pointer
	var src reflect.Type
	switch q := p.R.(type) {
	case *Value:
		rv, ok := q.R.(reflect.Value)
		if !ok || !rv.IsValid() || !rv.CanAddr() {
			return Value{}, false
		}
		base, src = rv.Addr().UnsafePointer(), rv.Type()
	case reflect.Value:
		if !q.IsValid() || q.Kind() != reflect.Pointer || q.IsNil() {
			return Value{}, false
		}
		base, src = q.UnsafePointer(), q.Type().Elem()
	default:
		return Value{}, false
	}
	if rt := m.tryReflectType(to); rt == src {
		return Value{}, false // the same type: p itself
	}
	name := src.String()
	if from != nil {
		name = from.String()
	}
	if m.restricted && hostMem().has(uintptr(base)) {
		// A variable of the host (crc32.IEEETable): a view of another type
		// could reach native code, which would write into it (hostmem.go).
		unsupported("unsafe: viewing a %s as a %s: the value is memory of the host", name, to)
	}
	why := "the native type " + src.String() + " is not plain data (integers, floats, booleans, structs and arrays of them)"
	if plainNative(src) {
		why = "the type " + to.String() + " is not plain data"
		switch {
		case m.isNativeNamed(to):
			rt := m.tryReflectType(to)
			if rt == nil || !plainNative(rt) {
				break
			}
			if rt.Size() > src.Size() || uintptr(base)%uintptr(rt.Align()) != 0 {
				why = "it does not fit inside the value (size or alignment)"
				break
			}
			return Value{R: reflect.NewAt(rt, base)}, true
		case m.isPOD(to):
			size, align := gcSizes.Sizeof(to), gcSizes.Alignof(to)
			if size > int64(src.Size()) || uintptr(base)%uintptr(align) != 0 {
				why = "it does not fit inside the value (size or alignment)"
				break
			}
			b := unsafe.Slice((*byte)(base), src.Size())
			return Value{R: &byteView{b: b, t: to}}, true
		}
	}
	unsupported("unsafe: viewing a %s as a %s needs byte-level memory: %s", name, to, why)
	return Value{}, false
}

// nativePlainType returns the native type of t when t is a native type of
// plain data (plainNative), nil otherwise.
func (m *Machine) nativePlainType(t types.Type) reflect.Type {
	if !m.isNativeNamed(t) {
		return nil
	}
	if rt := m.tryReflectType(t); rt != nil && plainNative(rt) {
		return rt
	}
	return nil
}

// nativeBytesView returns the native value of type rt (plain data) at offset
// off of the bytes of r, or the reason why there is none: the value must be
// inside the array, at an address aligned for rt, and the bytes must be
// writable (the bytes of a string outside the heap are read-only memory,
// where a store through a native pointer would stop the process), and under
// the restrictions of plugins they must not be memory of the host
// (hostmem.go): native code that gets the pointer would write into it.
func (m *Machine) nativeBytesView(r *uregion, off int, rt reflect.Type) (Value, string) {
	if off < 0 || uint64(off)+uint64(rt.Size()) > uint64(len(r.bytes)) || rt.Size() == 0 {
		return Value{}, "reaches outside the memory known to the VM"
	}
	if r.ro {
		return Value{}, "would be in read-only memory (the bytes of a string constant)"
	}
	p := unsafe.Pointer(&r.bytes[off])
	if uintptr(p)%uintptr(rt.Align()) != 0 {
		return Value{}, "is not at an address aligned for it"
	}
	if m.restricted && hostMem().has(uintptr(p)) {
		return Value{}, "would be memory of the host"
	}
	return Value{R: reflect.NewAt(rt, p)}, ""
}

// plainNative reports whether values of the native type t are plain data:
// booleans, integers (also uintptr) and floats, and structs and arrays of
// them, with unexported fields too.
func plainNative(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Array:
		return plainNative(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if !plainNative(t.Field(i).Type) {
				return false
			}
		}
		return true
	}
	return false
}
