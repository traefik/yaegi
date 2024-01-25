package main

func main() {
	a, b, c := 1, 2
	_, _, _ = a, b, c
}

// Error:
// _test/assign19.go:4:2: cannot assign 2 values to 3 variables
