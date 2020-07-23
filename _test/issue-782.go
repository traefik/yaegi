package main

import "fmt"

func main() {
	a := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	from := uint32(2)
	to := uint32(4)
	b := a[from:to]
	fmt.Print(b)
}

// Output:
// [3 4]
