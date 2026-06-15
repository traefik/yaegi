package main

import "fmt"

func f(a, b float64) any { return complex(a, b) }

func main() {
	a := f(3, 2)
	fmt.Println(a.(complex128))
}

// Output:
// (3+2i)
