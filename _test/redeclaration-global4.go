package main

import (
	"time"
)

type time string

func main() {
	var t time = "hello"
	println(t)
}

// Error:
// ../_test/redeclaration-global4.go:7:6: time redeclared in this block
