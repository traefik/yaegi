package main

type T struct {
	Name string
}

func (t *T) foo(a string) string {
	return t.Name + a
}

var g = &T{"global"}

var f = g.foo

func main() {
	println(f("-x"))
}

// Output:
// global-x
