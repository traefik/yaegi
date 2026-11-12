package main

import (
	"fmt"
	"hash/maphash"
	"unsafe"
)

type T struct{ A, B int64 }

func main() {
	t := &T{1, 2}
	p := unsafe.Pointer(t)
	// unsafe.Pointer crossing to native code that only stores/compares it.
	m := map[unsafe.Pointer]string{p: "t"}
	fmt.Println(m[unsafe.Pointer(t)])
	fmt.Println(fmt.Sprintf("%p", p) == fmt.Sprintf("%p", t))
	// Native code reading the pointed bytes.
	b := unsafe.Slice((*byte)(p), unsafe.Sizeof(*t))
	var h maphash.Hash
	h.SetSeed(maphash.MakeSeed())
	h.Write(b)
	fmt.Println(len(b), b[0], b[8])
}
