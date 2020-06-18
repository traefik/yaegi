package main

import (
	"fmt"
	"unsafe"
)

func main() {
	str := "foobar"

	ptr := unsafe.Pointer(&str)
	p := uintptr(ptr)

	s1 := fmt.Sprintf("%x", ptr)
	s2 := fmt.Sprintf("%x", p)
	println(s1 == s2)
}

// Output:
// true
