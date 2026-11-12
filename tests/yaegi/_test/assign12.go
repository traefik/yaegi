package main

import "fmt"

func main() {
	a, b, c := fmt.Println("test")
	println(a, b, c)
}

// Error:
// 6:2: assignment mismatch: 3 variables but fmt.Println returns 2 values
