package main

import (
	"fmt"
	"strings"
)

func Bar(s string) bool {
	a := strings.HasPrefix("fas", "f")
	b := strings.HasPrefix("aaaaa", "a")
	a_and_b := strings.HasPrefix("fas", "f") && strings.HasPrefix("aaaaa", "a")
	fmt.Println(a, b, a && b, a_and_b)
	return a && b
}

func main() {
	println(Bar("kung"))
}
