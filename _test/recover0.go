package main

import "fmt"

func main() {
	println("hello")
	defer func() {
		r := recover()
		fmt.Println("recover:", r)
	}()
	println("world")
}

// Output:
// hello
// world
// recover: <nil>
