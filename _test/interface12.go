package main

type I1 interface {
	Truc()
}

type T1 struct{}

func (T1) Truc() { println("in T1 truc") }

var x I1 = T1{}

func main() {
	x.Truc()
}

// Output:
// in T1 truc
