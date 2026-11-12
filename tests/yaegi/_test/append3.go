package main

import "fmt"

func main() {
	a := []int{1, 2}
	b := [2]int{3, 4}
	fmt.Println(append(a, b[:]...))
	fmt.Println(append(a, []int{5, 6}...))
}

// Output:
// [1 2 3 4]
// [1 2 5 6]
