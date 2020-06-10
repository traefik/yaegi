package main

import "fmt"

func main() {
	var a, b string

	pattern := "%s %s"
	dest := []interface{}{&a, &b}

	n, err := fmt.Sscanf("test1 test2", pattern, dest...)
	if err != nil || n != len(dest) {
		println("error")
		return
	}
	println(a, b)
}

// Output:
// test1 test2
