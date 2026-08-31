package main

import (
	"fmt"
	"unsafe"
)

func main() {
	arr := []int64{10, 20, 30, 40, 50}
	sl := unsafe.Slice(&arr[0], 3)
	sl[0] = 99
	var np *int64
	ns := unsafe.Slice(np, 0)
	bs := []byte("hello")
	s := unsafe.String(&bs[0], len(bs))
	fmt.Println(sl, len(sl), arr[0])
	fmt.Println(ns == nil, len(ns))
	fmt.Println(s)
}

// Output:
// [99 20 30] 3 99
// true 0
// hello
