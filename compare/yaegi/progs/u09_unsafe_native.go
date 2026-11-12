package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

type T struct{ A, B int64 }

func main() {
	t := &T{1, 2}
	// Native reflect builds a typed pointer from an unsafe.Pointer.
	v := reflect.NewAt(reflect.TypeOf(T{}), unsafe.Pointer(t)).Elem()
	fmt.Println(v.Field(1).Int())
	v.Field(0).SetInt(10)
	fmt.Println(t.A)
	b := []byte{1, 2, 3, 4}
	h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	fmt.Println(h.Len, h.Cap)
	fmt.Println(reflect.ValueOf(unsafe.Pointer(t)).Kind())
}
