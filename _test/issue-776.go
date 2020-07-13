package main

import "fmt"

// Filter is a filter
type Filter interface {
	Foo()
}

// GIFT is a gift
type GIFT struct {
	Filters []Filter
}

// New is a new filter list
func New(filters ...Filter) *GIFT {
	return &GIFT{
		Filters: filters,
	}
}

// List lists filters
func (g *GIFT) List() {
	fmt.Printf("Hello from List!\n")
}

// MyFilter is one of the filters
type MyFilter struct{}

// Foo is a foo
func (f *MyFilter) Foo() {}

func main() {
	g := New(&MyFilter{})
	g.List()
}

// Output:
// Hello from List!
