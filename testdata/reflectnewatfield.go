package main

// reflect.NewAt of the address given by Value.UnsafeAddr, in interpreted
// code: the idiom that sets an unexported field (openai-go and
// anthropic-sdk-go, apijson.setUnexportedField).

import (
	"fmt"
	"reflect"
	"unsafe"
)

type meta struct{ raw string }

type T struct {
	X    int
	JSON meta
}

func setUnexportedField(field reflect.Value, value any) {
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func main() {
	var t T
	v := reflect.ValueOf(&t).Elem()
	setUnexportedField(v.Field(1).Field(0), "hello")
	fmt.Println(t.JSON.raw)
}
