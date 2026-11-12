package main

import "fmt"

type foo struct {
	bar string
}

func (f *foo) String() string {
	return "Hello from " + f.bar
}

func main() {
	var f fmt.Stringer = &foo{bar: "bar"}
	fmt.Println(f)
}

// Output:
// Hello from bar
