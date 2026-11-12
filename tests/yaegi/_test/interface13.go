package main

import (
	"fmt"
)

type X struct{}

func (X) Foo() int {
	return 1
}
func (X) Bar() int {
	return 2
}

type Foo interface {
	Foo() int
}
type Bar interface {
	Bar() int
}

func main() {
	var x X
	var i Foo = x
	j := i.(Bar)

	fmt.Println(j.Bar())
}

// Output:
// 2
