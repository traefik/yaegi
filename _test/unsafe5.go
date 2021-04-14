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
	x := S{}
	size := unsafe.Sizeof(x)
	align := unsafe.Alignof(x.Y)
	offset := unsafe.Offsetof(x.Z)

	fmt.Println(size, align, offset)
}

// Output:
// 24 8 16
