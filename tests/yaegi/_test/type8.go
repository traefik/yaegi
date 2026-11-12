package main

import (
	"fmt"
	"time"
)

func main() {
	v := (*time.Time)(nil)
	fmt.Println(v)
}

// Output:
// <nil>
