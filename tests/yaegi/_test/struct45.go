package main

type T struct {
	b bool
}

type T1 struct {
	T
}

func main() {
	t := &T1{}
	t.b = true
	println(t.b)
}

// Output:
// true
