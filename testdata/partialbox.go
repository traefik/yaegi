package main

// A pointer to an interpreted struct that cannot be copied to native code
// (it holds an interpreted channel), passed twice to the same native call:
// the failed copy of the first argument must not leave a half-built native
// copy behind for the second one.

import (
	"fmt"
	"strings"
)

type T struct {
	N  int
	Ch chan int
}

func main() {
	s := &T{N: 42, Ch: make(chan int)}
	a, b, _ := strings.Cut(fmt.Sprintf("%v|%v", s, s), "|")
	fmt.Println(s.N, a == b)
}
