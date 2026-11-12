package main

type T struct {
	a int
	b string
}

var t = T{1, "hello"}

func main() {
	println(t.a, t.b)
}

// Output:
// 1 hello
