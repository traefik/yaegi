package main

import "fmt"

const (
	a = iota
	b
	c
	d
)

type T [c]int

func main() {
	fmt.Println(T{})
}

// Output:
// [0 0]
