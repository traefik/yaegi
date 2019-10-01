package main

import (
	"fmt"
	"math"
)

func main() {
	m := map[float64]bool{math.NaN(): true, math.NaN(): true, math.NaN(): true}
	for _, v := range m {
		fmt.Println(v)
	}
}

// Output:
// true
// true
// true
