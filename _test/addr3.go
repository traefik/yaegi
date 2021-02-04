package main

import (
	"fmt"
)

func main() {
	var a interface{}
	a = 2
	fmt.Println(a)

	var b *interface{}
	b = &a
	fmt.Println(*b)

	var c **interface{}
	c = &b
	fmt.Println(**c)
}

// Output:
// 2
// 2
// 2
