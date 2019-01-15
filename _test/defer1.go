package main

import "fmt"

func main() {
	println("hello")
	defer func() {
		fmt.Println("bye")
	}()
	println("world")
}

// Output:
// hello
// world
// bye
