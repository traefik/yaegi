package main

import "fmt"

func main() {
	m := map[int]bool{1: true, 3: true, 5: true}
	var n int
	for range m {
		n++
	}
	fmt.Println(n)
}

// Output:
// 3
