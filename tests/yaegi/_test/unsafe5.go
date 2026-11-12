package main

import (
	"math/bits"
	"unsafe"
)

const WSize = bits.UintSize / 8

type S struct {
	X int
	Y int
	Z int
}

func main() {
	x := S{}
	size := unsafe.Sizeof(x) / WSize
	align := unsafe.Alignof(x.Y) / WSize
	offset := unsafe.Offsetof(x.Z) / WSize

	println(size, align, offset)
}

// Output:
// 3 1 2
