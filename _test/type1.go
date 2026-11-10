package main

import "fmt"

func main() {
	var i any = "hello"
	s := i.(string)
	fmt.Println(s)
}

// Output:
// hello
