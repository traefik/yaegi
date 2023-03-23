package main

type F func(a *A)

type A struct {
	B string
	D
}

type D struct {
	*A
	E *A
	f F
}

func f1(a *A) { println("in f1", a.B) }

func main() {
	a := &A{B: "b"}
	a.D = D{f: f1}
	a.f(a)
}

// Output:
// in f1 b
