package main

import "unsafe"

func main() {
	str := "foobar"

	p := unsafe.Pointer(&str)
	str2 := *(*string)(p)

	println(str2)
}

// Output:
// foobar
