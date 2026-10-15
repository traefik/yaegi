package main

import (
	"fmt"

	"guthib.com/generic"
)

type holder struct {
	c generic.Cache[string, []byte]
}

func main() {
	h := holder{c: generic.New[string, []byte]()}
	h.c.Set("a", []byte("hello"), 5)
	fmt.Println(h.c.Get())

	p := &generic.Cache[string, int]{}
	p.Set("b", 2, 3)
	fmt.Println(p.Get())
}

// GOPATH:testdata/generic-cross-pkg
// Output:
// a [104 101 108 108 111] 5ns
// b 2 3ns
