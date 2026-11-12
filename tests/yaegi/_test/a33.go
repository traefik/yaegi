package main

import "fmt"

func main() {
	a := [...]int{1, 2, 3}
	b := a
	fmt.Println(b)
}

// Output:
// [1 2 3]
