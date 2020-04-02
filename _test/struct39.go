package main

type T struct {
	t *T1
	y *xxx
}

type T1 struct {
	T
}

var x = &T1{}
var t = &T{}

type xxx struct{}

func main() {
	println("ok")
}

// Output:
// ok
