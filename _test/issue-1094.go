package main

import "fmt"

func main() {
	var x any
	x = "a" + fmt.Sprintf("b")
	fmt.Printf("%v %T\n", x, x)
}

// Output:
// ab string
