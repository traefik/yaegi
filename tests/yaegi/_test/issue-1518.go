package main

import "fmt"

func main() {
	n := 0
	for low := 0; ; {
		n++
		if n > 20 || low >= 10 {
			break
		}
		low = low + 3
	}
	fmt.Println(n)
}

// Output:
// 5
