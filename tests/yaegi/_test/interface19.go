package main

import "fmt"

var I interface{}

func main() {
	fmt.Printf("%T %v\n", I, I)
}

// Output:
// <nil> <nil>
