package main

import (
	"fmt"
	"unsafe"
)

// Idiom found in fasthttp/gjson-era code: build a slice header by hand.
func s2b(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&struct {
		string
		Cap int
	}{s, len(s)}))
}

func main() {
	b := s2b("header idiom")
	fmt.Println(string(b), len(b), cap(b))
}
