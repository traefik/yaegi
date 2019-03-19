package main

import "fmt"

func main() {
	a := []int{10, 20, 30}
	b := [4]int{}
	c := b[:]
	copy(c, a)
	fmt.Println(c)
}

// Output:
// [10 20 30 0]
