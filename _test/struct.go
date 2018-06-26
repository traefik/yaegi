package main

type T struct {
	f int
	g int
}

func main() {
	a := T{7, 8}
	println(a.f, a.g)
}

// Output:
// 7 8
