package unsafe2

import (
	"reflect"
	"unsafe"
)

type dummy struct{}

// DummyType represents a stand-in for a recursive type.
var DummyType = reflect.TypeOf(dummy{})

type rtype struct {
	_ [48]byte
}

type emptyInterface struct {
	typ *rtype
	_   unsafe.Pointer
}

type structField struct {
	_   int64
	typ *rtype
	_   uintptr
}

type structType struct {
	rtype
	_      int64
	fields []structField
}

// SwapFieldType swaps the type of the struct field with the given type.
//
// The struct type must have been created at runtime. This is very unsafe.
func SwapFieldType(s reflect.Type, idx int, t reflect.Type) {
	if s.Kind() != reflect.Struct || idx >= s.NumField() {
		return
	}

	rtyp := unpackType(s)
	styp := (*structType)(unsafe.Pointer(rtyp))
	f := styp.fields[idx]
	f.typ = unpackType(t)
	styp.fields[idx] = f
}

func unpackType(t reflect.Type) *rtype {
	v := reflect.New(t).Elem().Interface()
	return (*emptyInterface)(unsafe.Pointer(&v)).typ
}
