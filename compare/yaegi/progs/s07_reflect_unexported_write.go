package main

import (
	"fmt"
	"net/http"
	"reflect"
	"unsafe"
)

// Writing an unexported field of a host (native) type through reflect+unsafe.
func main() {
	c := &http.Client{}
	f := reflect.ValueOf(c).Elem().FieldByName("Timeout")
	fmt.Println(f.CanSet())
	r, _ := http.NewRequest("GET", "http://x", nil)
	v := reflect.ValueOf(r).Elem().FieldByName("ctx")
	v = reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	fmt.Println(v.IsNil())
}
