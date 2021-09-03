package main

import (
	"fmt"
)

func init() {
	fmt.Println(constString)
	fmt.Println(const2)
	fmt.Println(varString)
}

const constString string = "hello"

const (
	const1 = iota + 10
	const2
	const3
)

var varString string = "test"

func main() {}

// Output:
// hello
// 11
// test
