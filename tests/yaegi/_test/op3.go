package main

import "fmt"

func main() {
	a := -1.2
	fmt.Printf("a: %v %T\n", a, a)
	b := -(2 + 1i)
	fmt.Printf("b: %v %T\n", b, b)
}

// Output:
// a: -1.2 float64
// b: (-2-1i) complex128
