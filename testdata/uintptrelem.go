// uintptr(p) of an element of an array is the address of the array plus
// the gc offset of the element, as in gc (GOROOT test
// fixedbugs/issue36516.go): a loop of uintptr(x) + 1 and - 1 goes from one
// element to the next and back.
package main

import (
	"fmt"
	"unsafe"
)

var buf [2]byte
var x unsafe.Pointer = unsafe.Pointer(&buf[0])

type pair struct {
	a int32
	b int64
}

func main() {
	fmt.Println(uintptr(unsafe.Pointer(&buf[1])) - uintptr(unsafe.Pointer(&buf[0])))
	for i := 0; i < 200; i++ {
		x = unsafe.Pointer(uintptr(x) + 1)
		if x != unsafe.Pointer(&buf[1]) {
			fmt.Println("not buf[1] at", i)
			return
		}
		x = unsafe.Pointer(uintptr(x) - 1)
		if x != unsafe.Pointer(&buf[0]) {
			fmt.Println("not buf[0] at", i)
			return
		}
	}
	fmt.Println("loop ok")

	var arr [4]int64
	fmt.Println(uintptr(unsafe.Pointer(&arr[3])) - uintptr(unsafe.Pointer(&arr[1])))
	p := unsafe.Pointer(&arr[2])
	*(*int64)(unsafe.Pointer(uintptr(p) + 8)) = 7
	*(*int64)(unsafe.Pointer(uintptr(p) - 16)) = 5
	fmt.Println(arr)

	ps := make([]pair, 3)
	d := uintptr(unsafe.Pointer(&ps[2])) - uintptr(unsafe.Pointer(&ps[0]))
	fmt.Println(d, d == 2*unsafe.Sizeof(pair{}))
	q := (*pair)(unsafe.Add(unsafe.Pointer(&ps[1]), unsafe.Sizeof(pair{})))
	q.b = 9
	fmt.Println(ps)
}
