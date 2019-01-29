package main

import (
	"fmt"
)

type Foo struct {
	A string
}

var f = Foo{"world"} // <-- the root cause

func Hello() {
	fmt.Println("in")
}

var name = "v1" // <-- the root cause

func main() {
	Hello()
	fmt.Println("Hello", f.A, name)
}

// Output:
// in
// Hello world v1
