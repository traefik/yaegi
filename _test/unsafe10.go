package main

import "unsafe"

type T struct {
	X uint64
	Y uint64
}

func f(off uintptr) { println(off) }

func main() {
	f(unsafe.Offsetof(T{}.Y))
}

// Output:
// 8
