package main

import "fmt"

func main() {
	a := [...]byte{}
	b := a
	fmt.Printf("%T %T\n", a, b)
}

// Output:
// [0]uint8 [0]uint8
