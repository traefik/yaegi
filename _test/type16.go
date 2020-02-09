package main

import "fmt"

func main() {
	a := uint8(15) ^ byte(0)
	fmt.Printf("%T %v\n", a, a)
}

// Output:
// uint8 15
