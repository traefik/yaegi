package main

import "fmt"

type IntArray []int

func (h *IntArray) Add(x int) {
	*h = append(*h, x)
}

func main() {
	a := IntArray{}
	a.Add(4)

	fmt.Println(a)
}

// Output:
// [4]
