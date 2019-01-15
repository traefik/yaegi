package main

import "fmt"

func main() {
	println("hello")
	defer fmt.Println("bye")
	defer fmt.Println("au revoir")
	println("world")
}

// Output:
// hello
// world
// au revoir
// bye
