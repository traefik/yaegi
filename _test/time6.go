package main

import (
	"fmt"
	"time"
)

func main() {
	t := &time.Time{}
	t.UnmarshalText([]byte("1985-04-12T23:20:50.52Z"))

	fmt.Println(t)
}

// Output:
// 1985-04-12 23:20:50.52 +0000 UTC
