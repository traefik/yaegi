package main

type T1 t1

type t1 int8

const (
	P2 T1 = 2
	P3 T1 = 3
)

func main() {
	println(P3)
}

// Output:
// 3
