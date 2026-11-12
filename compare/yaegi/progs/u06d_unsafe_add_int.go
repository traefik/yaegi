package main

import (
	"fmt"
	"unsafe"
)

type S struct {
	A byte
	B int32
	D string
}

func main() {
	s := S{A: 1, B: 2, D: "d"}
	p := unsafe.Pointer(&s)
	fmt.Println(*(*int32)(unsafe.Add(p, int(unsafe.Offsetof(s.B)))))
	fmt.Println(*(*string)(unsafe.Add(p, int(unsafe.Offsetof(s.D)))))
}
