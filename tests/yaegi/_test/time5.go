package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.Unix(1e9, 0).In(time.UTC)
	fmt.Println(t.Minute())
}

// Output:
// 46
