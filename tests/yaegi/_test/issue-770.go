package main

import "reflect"

type I interface {
	Foo() int
}

type T struct {
	Name string
}

func (t T) Foo() int { return 0 }

func f(v reflect.Value) int {
	i := v.Interface().(I)
	return i.Foo()
}

func main() {
	println("hello")
}

// Output:
// hello
