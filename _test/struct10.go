package main

type T struct {
	f int
	g int64
}

func main() {
	a := T{g: 8}
	println(a.f, a.g)
}

// Output:
// 0 8
