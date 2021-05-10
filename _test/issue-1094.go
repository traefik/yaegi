package main

import "fmt"

func main() {
	var x interface{}
	x = "a" + fmt.Sprintf("b")
	fmt.Printf("%v %T\n", x, x)
}

// Ouput:
// ab string
