package main

func main() {
	type foo struct {
		yolo string
	}

	var foo int
	foo = 2
	println(foo)
}

// Error:
// ../_test/redeclaration0.go:8:6: foo redeclared in this block
//	previous declaration at ../_test/redeclaration0.go:4:7
