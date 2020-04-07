package main

type T struct {
	t func(*T)
	y *xxx
}

func f(t *T) { println("in f") }

type xxx struct{}

func main() {
	x := &T{}
	x.t = f
	println("ok")
}

// Output:
// ok
