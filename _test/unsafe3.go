package main

import (
	"math/bits"
	"unsafe"
)

const (
	SSize = 16
	WSize = bits.UintSize / 8
)

type S struct {
	X int
	Y int
}

func main() {
	bigEndian := (*(*[2]uint8)(unsafe.Pointer(&[]uint16{1}[0])))[0] == 0
	var sBuf [SSize]byte
	s := (*S)(unsafe.Pointer(&sBuf[0]))

	s.X = 2
	s.Y = 4

	if bigEndian {
		println(sBuf[0+WSize-1], sBuf[WSize+WSize-1])
	} else {
		println(sBuf[0], sBuf[WSize])
	}
}

// Output:
// 2 4
