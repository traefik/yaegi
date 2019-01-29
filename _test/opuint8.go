package main

import "fmt"

func main() {
	var a uint8 = 6
	a += 6
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b uint8 = 6
	b -= 6
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c uint8 = 6
	c *= 6
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d uint8 = 6
	d /= 6
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	var e uint8 = 6
	e %= 6
	fmt.Printf("e: %v %T", e, e)
	fmt.Println()

	fmt.Println(a > b)
	fmt.Println(a >= b)
	fmt.Println(a < b)
	fmt.Println(a <= b)
	fmt.Println(b == e)
}

// Output:
// a: 12 uint8
// b: 0 uint8
// c: 36 uint8
// d: 1 uint8
// e: 0 uint8
// true
// true
// false
// false
// true
