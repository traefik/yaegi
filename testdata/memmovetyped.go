// runtime.memmove (linked with go:linkname) of a whole value, when the SSA
// shows the same type behind both pointers, is the assignment *dst = *src,
// as in gc, also for values that the VM holds in cells (a struct with a
// string, an array of structs, a native time.Time). Before, the VM refused
// the copy: it could only copy bytes it knows (a []byte, a string, plain
// data in real bytes).
package main

import (
	"fmt"
	"time"
	"unsafe"
)

//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)

type T struct {
	A int
	B string
	C []int
	D *int
}

func main() {
	x := 7
	a := T{1, "x", []int{1, 2}, &x}
	var b T
	memmove(unsafe.Pointer(&b), unsafe.Pointer(&a), unsafe.Sizeof(a))
	fmt.Println(b.A, b.B, b.C, *b.D, b.D == a.D)
	a.A, a.B = 2, "y"
	a.C[0] = 9
	fmt.Println(b.A, b.B, b.C) // b is a copy; the slice shares its array

	arr := [3]T{{A: 1}, {A: 2, B: "two"}, {A: 3}}
	var arr2 [3]T
	memmove(unsafe.Pointer(&arr2), unsafe.Pointer(&arr), unsafe.Sizeof(arr))
	arr[1].B = "changed"
	fmt.Println(arr2[0].A, arr2[1].A, arr2[1].B, arr2[2].A)

	t1 := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	var t2 time.Time
	memmove(unsafe.Pointer(&t2), unsafe.Pointer(&t1), unsafe.Sizeof(t1))
	fmt.Println(t2.Equal(t1), t2.Year())

	n1, n2 := 1.5, 0.0
	memmove(unsafe.Pointer(&n2), unsafe.Pointer(&n1), 8)
	fmt.Println(n2)

	// n = 0 copies nothing
	var c T
	memmove(unsafe.Pointer(&c), unsafe.Pointer(&a), 0)
	fmt.Println(c.A, c.B == "")
}
