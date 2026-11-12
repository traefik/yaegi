package main

import "fmt"

func main() {
	a := []map[int]int{make(map[int]int)}

	for _, b := range a {
		fmt.Println(b)
	}
}

// Output:
// map[]
