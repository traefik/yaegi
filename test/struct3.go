package main

type T struct {
	f int
	g int
	h struct {
		k int
	}
}

func f(i int) int { return i + 3 }

func main() {
	a := T{}
	a.h.k = f(4)
	println(a.h.k)
}

// Output:
// 7
