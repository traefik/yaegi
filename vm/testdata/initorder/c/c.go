// Package c has no imports (testdata/initorder.go).
package c

var C = "c"

func init() { println("init", C) }
