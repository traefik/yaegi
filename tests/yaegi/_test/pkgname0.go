package main

import (
	"guthib.com/bar" // pkg name is actually quux
	baz "guthib.com/baz" // pkg name is also quux, force it to baz.
)

func main() {
	println("Hello", quux.Quux())
	println("Hello", baz.Quux())
}

// GOPATH:testdata/redeclaration-global7
// Output:
// Hello bar
// Hello baz
