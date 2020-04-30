package main

import "fmt"

func f() interface{} {
	return make(map[int]int)
}

func main() {
	a, ok := f().(map[int]int)
	fmt.Println(a, ok)
}

// Output:
// map[] true
