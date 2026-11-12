package main

import "fmt"

type T struct {
	Name string
}

func (T) create() *T {
	return &T{"Hello"}
}

func main() {
	fmt.Println(T{}.create())
}

// Output:
// &{Hello}
