package main

type F func(a *A)

type A struct {
	Name string
	F
}

func main() {
	a := &A{"Test", func(a *A) { println("in f", a.Name) }}
	a.F(a)
}

// Output:
// in f Test
