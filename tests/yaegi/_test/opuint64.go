package main

import "fmt"

func main() {
	var a uint64 = 64
	a += 64
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b uint64 = 64
	b -= 64
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c uint64 = 64
	c *= 64
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d uint64 = 64
	d /= 64
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	var e uint64 = 64
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
// a: 128 uint64
// b: 0 uint64
// c: 4096 uint64
// d: 1 uint64
// e: 0 uint64
// true
// true
// false
// false
// true
