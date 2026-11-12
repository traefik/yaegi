package main

import (
	"fmt"
)

func interfaceAsInts() {
	var a interface{}
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
	var a, b, c interface{}
	b = 2
	c = 3
	a = []interface{}{b, c}

	d, ok := a.([]interface{})
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
