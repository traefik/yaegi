package main

type T struct {
	t func(*T)
	y *xxx
}

func f(t *T) { println("in f") }

var x = &T{t: f}

type xxx struct{}

func main() {
	println("ok")
}

// Output:
// ok
