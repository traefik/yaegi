package main

type T struct {
	f int
	g int
}

func f(i int) int { return i * i }

func main() {
	a := T{7, f(4)}
	println(a.f, a.g)
}

// Output:
// 7 16
