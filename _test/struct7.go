package main

type Opt struct {
	b bool
}

type T struct {
	i   int
	opt Opt
}

func main() {
	a := T{}
	println(a.i, a.opt.b)
}

// Output:
// 0 false
