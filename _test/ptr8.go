package main

var a = func() *bool { b := true; return &b }()

func main() {
	println(*a && true)
}

// Output:
// true
