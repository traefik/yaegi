// The capacity of []byte("constant"): gc makes a [len]byte array for a
// constant string, so cap == len, and a reflect.SliceHeader view of the
// slice reports the same Cap. The capacity of []byte(s) for a string that
// is not constant depends on escape analysis in gc (a 32-byte stack buffer
// or a size class), so it is not printed here.
package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

const long = "a constant string of more than thirty-two bytes, to skip the stack buffer"

func main() {
	b := []byte("hello world")
	h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	fmt.Println(len(b), cap(b), h.Len, h.Cap)

	// The idiom of the header view: shorten the slice, then build a
	// string on the same bytes.
	h.Len = 5
	var s string
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	sh.Data = h.Data
	sh.Len = h.Len
	fmt.Println(string(b), s, h.Cap, cap(b))

	// The capacity after the length: what append sees.
	c := []byte("abc")
	c2 := append(c, 'd')
	c[0] = 'X'
	fmt.Println(cap(c), string(c2)) // a new array: c2 does not see 'X'

	l := []byte(long)
	fmt.Println(len(l) == cap(l))

	var e []byte = []byte("")
	fmt.Println(len(e), cap(e), e != nil)

	type myBytes []byte
	m := myBytes("named")
	fmt.Println(len(m), cap(m))
}
