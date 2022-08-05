package main

func genFunc() (f func()) {
	return f
}

func main() {
	println(genFunc() == nil)
}

// Output:
// true
