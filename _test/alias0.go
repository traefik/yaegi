package main

import "fmt"

type Root struct {
	Name string
}

func (r *Root) Hello() {
	fmt.Println("Hello", r.Name)
}

type One = Root

type Hi interface {
	Hello()
}

func main() {
	one := &One{Name: "test"}

	fmt.Println(one)
}

// Output:
// &{test}
