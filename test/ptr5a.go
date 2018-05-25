package main

type Foo struct {
	val int
}

func main() {
	var a = Foo{3}
	b := &a
	println(b.val)
}

// Output:
// 3
