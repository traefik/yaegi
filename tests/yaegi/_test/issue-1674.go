package main

import "fmt"

func main() {
	fmt.Println(max(1, 2), min(3, 1, 2), max(1.5, 2), min("b", "a"))
}

// Output:
// 2 1 2 a
