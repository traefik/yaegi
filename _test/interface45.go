package main

import "fmt"

func main() {
	var i any = 1
	var s struct{}
	s, _ = i.(struct{})
	fmt.Println(s)
}

// Output:
// {}
