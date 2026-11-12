package main

import "fmt"

func f(a ...int) {
	fmt.Println(a)
}

func main() {
	f(1, 2, 3, 4)
}

// Output:
// [1 2 3 4]
