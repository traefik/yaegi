package main

import (
	"fmt"
	"strings"
	"unsafe"
)

// A data pointer and a length kept in a struct, and made into a slice or a
// string again much later, as protobuf's protoreflect.Value does
// (valueOfBytes, getBytes). The VM knew the memory behind such a pointer
// only through a small ring of recent regions: after more than 64 other
// unsafe conversions (the init of cel-go), unsafe.Slice was refused with
// "reaches outside the memory known to the VM".

type value struct {
	ptr unsafe.Pointer
	num uint64
}

func ofBytes(b []byte) value {
	return value{ptr: unsafe.Pointer(unsafe.SliceData(b)), num: uint64(len(b))}
}

func ofString(s string) value {
	return value{ptr: unsafe.Pointer(unsafe.StringData(s)), num: uint64(len(s))}
}

func ofInts(s []int) value {
	return value{ptr: unsafe.Pointer(unsafe.SliceData(s)), num: uint64(len(s))}
}

func (v value) bytes() []byte { return unsafe.Slice((*byte)(v.ptr), v.num) }
func (v value) str() string   { return unsafe.String((*byte)(v.ptr), v.num) }
func (v value) ints() []int   { return unsafe.Slice((*int)(v.ptr), v.num) }

// churn makes many other regions, as a big init does.
func churn(n int) int {
	t := 0
	for i := 0; i < n; i++ {
		b := make([]byte, 8+i%5)
		s := strings.Repeat("x", 3+i%7)
		t += len(unsafe.Slice(unsafe.SliceData(b), len(b)))
		t += len(unsafe.String(unsafe.StringData(s), len(s)))
	}
	return t
}

func main() {
	b := []byte("0123456789")
	b = b[:10:10]
	s := strings.Repeat("ab", 6)
	ints := make([]int, 6)
	for i := range ints {
		ints[i] = i * i
	}
	lit := []int{7, 8, 9}
	vb, vs, vi, vl := ofBytes(b), ofString(s), ofInts(ints), ofInts(lit)

	fmt.Println(churn(500))

	got := vb.bytes()
	fmt.Println(string(got), len(got), cap(got))
	got[0] = 'X' // shared with b, as in gc
	fmt.Println(string(b))
	fmt.Println(vs.str())
	fmt.Println(vi.ints())
	fmt.Println(vl.ints())

	// a sub-slice from the data pointer
	sub := ofBytes(b[2:5])
	churn(200)
	fmt.Println(string(sub.bytes()))
	// up to the capacity of the sub-slice (the rest of b), as in gc
	sub.num = 8
	fmt.Println(string(sub.bytes()))
}
