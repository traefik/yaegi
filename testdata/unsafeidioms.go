// unsafe idioms of real libraries that the VM refused before (docs/UNSAFE.md).
package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

// sliceHeader is the type of cespare/xxhash v2.3.0 (xxhash_unsafe.go): a
// string and a capacity, read as a []byte to hash a string without a copy.
type sliceHeader struct {
	s   string
	cap int
}

func strBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&sliceHeader{s, len(s)}))
}

func main() {
	// xxhash: Sum64String and Digest.WriteString.
	s := "hello, world"
	b := strBytes(s)
	fmt.Println(len(b), cap(b), string(b[7:]), unsafe.SliceData(b) == unsafe.StringData(s))
	heap := string([]byte("on the heap"))
	fmt.Println(string(strBytes(heap)), len(strBytes("")))

	// A slice header whose data moves first, then the length and the
	// capacity (the header is outside the array for a while).
	h1 := make([]byte, 8, 16)
	copy(h1, "abcdefgh")
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&h1))
	sh.Data += 2
	sh.Len -= 2
	sh.Cap -= 2
	fmt.Println(string(h1), len(h1), cap(h1))

	// Words of a []byte seen as a []uint32 or a []float64 (unsafe.Slice):
	// the same memory, both ways.
	buf := make([]byte, 16)
	copy(buf, []byte{1, 0, 0, 0, 2, 0, 0, 0})
	ws := unsafe.Slice((*uint32)(unsafe.Pointer(&buf[0])), len(buf)/4)
	fmt.Println(len(ws), cap(ws), ws[0], ws[1])
	ws[3] = 0x0a0b0c0d
	buf[0] = 7
	fmt.Println(buf[12:], ws[0], ws[2:])
	var sum uint32
	for _, w := range ws {
		sum += w
	}
	fmt.Println(sum, fmt.Sprint(ws[:2]))
	fs := unsafe.Slice((*float64)(unsafe.Pointer(&buf[8])), 1)
	fs[0] = 1.5
	fmt.Println(buf[8:], fs[0])
	lit := []byte{1, 0, 0, 0, 2, 0, 0, 0}
	fmt.Println(unsafe.Slice((*uint32)(unsafe.Pointer(&lit[0])), 2))
	fmt.Println(len(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[2])), 0)))
}
