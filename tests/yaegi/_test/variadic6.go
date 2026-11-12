package main

import "fmt"

type A struct {
}

func (a A) f(vals ...bool) {
	for _, v := range vals {
		fmt.Println(v)
	}
}
func main() {
	bools := []bool{true}
	a := A{}
	a.f(bools...)
}

// Output:
// true
