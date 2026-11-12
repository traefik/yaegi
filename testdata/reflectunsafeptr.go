package main

// Value.UnsafePointer of a host object converts back to the object, and
// Value.Pointer of a native slice is the address of its data.

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"unsafe"
)

func main() {
	r, _ := http.NewRequest("GET", "/", nil)
	var up unsafe.Pointer = reflect.ValueOf(r).UnsafePointer()
	rp := (*http.Request)(up)
	fmt.Println(rp == r, rp.Method)

	b := bytes.NewBufferString("hello")
	buf := reflect.ValueOf(b).Elem().Field(0) // the unexported []byte
	fmt.Println(buf.Pointer() != 0, buf.Pointer() == uintptr(buf.UnsafePointer()), buf.Len())
	fmt.Println(*(*byte)(buf.UnsafePointer()) == 'h')
}
