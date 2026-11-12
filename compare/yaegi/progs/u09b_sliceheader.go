package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

func main() {
	b := []byte{1, 2, 3, 4}
	h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	fmt.Println(h.Len, h.Cap)
	s := "abc"
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	fmt.Println(sh.Len)
}
