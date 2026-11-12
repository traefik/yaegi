package main

import "fmt"

func newT2() *T2 { return &T2{} }

type T2 struct {
	T1
}

type T1 struct {
	bs []byte
}

func main() {
	fmt.Println(newT2())
}

// Output:
// &{{[]}}
