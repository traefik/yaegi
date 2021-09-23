package main

import "fmt"

type Foo = int

func (f Foo) Bar() int {
	return f * f
}

func main() {
	x := Foo(1)
	fmt.Println(x.Bar())
}

// Error:
// 7:1: cannot define new methods on non-local type int
