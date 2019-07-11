package main

import "fmt"

const (
	zero = iota
	one
	two
)

func main() {
	a := [...]string{
		zero: "zero",
		one:  "one",
		two:  "two",
	}
	fmt.Printf("%v %T\n", a, a)
}

// Output:
// [zero one two] [3]string
