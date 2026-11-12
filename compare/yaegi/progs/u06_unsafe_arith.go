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
	fmt.Println(*(*int32)(unsafe.Add(p, unsafe.Offsetof(s.B))))
	c1 := (*int64)(unsafe.Pointer(uintptr(p) + unsafe.Offsetof(s.C) + 8))
	fmt.Println(*c1)
	*c1 = 99
	fmt.Println(s.C)
	fmt.Println(*(*string)(unsafe.Add(p, unsafe.Offsetof(s.D))))
	arr := []int64{5, 6, 7, 8}
	q := unsafe.Pointer(&arr[0])
	for i := 0; i < len(arr); i++ {
		fmt.Print(*(*int64)(unsafe.Add(q, uintptr(i)*unsafe.Sizeof(arr[0]))), " ")
	}
	fmt.Println()
	fmt.Println(unsafe.Pointer(&s) == unsafe.Pointer(&s.A))
	fmt.Println(uintptr(unsafe.Pointer(&arr[1])) - uintptr(unsafe.Pointer(&arr[0])))
}
