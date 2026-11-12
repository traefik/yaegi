package main

import (
	"fmt"
)

func main() {
	const tooBig = 1267650600228229401496703205376
	const huge = 1 << 100
	const large = huge >> 38

	fmt.Println(int64(large))
}

// Output:
// 4611686018427387904
