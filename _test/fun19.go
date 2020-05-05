package main

import (
	"fmt"
)

func foo() ([]string, error) {
	return nil, fmt.Errorf("bar")
}

func main() {
	a, b := foo()
	fmt.Println(a, b)
}

// Output:
// [] bar
