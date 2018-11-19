package main

import "fmt"

func f(s string, a ...int32) {
	fmt.Println(s, a)
}

func main() {
	f("hello", 1, 2, 3)
}

// Output:
// hello [1 2 3]
