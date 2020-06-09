package main

var time int

func time() string {
	return "hello"
}

func main() {
	t := time()
	println(t)
}

// Error:
// ../_test/redeclaration-global5.go:5:1: time redeclared in this block
