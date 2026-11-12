package main

import "fmt"

func main() {
	a, b := 1, 1
	for i := 0; i < 10; i++ {
		fmt.Println(a)
		a, b = b, a+b
	}
}

// Output:
// 1
// 1
// 2
// 3
// 5
// 8
// 13
// 21
// 34
// 55
