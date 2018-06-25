package main

import "fmt"

func f(s string, a ...int) {
	fmt.Println(s, a)
}

func main() {
	f("hello")
}

// Output:
// hello []
