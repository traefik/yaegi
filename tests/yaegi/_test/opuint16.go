package main

import "fmt"

func main() {
	var a uint16 = 64
	a += 64
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b uint16 = 64
	b -= 64
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c uint16 = 64
	c *= 64
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d uint16 = 64
	d /= 64
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	var e uint16 = 64
	e %= 64
	fmt.Printf("e: %v %T", e, e)
	fmt.Println()

	fmt.Println(a > b)
	fmt.Println(a >= b)
	fmt.Println(a < b)
	fmt.Println(a <= b)
	fmt.Println(b == e)
}

// Output:
// a: 128 uint16
// b: 0 uint16
// c: 4096 uint16
// d: 1 uint16
// e: 0 uint16
// true
// true
// false
// false
// true
