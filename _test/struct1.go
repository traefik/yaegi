package main

type T struct {
	f int
	g struct {
		h int
	}
}

func main() {
	a := T{}
	a.g.h = 3 + 2
	println("a.g.h", a.g.h)
}

// Output:
// a.g.h 5
