package main

import "fmt"

type T struct {
	A int
	B int
}

func main() {
	a := &[]T{
		{1, 2},
		{3, 4},
	}
	fmt.Println("a:", a)
	x := &(*a)[1:][0]
	fmt.Println("x:", x)
}

// Output:
// a: &[{1 2} {3 4}]
// x: &{3 4}
