package main

import "fmt"

func main() {
	var a int16 = 64
	a += 64
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b int16 = 64
	b -= 64
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c int16 = 64
	c *= 64
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d int16 = 64
	d /= 64
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	var e int16 = 64
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
// a: 128 int16
// b: 0 int16
// c: 4096 int16
// d: 1 int16
// e: 0 int16
// true
// true
// false
// false
// true
