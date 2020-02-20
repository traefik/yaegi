package main

import "fmt"

func main() {
	var s int = 1 + complex(1, 0)
	fmt.Printf("%T %v\n", s, s)
}

// Output:
// int 2
