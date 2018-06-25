package main

type Foo struct {
	val int
}

func f(p *Foo) {
	p.val = p.val + 2
}

func main() {
	var a = Foo{3}
	f(&a)
	println(a.val)
}

// Output:
// 5
