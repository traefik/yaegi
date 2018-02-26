package main

type T struct {
	f int
	g int
}

func main() {
	a := T{ 1, 2 }
	println(a.f, a.g)
}
