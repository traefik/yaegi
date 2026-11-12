package main

import (
	"guthib.com/bar" // pkg name is actually quux
)

func main() {
	println("Hello", bar.Quux()) // bar should not be a known symbol.
}

// GOPATH:testdata/redeclaration-global7
// Error:
// ../_test/pkgname1.go:8:19: undefined: bar
