package main

import (
	"fmt"

	"guthib.com/generic"
)

func wrap[K comparable, V any](a any) *generic.Cache[K, V] {
	var c *generic.Cache[K, V]
	if p, ok := a.(*generic.Cache[K, V]); ok {
		c = p
	}
	return c
}

func main() {
	c := &generic.Cache[string, int]{}
	c.Set("a", 1, 2)
	fmt.Println(wrap[string, int](c).Get())
}

// GOPATH:testdata/generic-cross-pkg
// Output:
// a 1 2ns
