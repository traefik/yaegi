package main

import (
	"fmt"
	"math"
	"unsafe"
)

// Uses of package unsafe the VM emulates (docs/UNSAFE.md).

type Point struct {
	X, Y int64
	Name string
}

type Pair struct {
	A, B int64
	S    string
}

func b2s(b []byte) string { return *(*string)(unsafe.Pointer(&b)) }

func s2b(s string) []byte { return *(*[]byte)(unsafe.Pointer(&s)) }

func main() {
	// Zero-copy conversion idioms.
	b := []byte("hello")
	fmt.Println(b2s(b), string(s2b("world")))
	s := unsafe.String(unsafe.SliceData(b), len(b))
	fmt.Println(s, string(unsafe.Slice(unsafe.StringData("abc"), 3)))

	// Sizes are gc's.
	var p Point
	fmt.Println(unsafe.Sizeof(p), unsafe.Alignof(p.Y), unsafe.Offsetof(p.Name))

	// Field access through offsets.
	p = Point{X: 1, Y: 2, Name: "p"}
	py := (*int64)(unsafe.Add(unsafe.Pointer(&p), unsafe.Offsetof(p.Y)))
	*py = 20
	pn := (*string)(unsafe.Pointer(uintptr(unsafe.Pointer(&p)) + unsafe.Offsetof(p.Name)))
	fmt.Println(p.Y, *pn)

	// Element access through offsets.
	pts := []Point{{X: 1}, {X: 2}, {X: 3}}
	third := (*Point)(unsafe.Add(unsafe.Pointer(&pts[0]), 2*unsafe.Sizeof(pts[0])))
	third.Name = "third"
	arr := [4]int32{10, 20, 30, 40}
	last := (*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(&arr)) + 3*unsafe.Sizeof(arr[0])))
	fmt.Println(pts[2].Name, *last)

	// Pointers between types of the same layout share memory.
	q := (*Pair)(unsafe.Pointer(&p))
	q.B = 200
	f := 1.5
	bits := *(*uint64)(unsafe.Pointer(&f))
	fmt.Println(p.Y, q.S, bits == math.Float64bits(f))

	// Addresses: printing, comparing, round trips.
	up := uintptr(unsafe.Pointer(&p))
	fmt.Println(fmt.Sprintf("%x", up) == fmt.Sprintf("%x", unsafe.Pointer(&p)), (*Point)(unsafe.Pointer(up)) == &p)
	px, py2 := unsafe.Pointer(&p.X), unsafe.Pointer(&p.Y)
	seen := map[unsafe.Pointer]bool{px: true}
	fmt.Println(px == py2, px == unsafe.Pointer(&p.X), seen[px], seen[py2], unsafe.Pointer(nil) == nil)
}
