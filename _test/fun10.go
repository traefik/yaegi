package main

import "fmt"

func f() func() {
	return nil
}

func main() {
	g := f()
	fmt.Printf("%T %v\n", g, g)
	if g == nil {
		fmt.Println("nil func")
	}
}

// Output:
// func() <nil>
// nil func
