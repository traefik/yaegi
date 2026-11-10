package main

import (
	"fmt"
)

func interfaceAsInts() {
	var a any
	b := 2
	c := 3
	a = []int{b, c}

	d, ok := a.([]int)
	if !ok {
		println("nope")
		return
	}

	for _, v := range d {
		fmt.Println(v)
	}
}

func interfaceAsInterfaces() {
	var a, b, c any
	b = 2
	c = 3
	a = []any{b, c}

	d, ok := a.([]any)
	if !ok {
		println("nope")
		return
	}
	fmt.Println(d)

	for _, v := range d {
		fmt.Println(v)
	}
}

func main() {
	interfaceAsInts()
	interfaceAsInterfaces()
}

// Output:
// 2
// 3
// [2 3]
// 2
// 3
