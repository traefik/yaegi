package main

import (
	"fmt"
	"time"
)

var t time.Time

func f() time.Time {
	time := t
	return time
}

func main() {
	fmt.Println(f())
}

// Output:
// 0001-01-01 00:00:00 +0000 UTC
