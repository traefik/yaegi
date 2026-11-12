package main

import (
	"fmt"
)

type A struct {
	B map[string]*B
	C map[string]*C
}

type C struct {
	D *D
	E *E
}

type D struct {
	F *F
	G []G
}

type E struct {
	H []H
	F *F
}

type B struct{}
type F struct{}
type G struct{}
type H struct{}

func main() {
	conf := &A{
		B: make(map[string]*B),
		C: make(map[string]*C),
	}
	fmt.Println(conf)
}

// Output:
// &{map[] map[]}
