package main

import "fmt"

func main() {
	i := 100
	j := i % 1e2
	fmt.Printf("%T %v\n", j, j)
}

// Output:
// int 0
