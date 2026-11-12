package main

import (
	"guthib.com/toto" // pkg name is actually titi
)

func main() {
	println("Hello", titi.Quux())
}

// GOPATH:testdata/redeclaration-global7
// Output:
// Hello bar
