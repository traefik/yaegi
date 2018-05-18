package main

type T struct {
	f int
	g int
}

func main() {
	a := T{}
	println(a.f, a.g)
}

// Output:
// 0 0
