package main

import "fmt"

type T struct{}

var a = []T{{}}

func main() {
	fmt.Println(a)
}

// Output:
// [{}]
