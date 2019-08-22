package main

var a = &T{}

type T struct{}

func main() {
	println(a != nil)
}

// Output:
// true
