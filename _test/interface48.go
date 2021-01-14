package main

import "fmt"

type I1 interface{ A }

type A = I2

type I2 interface{ F() I1 }

func main() {
	var i I1
	fmt.Println(i)
}

// Output:
// <nil>
