package main

var Foo int

func (f Foo) Bar() int {
	return 1
}

func main() {}

// Error:
// 5:1: cannot define new methods on non-local type int
