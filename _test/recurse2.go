package main

type F func(a *A)

type A struct {
	B string
	D
	f F
}

type D struct {
	*A
	E *A
}

func f1(a *A) { println("in f1", a.B) }

func main() {
	a := &A{B: "b", f: f1}
	a.D = D{E: a}
	println(a.D.E.B)
	a.f(a)
}

// Output:
// b
// in f1 b
