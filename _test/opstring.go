package main

import "fmt"

func main() {
	a := "hhh"
	a += "fff"
	fmt.Printf("a: %v %T", a, a)
	fmt.Println()

	// b := "hhh"
	// b -= "fff" // FIXME expect an error
	// fmt.Printf("b: %v %T", b, b)
	// fmt.Println()
	//
	// c := "hhh"
	// c *= "fff" // FIXME expect an error
	// fmt.Printf("c: %v %T", c, c)
	// fmt.Println()
	//
	// d := "hhh"
	// d /= "fff" // FIXME expect an error
	// fmt.Printf("d: %v %T", d, d)
	// fmt.Println()
	//
	// e := "hhh"
	// e %= "fff" // FIXME expect an error
	// fmt.Printf("e: %v %T", e, e)
	// fmt.Println()

	// FIXME panic
	// fmt.Println(a > "ggg")
	// fmt.Println(a >= "ggg")
	// fmt.Println(a < "ggg")
	// fmt.Println(a <= "ggg")
	// fmt.Println(a == "hhhfff")
}

// Output:
// a: hhhfff string
