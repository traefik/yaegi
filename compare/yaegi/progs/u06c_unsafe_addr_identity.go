package main

import (
	"fmt"
	"unsafe"
)

type S struct {
	A int64
	B int64
}

func main() {
	var s S
	arr := []int64{5, 6, 7, 8}
	fmt.Println(unsafe.Pointer(&s) == unsafe.Pointer(&s.A))
	fmt.Println(uintptr(unsafe.Pointer(&arr[1])) - uintptr(unsafe.Pointer(&arr[0])))
	fmt.Println(uintptr(unsafe.Pointer(&s.B)) - uintptr(unsafe.Pointer(&s)))
}
