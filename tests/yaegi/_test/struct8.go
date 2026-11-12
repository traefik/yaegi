package main

type T3 struct {
	k int
}

type T2 struct {
	h int
	T3
}

type T struct {
	f int
	g int
	T2
}

func f(i int) int { return i * i }

func main() {
	a := T{5, 7, T2{8, T3{9}}}
	println(a.f, a.g, a.T2.h, a.T2.T3.k)
}

// Output:
// 5 7 8 9
