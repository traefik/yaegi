package main

import (
	"fmt"
	"unsafe"
)

func b2s(b []byte) string { return unsafe.String(unsafe.SliceData(b), len(b)) }
func s2b(s string) []byte { return unsafe.Slice(unsafe.StringData(s), len(s)) }

func main() {
	b := []byte("hello world")
	s := b2s(b)
	fmt.Println(s, len(s))
	bb := s2b("abc")
	fmt.Println(bb, string(bb))
	fmt.Println(b2s(nil) == "", len(s2b("")))
}
