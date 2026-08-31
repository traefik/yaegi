package main

import "fmt"

func main() {
	fmt.Println(max(1, 2), min(1, 2))
	fmt.Println(max(1.5, 2.5), min("abc", "abd"))
	var a, b int = 7, 3
	fmt.Println(max(a, b), min(a, b))
	fmt.Println(min(a, b, 9))
	type MyInt int
	var x, y MyInt = 5, 9
	fmt.Println(max(x, y))
	var i8 int8 = min(3, 10)
	var f32 float32 = max(1.5, 2.5)
	fmt.Println(i8, f32)
}

// Output:
// 2 1
// 2.5 abc
// 7 3
// 3
// 9
// 3 2.5
