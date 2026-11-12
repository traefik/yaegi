package main

import "unsafe"

type T struct {
	i uint64
}

var d T

var b [unsafe.Sizeof(d)]byte

func main() {
	println(len(b))
}

// Output:
// 8
