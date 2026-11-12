package main

import (
	"fmt"
	"unsafe"
)

func b2s(b []byte) string { return *(*string)(unsafe.Pointer(&b)) }

func s2b(s string) (b []byte) {
	*(*string)(unsafe.Pointer(&b)) = s
	(*[3]int)(unsafe.Pointer(&b))[2] = len(s)
	return b
}

func main() {
	fmt.Println(b2s([]byte("zero copy")))
	b := s2b("old idiom")
	fmt.Println(string(b), len(b), cap(b))
}
