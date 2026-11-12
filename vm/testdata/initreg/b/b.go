// Package b imports unicode/utf8, which has nothing to initialize in gc: b
// only waits for reg (testdata/initregistry.go).
package b

import (
	"unicode/utf8"

	"github.com/traefik/yaegi/v2/vm/testdata/initreg/reg"
)

func init() {
	println("init b")
	reg.Add("b" + string(rune('0'+utf8.RuneLen('b'))))
}
