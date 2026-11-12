package main

import "fmt"

var foo = make([]int, 1)

func main() {
	for _, v := range foo {
		fmt.Println(v)
	}
}

// Output:
// 0
