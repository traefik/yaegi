package main

type T1 struct {
	A []T2
	M map[uint64]T2
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
