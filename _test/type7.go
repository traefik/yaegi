package main

import "fmt"

func main() {
	var i interface{} = "hello"
	s, ok := i.(string)
	fmt.Println(s, ok)
}

// Output:
// hello
