package main

import "fmt"

type funclistItem func()

type funclist struct {
	list []funclistItem
}

func main() {
	funcs := funclist{}

	funcs.list = append(funcs.list, func() { fmt.Println("first") })

	for _, f := range funcs.list {
		f()
	}
}

// Output:
// first
