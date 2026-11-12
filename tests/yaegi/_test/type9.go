package main

import "fmt"

type Hello struct {
	Goodbye GoodbyeProvider
}

func main() {
	a := &Hello{}

	fmt.Println(a)
}

type GoodbyeProvider func(message string) string

// Output:
// &{<nil>}
