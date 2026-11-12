// Package a imports unicode, which has init work in gc (an init task): a
// waits for it (testdata/initregistry.go).
package a

import (
	"unicode"

	"github.com/traefik/yaegi/v2/vm/testdata/initreg/reg"
)

func init() {
	println("init a")
	reg.Add(string(unicode.ToUpper('a')))
}
