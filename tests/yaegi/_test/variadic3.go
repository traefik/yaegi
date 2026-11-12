package main

import "fmt"

func f(a ...int) int {
	fmt.Println(a)
	res := 0
	for _, v := range a {
		res += v
	}
	return res
}

func main() {
	fmt.Println(f(1, 2, 3, 4))
}

// Output:
// [1 2 3 4]
// 10
