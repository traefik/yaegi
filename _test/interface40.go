package main

import "fmt"

type foo struct {
	bar string
}

func (f foo) String() string {
	return "Hello from " + f.bar
}

func NewFoo(s string) fmt.Stringer {
	return foo{s}
}

func main() {
	f := NewFoo("bar")
	fmt.Println(f)
}

// Output:
// Hello from bar
