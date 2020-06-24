package main

func main() {
	var i complex128 = 1i
	var f complex128 = 0.4i

	print(i > f)
}

// Error:
// _test/op9.go:7:8: invalid operation: operator > not defined on complex128
