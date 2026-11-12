package main

func main() {
	a := 0
	if false {
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
