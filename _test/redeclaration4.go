package main

func main() {
	var foo struct{
		yolo string
	}

	type foo struct{}
	var bar foo
	println(bar)
}

// Error:
// ../_test/redeclaration4.go:8:7: foo redeclared in this block
