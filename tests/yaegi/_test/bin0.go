package main

import "strings"

func main() {
	a := strings.SplitN("truc machin", " ", 2)
	println(a[0])
}

// Output:
// truc
