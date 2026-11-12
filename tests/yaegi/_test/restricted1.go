package main

import (
	"fmt"
	"os"
)

func main() {
	defer func() {
		r := recover()
		fmt.Println("recover:", r)
	}()
	os.Exit(1)
	println("not printed")
}

// Output:
// recover: os.Exit(1)
