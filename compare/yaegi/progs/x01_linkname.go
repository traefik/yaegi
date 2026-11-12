package main

import (
	"fmt"
	_ "unsafe"
)

//go:linkname nanotime runtime.nanotime
func nanotime() int64

func main() {
	a := nanotime()
	b := nanotime()
	fmt.Println(a > 0, b >= a)
}
