// reflect.Value.Pointer of a pointer to a byte of a []byte gives the address
// of that byte, as uintptr(unsafe.Pointer(p)) does. The purego version of
// golang.org/x/crypto/internal/alias compares these addresses to find
// buffers that overlap. Before, the VM gave 0 for every such pointer, so two
// separate buffers overlapped ("salsa20: invalid buffer overlap").
package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

type rec struct {
	A, B uint32
}

func anyOverlap(x, y []byte) bool {
	return len(x) > 0 && len(y) > 0 &&
		reflect.ValueOf(&x[0]).Pointer() <= reflect.ValueOf(&y[len(y)-1]).Pointer() &&
		reflect.ValueOf(&y[0]).Pointer() <= reflect.ValueOf(&x[len(x)-1]).Pointer()
}

func main() {
	a := make([]byte, 16)
	b := []byte("sixteen byte msg")
	fmt.Println(anyOverlap(a, b), anyOverlap(a, a[4:]), anyOverlap(a[:4], a[4:]), anyOverlap(a[:5], a[4:]))

	p0, p3 := reflect.ValueOf(&a[0]).Pointer(), reflect.ValueOf(&a[3]).Pointer()
	fmt.Println(p3-p0, p0 == uintptr(unsafe.Pointer(&a[0])), p0 != 0)
	fmt.Println(reflect.ValueOf(&a[2]).UnsafePointer() == unsafe.Pointer(&a[2]))

	// plain data over the bytes of a []byte
	buf := make([]byte, 16)
	r := (*rec)(unsafe.Pointer(&buf[8]))
	fmt.Println(reflect.ValueOf(r).Pointer()-reflect.ValueOf(&buf[0]).Pointer(), reflect.ValueOf(&r.B).Pointer()-reflect.ValueOf(r).Pointer())
	var nb *byte
	fmt.Println(reflect.ValueOf(nb).Pointer())

	// a byte of a string (read-only bytes) and the data pointer of a slice
	s := "hello, world"
	sp := unsafe.StringData(s)
	fmt.Println(reflect.ValueOf(sp).Pointer() == uintptr(unsafe.Pointer(sp)), reflect.ValueOf(unsafe.StringData(s[7:])).Pointer()-reflect.ValueOf(sp).Pointer())
	fmt.Println(reflect.ValueOf(unsafe.SliceData(a)).Pointer() == p0)

	// back to the byte through unsafe.Pointer(v.Pointer())
	a[1] = 'b'
	fmt.Println(*(*byte)(unsafe.Pointer(reflect.ValueOf(&a[1]).Pointer())))
}
