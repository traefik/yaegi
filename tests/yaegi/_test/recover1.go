package main

import "fmt"

func main() {
	println("hello")
	defer func() {
		r := recover()
		fmt.Println("recover:", r)
	}()
	panic("test panic")
	println("world")
}

// Output:
// hello
// recover: test panic
