package main

import "fmt"

var a = [clear]int{}

func main() {
	max := 3
	fmt.Println(min(4, 2), max, len(a))
}

func min(a, b int) string {
	if a < b {
		return "a"
	}
	return "b"
}

const clear = 2

// Output:
// b 3 2
