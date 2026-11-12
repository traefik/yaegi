package main

import (
	"fmt"
	"unsafe"
)

type A struct {
	X int64
	Y string
	Z []int
}

type B struct {
	U int64
	V string
	W []int
}

type Raw [16]byte

type Hdr struct {
	Magic uint32
	Len   uint32
	ID    uint64
}

func main() {
	a := &A{1, "y", []int{1, 2}}
	b := (*B)(unsafe.Pointer(a))
	b.U = 42
	fmt.Println(a.X, b.V, b.W)
	as := []A{{1, "a", nil}, {2, "b", nil}}
	bs := *(*[]B)(unsafe.Pointer(&as))
	fmt.Println(len(bs), bs[1].V)
	var r Raw
	r[0], r[4], r[8] = 7, 9, 1
	h := (*Hdr)(unsafe.Pointer(&r))
	fmt.Println(h.Magic, h.Len, h.ID)
}
