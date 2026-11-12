package main

import "fmt"

var a = &[]*T{}

type T struct{ name string }

func main() {
	fmt.Println(a)
}

// Output:
// &[]
