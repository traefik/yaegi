package main

func Bar() {
	s := Obj.Foo()
	println(s)
}

var Obj = NewT()

func NewT() *T { return &T{} }

type T struct{}

func (t *T) Foo() bool { return t != nil }

func main() {
	Bar()
}

// Output:
// true
