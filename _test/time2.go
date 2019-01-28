package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.Date(2009, time.November, 10, 23, 4, 5, 0, time.UTC)
	h, m, s := t.Clock()
	fmt.Println(h, m, s)
}

// Output:
// 23 4 5
