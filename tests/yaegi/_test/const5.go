package main

import (
	"fmt"
)

const (
	a uint8 = 2 * iota
	b
	c
)

func main() {
	fmt.Printf("%T\n", c)
	fmt.Println(a, b, c)
}

// Output:
// uint8
// 0 2 4
