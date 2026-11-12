package main

import "fmt"

func main() {
	a := [...]byte{}
	fmt.Printf("%T\n", a)
}

// Output:
// [0]uint8
