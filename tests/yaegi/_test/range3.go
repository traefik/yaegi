package main

import "fmt"

func main() {
	m := map[int]bool{1: true, 3: true, 5: true}
	for k := range m {
		m[k*2] = true
	}
	fmt.Println("ok")
}

// Output:
// ok
