package main

import "fmt"

type T [l1 + l2]int

const (
	l1 = 2
	l2 = 3
)

func main() {
	fmt.Println(T{})
}

// Output:
// [0 0 0 0 0]
