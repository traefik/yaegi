package main

type I interface {
	F()
}

type T struct {
	Name string
}

func (t *T) F() { println("in F", t.Name) }

func NewI(s string) I { return newT(s) }

func newT(s string) *T { return &T{s} }

func main() {
	i := NewI("test")
	i.F()
}

// Output:
// in F test
