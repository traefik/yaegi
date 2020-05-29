package main

func main() {
	type foo struct{
		yolo string
	}

	type foo struct{}
	var bar foo
	println(bar)
}

// Error:
// ../_test/redecleration5.go:8:7: foo redeclared in this block
