package main

func main() {
	var c chan<- struct{} = make(chan struct{})
	var d <-chan struct{} = c

	_ = d
}

// Error:
// _test/assign15.go:5:26: cannot use type chan<- struct {} as type <-chan struct {} in assignment
