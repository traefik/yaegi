package main

type T struct {
	v bool
}

func main() {
	a := T{}
	if a.v {
		println("ok")
	} else {
		println("nok")
	}
}

// Output:
// nok
