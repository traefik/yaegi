package main

import "fmt"

func f(a []int, b int) interface{} { return append(a, b) }

func main() {
	a := []int{1, 2}
	r := f(a, 3)
	fmt.Println(r.([]int))
}

// Output:
// [1 2 3]
