package main

import "fmt"

func main() {
	var i any = "hello"
	if s, ok := i.(string); ok {
		fmt.Println(s, ok)
	}
}

// Output:
// hello true
