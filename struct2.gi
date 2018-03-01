package main

type T struct {
	f int
	g int
}

func main() {
	a := T{f: 7, g: 8}
	println(a.f, a.g)
}
