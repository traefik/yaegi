package main

import "fmt"

func f(c complex128) any { return real(c) }

func main() {
	c := complex(3, 2)
	a := f(c)
	fmt.Println(a.(float64))
}

// Output:
// 3
