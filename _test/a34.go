package main

import "fmt"

func main() {
	a := [...]int{1, 2, 3}
	var b [3]int = a
	fmt.Println(b)
}

// Output:
// [1 2 3]
