package main

type T int

func (t T) foo() { println("foo", t) }

func main() {
	var t T = 2
	t.foo()
}
