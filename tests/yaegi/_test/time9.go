package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println((5 * time.Minute).Seconds())
}

// Output:
// 300
