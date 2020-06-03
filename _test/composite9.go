package main

import "fmt"

func main() {
	a := [][]int{make([]int,0)}

	for _, b := range a {
		fmt.Println(b)
	}
}

// Output:
// []
