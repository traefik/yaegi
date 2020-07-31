package main

import "fmt"

type I interface {
	Foo() string
}

type Printer struct {
	i I
}

func New(i I) *Printer {
	return &Printer{
		i: i,
	}
}

func (p *Printer) Print() {
	fmt.Println(p.i.Foo())
}

type T struct{}

func (t *T) Foo() string {
	return "test"
}

func main() {
	g := New(&T{})
	g.Print()
}

// Output:
// test
