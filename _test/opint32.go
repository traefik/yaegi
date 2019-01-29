package main

import "fmt"

func main() {
	var a int32 = 64
	a += 64
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b int32 = 64
	b -= 64
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c int32 = 64
	c *= 64
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d int32 = 64
	d /= 64
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	var e int32 = 64
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
// a: 128 int32
// b: 0 int32
// c: 4096 int32
// d: 1 int32
// e: 0 int32
// true
// true
// false
// false
// true
