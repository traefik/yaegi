package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

type T struct{ A, B int64 }

func main() {
	t := &T{1, 2}
	v := reflect.NewAt(reflect.TypeOf(T{}), unsafe.Pointer(t)).Elem()
	fmt.Println(v.Field(1).Int())
	v.Field(0).SetInt(10)
	fmt.Println(t.A)
}
