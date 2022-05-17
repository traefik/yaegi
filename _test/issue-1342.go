package main

import "fmt"

func main() {
	var a interface{}
	a = "a"
	fmt.Println(a, a == "a")
}

// Output:
// a true
