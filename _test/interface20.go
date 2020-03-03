package main

import "fmt"

func main() {
	var a interface{}
	a = string("A")
	fmt.Println(a)
}

// Output:
// A
