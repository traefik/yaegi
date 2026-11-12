package main

import "fmt"

func main() {
	var a, b, c uint16
	a = 64
	b = 64
	c = a * b
	fmt.Printf("c: %v %T", c, c)
}

// Output:
// c: 4096 uint16
