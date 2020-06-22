package main

import (
	"fmt"
	"time"
)

func main() {
	fn1 := func(args ...*time.Duration) string {
		return ""
	}

	fmt.Printf("%T\n", fn1)
}

// Output:
// func(...*time.Duration) string
