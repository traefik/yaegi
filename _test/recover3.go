package main

import "fmt"

func main() {
	println("hello")

	var r any = 1
	r = recover()
	fmt.Printf("%v\n", r)
	if r == nil {
		println("world")
	}
	if r != nil {
		println("exception")
	}
}

// Output:
// hello
// <nil>
// world
