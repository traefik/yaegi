package main

import (
	"fmt"
)

type A struct {
}

func (a A) f(vals ...bool) {
	for _, v := range vals {
		fmt.Println(v)
	}
}

func main() {
	a := A{}
	a.f(true)
}

// Output:
// true
