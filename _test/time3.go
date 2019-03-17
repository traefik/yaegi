package main

import (
	"fmt"
	"time"
)

// FIXME related to named returns
func main() {
	t := time.Date(2009, time.November, 10, 23, 4, 5, 0, time.UTC)
	fmt.Println(t.Clock())
}

// Output:
// 23 4 5
