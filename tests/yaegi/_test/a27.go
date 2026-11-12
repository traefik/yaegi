package main

import "fmt"

func main() {
	a := [...]string{"hello", "world"}
	fmt.Printf("%v %T\n", a, a)
}

// Output:
// [hello world] [2]string
