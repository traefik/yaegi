package main

import "fmt"

type fn func(string, string) bool

var funcs = []fn{
	cmpLessFn,
	cmpGreaterFn,
	nil,
}

func cmpLessFn(a string, b string) bool {
	return a < b
}

func cmpGreaterFn(a string, b string) bool {
	return a > b
}

func main() {
	for _, f := range funcs {
		if f == nil {
			continue
		}
		fmt.Println(f("a", "b"))
	}
}

// Output:
// true
// false
