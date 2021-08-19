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
	x := [2]S{{Z: 5}, {Z: 10}}

	s := unsafe.Slice(&x[0], 2)

	fmt.Println(s)
}
