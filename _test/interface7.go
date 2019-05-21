package main

import "fmt"

type T int

func (t T) Error() string { return fmt.Sprintf("This is an error from T: %d", t) }

func f(t T) error { return t }

func main() {
	x := T(1)
	fmt.Println(f(x))
}

// Output:
// This is an error from T: 1
