package main

type Foo struct {
	val int
}

func main() {
	var a = &Foo{3}
	println(a.val)
}

// Output:
// 3
