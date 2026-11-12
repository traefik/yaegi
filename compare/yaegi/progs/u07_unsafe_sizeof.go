package main

import (
	"fmt"
	"unsafe"
)

type S struct {
	A byte
	B int32
	C string
	D []int
	E any
	F map[string]int
	G *S
}

func main() {
	var s S
	fmt.Println(unsafe.Sizeof(s), unsafe.Alignof(s), unsafe.Alignof(s.B))
	fmt.Println(unsafe.Offsetof(s.B), unsafe.Offsetof(s.C), unsafe.Offsetof(s.D), unsafe.Offsetof(s.E), unsafe.Offsetof(s.G))
	fmt.Println(unsafe.Sizeof(""), unsafe.Sizeof([]int{}), unsafe.Sizeof(any(nil)), unsafe.Sizeof(uintptr(0)))
	n := 3
	fmt.Println(unsafe.Sizeof([4]int16{}), unsafe.Sizeof(n))
}
