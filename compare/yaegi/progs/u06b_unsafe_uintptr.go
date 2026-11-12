package main

import (
	"fmt"
	"unsafe"
)

type S struct {
	A byte
	B int32
	C [3]int64
	D string
}

func main() {
	s := S{A: 1, B: 2, C: [3]int64{10, 20, 30}, D: "d"}
	p := unsafe.Pointer(&s)
	c1 := (*int64)(unsafe.Pointer(uintptr(p) + unsafe.Offsetof(s.C) + 8))
	fmt.Println(*c1)
	*c1 = 99
	fmt.Println(s.C)
	arr := []int64{5, 6, 7, 8}
	fmt.Println(*(*int64)(unsafe.Pointer(uintptr(unsafe.Pointer(&arr[0])) + 2*unsafe.Sizeof(arr[0]))))
}
