package main

import "fmt"

type T struct{}

func newT() (T, error) { return T{}, nil }

func main() {
	var (
		i   any
		err error
	)
	i, err = newT()
	fmt.Println(i, err)
}

// Output:
// {} <nil>
