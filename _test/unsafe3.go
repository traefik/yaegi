package main

import (
	"fmt"
	"unsafe"
)

const SSize = 16

type S struct {
	X int
	Y int
}

func main() {
	var sBuf [SSize]byte
	s := (*S)(unsafe.Pointer(&sBuf[0]))

	s.X = 2
	s.Y = 4

	fmt.Println(sBuf)
}

// Output:
// [2 0 0 0 0 0 0 0 4 0 0 0 0 0 0 0]
