package main

import "fmt"

const (
	zero = iota
	one
	two
	three
)

func main() {
	a := [...]string{
		zero:      "zero",
		one:       "one",
		three:     "three",
		three + 2: "five",
	}
	fmt.Printf("%v %T\n", a, a)
}

// Output:
// [zero one  three  five] [6]string
