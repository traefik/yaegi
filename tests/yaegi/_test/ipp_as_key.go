package main

import (
	"guthib.com/toto" // pkg name is actually titi
	"guthib.com/tata" // pkg name is actually tutu
)

func main() {
	println("Hello", titi.Quux())
	println("Hello", tutu.Quux())
}

// GOPATH:testdata/redeclaration-global7
// Output:
// Hello bar
// Hello baz
