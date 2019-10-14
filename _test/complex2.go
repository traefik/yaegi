package main

import "fmt"

func main() {
	c := complex(1, 0)
	c += 1
	fmt.Printf("%T %v\n", c, c)
}

// Output:
// complex128 (2+0i)
