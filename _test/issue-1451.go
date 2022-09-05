package main

type t1 uint8

const (
	n1 t1 = iota
	n2
)

type T struct {
	elem [n2 + 1]int
}

func main() {
	println(len(T{}.elem))
}

// Output:
// 2
