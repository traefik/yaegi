// Package unsafe2 provides helpers to generate recursive struct types.
package unsafe2

import (
	"reflect"
	"unsafe"
)

type dummy struct{}

// DummyType represents a stand-in for a recursive type.
var DummyType = reflect.TypeOf(dummy{})

// the following type sizes must match their original definition in Go src/reflect/type.go.

type rtype struct {
	_ uintptr
	_ uintptr
	_ uint32
	_ uint32
	_ uintptr
	_ uintptr
	_ uint32
	_ uint32
}

type emptyInterface struct {
	typ *rtype
	_   unsafe.Pointer
}

type structField struct {
	_   uintptr
	typ *rtype
	_   uintptr
}

type structType struct {
	rtype
	_      uintptr
	fields []structField
}

// SetFieldType sets the type of the struct field at the given index, to the given type.
//
// The struct type must have been created at runtime. This is very unsafe.
func SetFieldType(s reflect.Type, idx int, t reflect.Type) {
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
