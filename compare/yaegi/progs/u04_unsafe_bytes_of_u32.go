package main

import (
	"fmt"
	"unsafe"
)

func main() {
	u := uint32(0x01020304)
	p := (*[4]byte)(unsafe.Pointer(&u))
	fmt.Println(p[0], p[3]) // little endian on amd64/arm64
	p[0] = 0xff
	fmt.Printf("%#x\n", u)
	var x uint16 = 1
	littleEndian := *(*byte)(unsafe.Pointer(&x)) == 1
	fmt.Println("little endian:", littleEndian)
	f := float32(1.5)
	fmt.Printf("%#x\n", *(*uint32)(unsafe.Pointer(&f)))
}
