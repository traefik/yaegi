package main

import "fmt"

func main() {
	var a float64 = 64
	a += 64
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b float64 = 64
	b -= 64
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c float64 = 64
	c *= 64
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d float64 = 64
	d /= 64
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	// FIXME expect an error
	// var e float64 = 64
	// e %= 64
	// fmt.Printf("e: %v %T", e, e)
	// fmt.Println()

	fmt.Println(a > b)
	fmt.Println(a >= b)
	fmt.Println(a < b)
	fmt.Println(a <= b)
	fmt.Println(b == d)
}

// Output:
// a: 128 float64
// b: 0 float64
// c: 4096 float64
// d: 1 float64
// true
// true
// false
// false
// false
