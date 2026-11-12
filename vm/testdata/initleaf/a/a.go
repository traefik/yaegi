// Package a imports unicode/utf8, which has nothing to initialize in gc
// (no init task): a does not wait for it (testdata/initleaf.go).
package a

import "unicode/utf8"

var N = utf8.RuneLen('a')

func init() { println("init a", N) }
