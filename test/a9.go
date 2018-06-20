package main

import "fmt"

var samples = []int{}

func main() {
	samples = append(samples, 1)
	fmt.Println(samples)
}

// Output:
// [1]
