package main

import "fmt"

func (t MyT) Test() string {
	return "hello"
}

type MyT int

func main() {
	t := MyT(1)

	fmt.Println(t.Test())
}

// Output:
// hello
