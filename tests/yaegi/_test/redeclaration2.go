package main

func main() {
	var foo struct {
		yolo string
	}

	var foo int
	foo = 2
	println(foo)
}

// Error:
// ../_test/redeclaration2.go:8:6: foo redeclared in this block
//	previous declaration at ../_test/redeclaration2.go:4:6
