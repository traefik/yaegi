package main

func main() {
	var c chan<- struct{} = make(chan struct{})

	for _ = range c {
	}
}

// Error:
// _test/range9.go:6:16: invalid operation: range c receive from send-only channel
