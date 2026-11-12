package main

import "fmt"

func main() {
	a := [...]int{1, 2, 3}
	b := a
	b[0] = -1
	fmt.Println(a)
}

// Output:
// [1 2 3]
