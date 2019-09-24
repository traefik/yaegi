package main

import "fmt"

type MyT T

type T struct {
	Name string
}

func main() {
	fmt.Println(MyT{})
}

// Output:
// {}
