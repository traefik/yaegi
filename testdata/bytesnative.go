package main

import (
	"bytes"
	"fmt"
	"strconv"
	"unsafe"
)

// Byte slices made by make or by []byte(s) are Go []byte in the VM: the
// memory is shared with unsafe views and with native code, as in gc.

func main() {
	n := 4
	dyn := make([]byte, n)
	fixed := make([]byte, 4)
	conv := []byte("abcd")
	for _, b := range [][]byte{dyn, fixed, conv} {
		s := unsafe.String(&b[0], len(b))
		b[0] = 'Z'
		fmt.Println(s[:1], len(s))
	}

	// A pointer to a byte goes to native code as the real pointer.
	p := &conv[1]
	fmt.Printf("%T %v\n", p, uintptr(unsafe.Pointer(p))-uintptr(unsafe.Pointer(&conv[0])))
	buf := strconv.AppendInt(fixed[:0], 42, 10)
	fmt.Println(string(buf), string(fixed[:2]))

	// The usual operations on both kinds of byte slices.
	lit := []byte{'x', 'y'}
	all := append(append(dyn[:1:1], lit...), conv...)
	fmt.Println(string(all), bytes.Equal(all[1:3], lit), cap(make([]byte, 0, n)))
	copy(dyn, "hello")
	fmt.Println(string(dyn), string(bytes.ToUpper(dyn)))
	var arr [3]byte
	copy(arr[:], conv)
	fmt.Println(arr, string(conv[1:3]))
	for i, c := range []byte("hi") {
		fmt.Print(i, c, " ")
	}
	fmt.Println()
}
