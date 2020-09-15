package main

type A struct {
	C D
}

type D struct {
	E string
}

func main() {
	a := A{}
	a.C = D{E: "bb"}

	println(a.C.E)
}

// Output:
// bb
