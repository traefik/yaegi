package main

import "fmt"

func f(a ...int) {
	if len(a) > 2 {
		fmt.Println(a[2])
	}
}

func main() {
	f(1, 2, 3, 4)
}

// Output:
// 3
