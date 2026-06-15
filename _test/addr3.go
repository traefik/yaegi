package main

import (
	"fmt"
)

func main() {
	var a any
	a = 2
	fmt.Println(a)

	var b *any
	b = &a
	fmt.Println(*b)

	var c **any
	c = &b
	fmt.Println(**c)
}

// Output:
// 2
// 2
// 2
