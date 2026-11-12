package main

import "fmt"

func main() {
	i := 0
	for i = 0; ; {
		i = i + 1
		if i >= 4 {
			break
		}
	}
	n := 0
	for low, high := 0, 8; ; {
		low = low + 3
		if low >= high {
			break
		}
		n++
	}
	fmt.Println(i, n)
}

// Output:
// 4 2
