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
	size := unsafe.Sizeof(S{})
	align := unsafe.Alignof(S{})

	fmt.Println(size, align)
}

// Output:
// 24 8
