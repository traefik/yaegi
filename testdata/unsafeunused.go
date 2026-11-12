// unsafe.Slice and unsafe.String whose result is never used run only the
// checks of gc (GOROOT test unsafebuiltins.go): a pointer that the VM does
// not know, or a length past the memory it knows, is not refused, since
// nothing reads the memory. The panics of gc stay.
package main

import (
	"fmt"
	"math"
	"unsafe"
)

func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panics:", r)
			return
		}
		fmt.Println(name, "ok")
	}()
	f()
}

const maxUintptr = 1 << (8 * unsafe.Sizeof(uintptr(0)))

func main() {
	last := (*byte)(unsafe.Pointer(^uintptr(0)))
	far := (*uint32)(unsafe.Pointer(uintptr(0x1000)))
	var word int64 = 7
	wp := (*byte)(unsafe.Pointer(&word))
	n, neg := 3, -1
	var huge uint64 = math.MaxUint64

	try("Slice(last, 1)", func() { _ = unsafe.Slice(last, 1) })
	try("Slice(last, 2)", func() { _ = unsafe.Slice(last, 2) })
	try("String(last, 1)", func() { _ = unsafe.String(last, 1) })
	try("String(last, 2)", func() { _ = unsafe.String(last, 2) })
	try("Slice(far, n)", func() { _ = unsafe.Slice(far, n) })
	try("Slice(far, too big)", func() { _ = unsafe.Slice(far, maxUintptr/4) })
	try("Slice(word, 64)", func() { _ = unsafe.Slice(wp, 64) })
	try("String(word, 64)", func() { _ = unsafe.String(wp, 64) })
	try("Slice(nil, 1)", func() { _ = unsafe.Slice((*int)(nil), 1) })
	try("String(nil, 1)", func() { _ = unsafe.String(nil, 1) })
	try("Slice(word, -1)", func() { _ = unsafe.Slice(wp, neg) })
	try("String(word, huge)", func() { _ = unsafe.String(wp, huge) })

	// a used result still reads the memory
	s := unsafe.Slice(wp, 8)
	fmt.Println(len(s), cap(s), s[0])
	fmt.Println(len(unsafe.String(wp, 1)))
}
