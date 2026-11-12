package main

func NewT(name string) *T { return &T{name} }

var C = NewT("test")

func (t *T) f() { println(t == C) }

type T struct {
	Name string
}

func main() {
	C.f()
}

// Output:
// true
