// unsafe.Pointer(v.Pointer()) of a reflect.Value gives back the object, as
// in gc, whatever way Pointer is called (a static call, a method
// expression, an interface, a method value), also when a garbage
// collection runs before the conversion (GOROOT test
// fixedbugs/issue15329.go).
package main

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

type point struct{ x, y int }

type pointerer interface {
	Pointer() uintptr
}

func newInt(n int) reflect.Value {
	runtime.GC()
	return reflect.ValueOf(&n)
}

func viaInterface(n int) pointerer { return newInt(n) }

func viaMethodValue(n int) func() uintptr { return newInt(n).Pointer }

func main() {
	a, b := unsafe.Pointer(newInt(1).Pointer()), unsafe.Pointer(newInt(2).Pointer())
	fmt.Println(*(*int)(a), *(*int)(b))

	a, b = unsafe.Pointer(reflect.Value.Pointer(newInt(3))), unsafe.Pointer(reflect.Value.Pointer(newInt(4)))
	fmt.Println(*(*int)(a), *(*int)(b))

	a, b = unsafe.Pointer(viaInterface(5).Pointer()), unsafe.Pointer(viaInterface(6).Pointer())
	fmt.Println(*(*int)(a), *(*int)(b))

	a, b = unsafe.Pointer(viaMethodValue(7)()), unsafe.Pointer(viaMethodValue(8)())
	fmt.Println(*(*int)(a), *(*int)(b))

	// the pointer is the object: a write through it is seen by the program
	p := &point{1, 2}
	var i pointerer = reflect.ValueOf(p)
	q := (*point)(unsafe.Pointer(i.Pointer()))
	q.y = 20
	fmt.Println(*p, q == p)

	// UnsafePointer, and a slice: the pointer to its first element
	s := []int{10, 20, 30}
	e := (*int)(reflect.ValueOf(s).UnsafePointer())
	*e = 11
	fmt.Println(s, *(*int)(unsafe.Pointer(reflect.ValueOf(s).Pointer())))
}
