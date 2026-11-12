package main

// Functions without a body linked to the runtime with //go:linkname, as
// wazero (memmove, nanotime) and golang.org/x/sys/cpu (getAuxv) declare
// them. The VM gives them with the meaning they have in Go: they are not
// refused at load time any more.

import (
	"fmt"
	"reflect"
	"unsafe"
)

//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)

//go:linkname nanotime runtime.nanotime
func nanotime() int64

//go:linkname getAuxv runtime.getAuxv
func getAuxv() []uintptr

// as wazero's internal/engine/wazevo/memmove.go
var memmovePtr = reflect.ValueOf(memmove).Pointer()

func main() {
	src := []byte("hello, world")
	dst := make([]byte, 5)
	memmove(unsafe.Pointer(&dst[0]), unsafe.Pointer(&src[7]), 5)
	fmt.Println(string(dst))
	// overlapping, forward
	memmove(unsafe.Pointer(&src[1]), unsafe.Pointer(&src[0]), 4)
	fmt.Println(string(src))
	// from the bytes of a string
	s := "abcdef"
	memmove(unsafe.Pointer(&dst[0]), unsafe.Pointer(unsafe.StringData(s)), 3)
	fmt.Println(string(dst))
	memmove(unsafe.Pointer(&dst[0]), unsafe.Pointer(&src[0]), 0)

	t0 := nanotime()
	t1 := nanotime()
	fmt.Println(t0 > 0, t1 >= t0)
	aux := getAuxv()
	fmt.Println(len(aux)%2 == 0)
	fmt.Println(memmovePtr != 0)
}
