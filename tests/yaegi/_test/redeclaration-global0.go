package main

type time int

var time string

func main() {
	time = "hello"
	println(time)
}

// Error:
// ../_test/redeclaration-global0.go:5:5: time redeclared in this block
//	previous declaration at ../_test/redeclaration-global0.go:3:6
