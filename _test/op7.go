package main

type T int

func (t T) Error() string { return "T: error" }

var invalidT T

func main() {
	var err error
	if err > invalidT {
		println("ok")
	}
}

// Error:
// _test/op7.go:11:5: illegal operand types for '>' operator
