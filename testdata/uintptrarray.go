// uintptr(unsafe.Pointer(&a)) of an array is the address of its first
// element, as in gc. The data word of a slice of capacity 0 cut from the
// array (a[0:0:0], a[2:2:2]) is then the address of the array, as the
// GOROOT test slice3.go checks.
package main

import (
	"fmt"
	"unsafe"
)

var array = new([10]byte)

type pair struct {
	a int32
	b int64
}

func header(x []byte) [3]uintptr { return *(*[3]uintptr)(unsafe.Pointer(&x)) }

func main() {
	base := uintptr(unsafe.Pointer(array))
	fmt.Println(uintptr(unsafe.Pointer(&array[0]))-base, uintptr(unsafe.Pointer(&array[3]))-base)
	for _, x := range [][]byte{array[0:0:0], array[2:2:2], array[0:0:1], array[3:5:7], array[10:10:10], array[:], array[4:4]} {
		h := header(x)
		fmt.Println(h[0]-base, h[1], h[2])
	}
	s := array[:]
	for _, x := range [][]byte{s[0:0:0], s[5:5:5], s[1:2:3]} {
		h := header(x)
		fmt.Println(h[0]-base, h[1], h[2])
	}

	var ps [3]pair
	pbase := uintptr(unsafe.Pointer(&ps))
	fmt.Println(uintptr(unsafe.Pointer(&ps[0]))-pbase, uintptr(unsafe.Pointer(&ps[2]))-pbase)

	// back to a pointer to the array: the same cells
	q := (*[10]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(array))))
	q[4] = 7
	fmt.Println(array[4], q == array)

	// an array of arrays: the inner arrays are elements of the outer one,
	// with its gc offsets (the elements of an inner array are not compared
	// with the outer array: the VM gives them addresses in the inner array)
	var g [2][3]int32
	gbase := uintptr(unsafe.Pointer(&g))
	fmt.Println(uintptr(unsafe.Pointer(&g[0]))-gbase, uintptr(unsafe.Pointer(&g[1]))-gbase, uintptr(unsafe.Pointer(&g[1]))-uintptr(unsafe.Pointer(&g[0])))
	fmt.Println(inner(&g))
	r := (*[3]int32)(unsafe.Pointer(uintptr(unsafe.Pointer(&g[1]))))
	r[2] = 5
	fmt.Println(g[1][2], r == &g[1])
}

// inner is the offset of the second inner array of a pointer parameter (the
// SSA shows the array, not where the pointer comes from).
func inner(p *[2][3]int32) uintptr {
	return uintptr(unsafe.Pointer(&p[1])) - uintptr(unsafe.Pointer(p))
}
