package main

import "fmt"

type A struct {
	B string
	D
}

type D struct {
	*A
	E *A
}

func main() {
	a := &A{B: "b"}
	a.D = D{E: a}
	fmt.Println(a.D.E.B)
}

// Output:
// b
