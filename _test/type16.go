package main

import "fmt"

type Barer interface {
	fmt.Stringer
	Bar()
}

type T struct{}

func (t *T) Foo() string { return "T: foo" }
func (*T) Bar()          { println("in bar") }

var t = &T{}

func main() {
	var f Barer
	if f != t {
		println("ok")
	}
}
