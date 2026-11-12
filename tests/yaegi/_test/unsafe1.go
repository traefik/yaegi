package main

import "unsafe"

type S struct {
	Name string
}

func main() {
	s := &S{Name: "foobar"}

	p := unsafe.Pointer(s)

	s2 := (*S)(p)

	println(s2.Name)
}

// Output:
// foobar
