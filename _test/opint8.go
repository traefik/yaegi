package main

import "fmt"

func main() {
	var a int8 = 6
	a += 6
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	var b int8 = 6
	b -= 6
	fmt.Printf("b: %v %T", b, b)
	fmt.Println()

	var c int8 = 6
	c *= 6
	fmt.Printf("c: %v %T", c, c)
	fmt.Println()

	var d int8 = 6
	d /= 6
	fmt.Printf("d: %v %T", d, d)
	fmt.Println()

	var e int8 = 6
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
// a: 12 int8
// b: 0 int8
// c: 36 int8
// d: 1 int8
// e: 0 int8
// true
// true
// false
// false
// true
