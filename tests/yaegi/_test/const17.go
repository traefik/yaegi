package main

import "fmt"

var t [7/3]int

func main() {
	t[0] = 3/2
	t[1] = 5/2
	fmt.Println(t)
}

// Output:
// [1 2]
