package main

import "fmt"

var a = []int{1, 2, 3}

func f() { fmt.Println(a) }

func main() {
	fmt.Println(a)
	a = []int{6, 7}
	f()
}

// Output:
// [1 2 3]
// [6 7]
