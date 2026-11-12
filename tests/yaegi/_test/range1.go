package main

import "fmt"

func main() {
	a := [...]int{2, 1, 0}
	for _, v := range a {
		a[v] = v
	}
	fmt.Println(a)
}

// Output:
// [0 1 2]
