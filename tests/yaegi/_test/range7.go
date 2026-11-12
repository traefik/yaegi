package main

import (
	"fmt"
)

func someChan() <-chan struct{} {
	c := make(chan struct{}, 1)
	c <- struct{}{}
	return c
}

func main() {
	for _ = range someChan() {
		fmt.Println("success")
		return
	}
}

// Output:
// success