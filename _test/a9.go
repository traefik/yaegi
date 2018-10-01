package main

import "fmt"

var (
	samples = []int{}
	b       = 1
)

func main() {
	samples = append(samples, 1)
	fmt.Println(samples)
}

// Output:
// [1]
