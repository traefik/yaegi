package main

import (
	"fmt"
	"os"
)

func main() {
	p, err := os.FindProcess(os.Getpid())
	fmt.Println(p, err)
}

// Output:
// <nil> restricted
