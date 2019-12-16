package main

type T1 struct {
	A []T2
	B []T2
}

type T2 struct {
	name string
}

var t = T1{}

func main() {
	println("ok")
}

// Output:
// ok
