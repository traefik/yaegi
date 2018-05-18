package main

type T struct {
	f int
}

func main() {
	a := T{}
	println(a.f)
	a.f = 8
	println(a.f)
}

// Output:
// 0
// 8
