package main

import "fmt"

func main() {
	var a int64 = 64
	a += 64
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b int64 = 64
	b -= 64
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c int64 = 64
	c *= 64
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d int64 = 64
	d /= 64
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	var e int64 = 64
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
// a: 128 int64
// b: 0 int64
// c: 4096 int64
// d: 1 int64
// e: 0 int64
// true
// true
// false
// false
// true
