package main

import "fmt"

func main() {
	ch := make(chan string, 4)
	ch <- "r"
	ch <- "s"
	ch <- "t"
	ch <- "u"

	var x, y string

	// A receive and a constant in the same assignment: both destinations must be set.
	x, y = <-ch, "b"
	fmt.Println(x, y)

	x, y = "b", <-ch
	fmt.Println(x, y)

	a, b := <-ch, "c"
	fmt.Println(a, b)

	c, d := "d", <-ch
	fmt.Println(c, d)
}

// Output:
// r b
// b s
// t c
// d u
