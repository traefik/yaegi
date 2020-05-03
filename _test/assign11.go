package main

import "fmt"

func main() {
	_, _, _ = fmt.Println("test")
}

// Error:
// 6:2: assignment mismatch: 3 variables but fmt.Println returns 2 values
