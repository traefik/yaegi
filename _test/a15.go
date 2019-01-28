package main

import "fmt"

const size = 12

func main() {
	var buf [size]int
	fmt.Println(buf[:])
}

// Output:
// [0 0 0 0 0 0 0 0 0 0 0 0]
