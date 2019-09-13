package main

type Edge interface {
	ReverseEdge() Edge
}

func main() {
	println("hello")
}

// Output:
// hello
