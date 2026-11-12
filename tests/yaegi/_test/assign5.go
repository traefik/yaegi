package main

import "fmt"

func main() {
	t := []int{1, 2}
	fmt.Println(t)
	t[0], t[1] = t[1], t[0]
	fmt.Println(t)
}

// Output:
// [1 2]
// [2 1]
