package main

import "fmt"

type rule uint8

const (
	r0 rule = iota
	r1
	r2
)

var a = [...]int{
	r0: 1,
	r1: 12,
}

func main() {
	fmt.Println(a)
}

// Output:
// [1 12]
