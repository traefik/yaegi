package main

import "fmt"

func main() {
	a := complex(float64(3), float64(2))
	fmt.Println(a, real(a), imag(a))
}

// Output:
// (3+2i) 3 2
