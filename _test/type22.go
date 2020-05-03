package main

type T1 T

func foo() T1 {
	return T1(T{"foo"})
}

type T struct {
	Name string
}

func main() {
	println(foo().Name)
}

// Output:
// foo
