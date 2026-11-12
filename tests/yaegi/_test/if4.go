package main

const bad = false

func main() {
	a := 0
	if bad {
		println("false")
		a = 1
	} else {
		println("true")
		a = -1
	}
	println(a)
}

// Output:
// true
// -1
