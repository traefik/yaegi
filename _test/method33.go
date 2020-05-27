package main

import (
	"fmt"
)

type T1 struct{}

func (t1 T1) f() {
	fmt.Println("T1.f()")
}

func (t1 T1) g() {
	fmt.Println("T1.g()")
}

type T2 struct {
	T1
}

func (t2 T2) f() {
	fmt.Println("T2.f()")
}

type I interface {
	f()
}

func printType(i I) {
	if t1, ok := i.(T1); ok {
		println("T1 ok")
		t1.f()
		t1.g()
	}

	if t2, ok := i.(T2); ok {
		println("T2 ok")
		t2.f()
		t2.g()
	}
}

func main() {
	println("T1")
	printType(T1{})
	println("T2")
	printType(T2{})
}

// Output:
// T1
// T1 ok
// T1.f()
// T1.g()
// T2
// T2 ok
// T2.f()
// T1.g()
