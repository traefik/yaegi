//go:build go1.21
// +build go1.21

// Package unsafe2 provides helpers to generate recursive struct types.
package unsafe2

import (
	"reflect"
	"unsafe"
)

type dummy struct{}

// DummyType represents a stand-in for a recursive type.
var DummyType = reflect.TypeOf(dummy{})

// The following type sizes must match their original definition in Go src/internal/abi/type.go.
type abiType struct {
	_ uintptr
	_ uintptr
	_ uint32
	_ uint8
	_ uint8
	_ uint8
	_ uint8
	_ uintptr
	_ uintptr
	_ int32
	_ int32
}

type abiName struct {
	Bytes *byte
}

type abiStructField struct {
	Name   abiName
	Typ    *abiType
	Offset uintptr
}

type abiStructType struct {
	abiType
	PkgPath abiName
	Fields  []abiStructField
}

type emptyInterface struct {
	typ *abiType
	_   unsafe.Pointer
}

// SetFieldType sets the type of the struct field at the given index, to the given type.
//
// The struct type must have been created at runtime. This is very unsafe.
func SetFieldType(s reflect.Type, idx int, t reflect.Type) {
	if s.Kind() != reflect.Struct || idx >= s.NumField() {
		return
	}

	rtyp := unpackType(s)
	styp := (*abiStructType)(unsafe.Pointer(rtyp))
	f := styp.Fields[idx]
	f.Typ = unpackType(t)
	styp.Fields[idx] = f
}

func unpackType(t reflect.Type) *abiType {
	v := reflect.New(t).Elem().Interface()
	eface := *(*emptyInterface)(unsafe.Pointer(&v))
	return eface.typ
}
