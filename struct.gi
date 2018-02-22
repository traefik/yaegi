package main

type T struct {
	//f, g int
	f int
	g int
	h struct {
		i int
		j int
	}
}

func main() {
	a := T{1, 2}
	println(a.f, a.g)
}
