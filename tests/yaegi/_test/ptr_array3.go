package main

import "fmt"

func main() {
	a := &[...]int{1, 2, 3}
	fmt.Println(a[:])
}

// Output:
// [1 2 3]
