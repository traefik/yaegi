package main

import "fmt"

func main() {
	m := map[int]bool{1: true, 3: true, 5: true}
	for _, v := range m {
		fmt.Println(v)
	}
}

// Output:
// true
// true
// true
