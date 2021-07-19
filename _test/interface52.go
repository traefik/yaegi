package main

import "testing"

func main() {
	t := testing.T{}
	var tb testing.TB
	tb = &t
	if tb.TempDir() == "" {
		println("FAIL")
		return
	}
	println("PASS")
}

// Output:
// PASS
