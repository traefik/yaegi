package main

import (
	"guthib.com/bar" // pkg name is actually quux
	"guthib.com/baz" // pkg name is also quux
)

func main() {
	println("Hello", quux.Quux())
}

// GOPATH:testdata/redeclaration-global7
// Error:
// ../_test/redeclaration-global7.go:5:2: quux/redeclaration-global7.go redeclared as imported package name
