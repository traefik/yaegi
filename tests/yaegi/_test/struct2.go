package main

type T struct {
	f int
	g int
}

func main() {
	a := T{g: 8, f: 7}
	println(a.f, a.g)
}

// Output:
// 7 8
