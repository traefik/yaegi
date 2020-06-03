package main

import "fmt"

type A struct {
	B string
	C D
}

func (a *A) Test() string {
	return "test"
}

type D struct {
	E *A
}

func main() {
	a := &A{B: "b"}
	d := D{E: a}
	a.C = d
	fmt.Println(a.C.E.Test())
}

// Output:
// test
