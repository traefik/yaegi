package main

import "fmt"

func main() {
	println("hello")
	i := 12
	defer func() {
		fmt.Println("i:", i)
	}()
	i = 20
	println("world")
}

// Output:
// hello
// world
// i: 20
