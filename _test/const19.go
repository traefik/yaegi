package main

import (
	"fmt"
	"time"
)

func get10Hours() time.Duration {
	return 10 * time.Hour
}

func main() {
	fmt.Println(get10Hours().String())
}

// Output:
// 10h0m0s
