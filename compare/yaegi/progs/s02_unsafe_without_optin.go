package main

import (
	"fmt"
	"unsafe"
)

func main() {
	x := int64(1)
	fmt.Println(*(*int64)(unsafe.Pointer(&x)), unsafe.Sizeof(x))
}
