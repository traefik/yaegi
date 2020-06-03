package main

func main() {
	var foo int
	foo = 2

	type foo struct{}
	var bar foo
	println(bar)
}

// Error:
// ../_test/redeclaration3.go:7:7: foo redeclared in this block
