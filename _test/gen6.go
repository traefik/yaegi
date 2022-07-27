package main

import "fmt"

func MapKeys[K comparable, V any](m map[K]V) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}

func main() {
	var m = map[int]string{1: "2", 2: "4", 4: "8"}

	// Test type inference
	fmt.Println("keys m:", MapKeys(m))
}

// Output:
// keys m: [1 2 4]
