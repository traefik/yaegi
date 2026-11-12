package main

import "fmt"

type Foo struct{}

func foo() *Foo { return nil }

func main() {
	f := foo()
	fmt.Println(f)
}

// Output:
// <nil>
