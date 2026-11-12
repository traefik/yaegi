package main

type T struct {
	v int
}

type comparator func(T, T) bool

func sort(items []T, comp comparator) {
	println("in sort")
}

func compT(t0, t1 T) bool { return t0.v < t1.v }

func main() {
	a := []T{}
	sort(a, comparator(compT))
}

// Output:
// in sort
