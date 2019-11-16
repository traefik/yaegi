package main

import "fmt"

func main() {
	var err error

	switch v := err.(type) {
	case fmt.Formatter:
		println("formatter")
	default:
		fmt.Println(v)
	}
}

// Output:
// <nil>
