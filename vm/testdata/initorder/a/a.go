// Package a imports a package of the standard library whose path sorts
// after the paths of the other packages of the test: it is initialized
// after them (testdata/initorder.go).
package a

import "strings"

var A = strings.ToUpper("a")

func init() { println("init", A) }
