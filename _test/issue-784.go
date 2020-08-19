package main

import "fmt"

// Filter is a filter interface
type Filter interface {
	Bounds(srcBounds string) (dstBounds string)
}

// GIFT type
type GIFT struct {
	Filters []Filter
}

// New creates a new filter list and initializes it with the given slice of filters.
func New(filters ...Filter) *GIFT {
	return &GIFT{
		Filters: filters,
	}
}

// Bounds calculates the appropriate bounds for the result image after applying all the added filters.
func (g *GIFT) Bounds(srcBounds string) (dstBounds string) {
	dstBounds = srcBounds
	for _, f := range g.Filters {
		dstBounds = f.Bounds(dstBounds)
	}
	return dstBounds
}

func main() {
	var filters []Filter
	bounds := "foo"
	g := New(filters...)
	fmt.Println(g.Bounds(bounds))
}

// Output:
// foo
