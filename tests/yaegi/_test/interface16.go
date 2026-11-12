package main

import "fmt"

type Barer interface {
	fmt.Stringer
	Bar()
}

type T struct{}

func (*T) String() string { return "T: nothing" }
func (*T) Bar()           { println("in bar") }

var t = &T{}

func main() {
	var f Barer
	if f != t {
		println("ok")
	}
}

// Output:
// ok
