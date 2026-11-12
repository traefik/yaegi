package main

import "fmt"

var a = T{}

type T struct{}

func main() {
	fmt.Println(a)
}

// Output:
// {}
