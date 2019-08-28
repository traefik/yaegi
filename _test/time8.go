package main

import (
	"time"
)

type durationValue time.Duration

func (d *durationValue) String() string { return (*time.Duration)(d).String() }

func main() {
	var d durationValue
	println(d.String())
}

// Output:
// 0s
