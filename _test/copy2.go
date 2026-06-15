package main

import "fmt"

func f(a, b []int) any { return copy(a, b) }

func main() {
	a := []int{10, 20, 30}
	b := [4]int{}
	c := b[:]
	r := f(c, a)
	fmt.Println(r.(int))
}

// Output:
// 3
