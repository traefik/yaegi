package main

func run(fn func(name string)) { fn("test") }

type T2 struct {
	name string
}

func (t *T2) f(s string) { println(s, t.name) }

func main() {
	t2 := &T2{"foo"}
	run(t2.f)
}

// Output:
// test foo
