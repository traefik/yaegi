package main

import "fmt"

var I any

func main() {
	fmt.Printf("%T %v\n", I, I)
}

// Output:
// <nil> <nil>
