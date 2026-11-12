package main

import "fmt"

func main() {
	var a float32 = 64
	a += 64
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b float32 = 64
	b -= 64
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c float32 = 64
	c *= 64
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d float32 = 64
	d /= 64
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	// FIXME expect an error
	// var e float32 = 64
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
// a: 128 float32
// b: 0 float32
// c: 4096 float32
// d: 1 float32
// true
// true
// false
// false
// false
