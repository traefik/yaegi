package main

import "fmt"

func div(a, b int) (result int) {
	defer func() {
		r := recover()

		fmt.Printf("r = %#v\n", r)

		if r != nil {
			result = 0
		}
	}()

	return a / b
}

func main() {
	println(div(30, 2))
}

// Output:
// r = <nil>
// 15
