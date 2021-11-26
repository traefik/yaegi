//go:build go1.17
// +build go1.17

// Package unsafe provides wrapper of standard library unsafe package to be imported natively in Yaegi.
package unsafe

import (
	"errors"
	"reflect"
	"unsafe"
)

func init() {
	// Add builtin functions to unsafe.
	Symbols["unsafe/unsafe"]["Add"] = reflect.ValueOf(add)
	Symbols["unsafe/unsafe"]["Slice"] = reflect.ValueOf(slice)
}

func add(ptr unsafe.Pointer, l int) unsafe.Pointer {
	return unsafe.Pointer(uintptr(ptr) + uintptr(l))
}

type emptyInterface struct {
	_    uintptr
	data unsafe.Pointer
}

func slice(i interface{}, l int) interface{} {
	if l == 0 {
		return nil
	}

	v := reflect.ValueOf(i)
	if v.Kind() != reflect.Ptr {
		panic(errors.New("first argument to unsafe.Slice must be pointer"))
	}
	if v.IsNil() {
		panic(errors.New("unsafe.Slice: ptr is nil and len is not zero"))
	}

	if l < 0 {
		panic(errors.New("unsafe.Slice: len out of range"))
	}

	ih := *(*emptyInterface)(unsafe.Pointer(&i))

	inter := reflect.MakeSlice(reflect.SliceOf(v.Type().Elem()), l, l).Interface()
	ptr := (*emptyInterface)(unsafe.Pointer(&inter)).data
	sh := (*reflect.SliceHeader)(ptr)
	sh.Data = uintptr(ih.data)
	sh.Len = l
	sh.Cap = l

	return inter
}
