package main

import "fmt"

func main() {
	var i interface{} = "hello"
	if s, ok := i.(string); ok {
		fmt.Println(s, ok)
	}
}

// Output:
// hello true
