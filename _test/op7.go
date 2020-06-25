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
// _test/op7.go:11:5: invalid operation: operator > not defined on error
