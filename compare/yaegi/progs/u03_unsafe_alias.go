package main

import (
	"fmt"
	"unsafe"
)

// Zero-copy conversions share memory: libraries that reuse a buffer after
// converting it rely on this (and on the absence of copy for performance).
func main() {
	b := []byte("aaaa")
	s := unsafe.String(unsafe.SliceData(b), len(b))
	b[0] = 'X'
	fmt.Println(s)
	s2 := *(*string)(unsafe.Pointer(&b))
	b[1] = 'Y'
	fmt.Println(s2)
	fmt.Println(unsafe.StringData(s) == &b[0])
}
