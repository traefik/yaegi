package main

import "fmt"

func (f *Foo) Bar() int {
	return *f * *f
}

type Foo = int

func main() {
	x := Foo(1)
	fmt.Println(x.Bar())
}

// Error:
// 9:6: cannot define new methods on non-local type int
