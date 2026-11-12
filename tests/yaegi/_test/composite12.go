package main

type A struct {
	C D
}

type D struct {
	E string
}

func main() {
	a := A{}
	a.C = D{"bb"}

	println(a.C.E)
}

// Output:
// bb
