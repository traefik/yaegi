package main

// unsafe.Sizeof, Alignof and Offsetof on operands that depend on a type
// parameter: ssa keeps them as builtin calls in each instance.

import (
	"fmt"
	"unsafe"
)

type Inner struct {
	X int8
	Y int64
}

type Outer[T any] struct {
	A T
	Inner
	B T
	C Inner
}

func layout[T any](p *Outer[T]) {
	var t T
	fmt.Println("size", unsafe.Sizeof(t), unsafe.Sizeof(*p), unsafe.Sizeof(p.C))
	fmt.Println("align", unsafe.Alignof(t), unsafe.Alignof(*p))
	fmt.Println("offset", unsafe.Offsetof(p.A), unsafe.Offsetof(p.Inner), unsafe.Offsetof(p.Y),
		unsafe.Offsetof(p.Inner.Y), unsafe.Offsetof(p.B), unsafe.Offsetof(p.C.Y))
	v := *p
	fmt.Println("value", unsafe.Offsetof(v.X), unsafe.Offsetof(v.C))
	f := func() uintptr { return unsafe.Offsetof(p.B) + unsafe.Sizeof(t) }
	fmt.Println("closure", f())
}

func main() {
	layout(new(Outer[int8]))
	layout(new(Outer[string]))
	layout(new(Outer[[3]int16]))
	layout(new(Outer[struct{}]))
}
