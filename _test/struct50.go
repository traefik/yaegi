package main

type Node struct {
	foo []*Node
}

func main() {
	a := Node{foo: []*Node{{}}}
	println(len(a.foo))
}

// Output:
// 1
