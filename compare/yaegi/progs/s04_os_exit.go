package main

import (
	"fmt"
	"os"
)

func main() {
	defer fmt.Println("deferred")
	fmt.Println("before")
	os.Exit(7)
}
