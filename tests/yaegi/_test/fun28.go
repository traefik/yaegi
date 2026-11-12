package main

import (
	"runtime"
)

type T struct {
	name string
}

func finalize(t *T) { println("finalize") }

func newT() *T {
	t := new(T)
	runtime.SetFinalizer(t, finalize)
	return t
}

func main() {
	t := newT()
	println(t != nil)
}

// Output:
// true
