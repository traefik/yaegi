package main

import (
	"fmt"
	"unsafe"
)

type S struct {
	X int
	Y int
	Z int
}

func main() {
	x := S{Z: 5}
	ptr := unsafe.Pointer(&x)
	offset := int(unsafe.Offsetof(x.Z))
	p := unsafe.Add(ptr, offset)

	i := *(*int)(p)

	fmt.Println(i)
}
