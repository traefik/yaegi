package main

import "fmt"

type A struct {
	B string
	C D
}

type D struct {
	F *A
	E *A
}

func main() {
	a := &A{B: "b"}
	a.C = D{E: a}
	fmt.Println(a.C.E.B)
}

// Output:
// b
