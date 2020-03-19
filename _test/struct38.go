package main

type T struct {
	f func(t *T1)
	y *xxx
}

type T1 struct {
	T
}

type xxx struct{}

var (
	x1 *T1 = x
	x      = &T1{}
)

func main() {
	println("ok")
}

// Output:
// ok
