package main

import "fmt"

func main() {
	var c complex128
	c = 1
	fmt.Printf("%T %v\n", c, c)
}

// Output:
// complex128 (1+0i)
