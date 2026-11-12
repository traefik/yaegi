// The packages are initialized in the order of gc: sorted by import path,
// each one when the packages it imports are initialized. Only the packages
// with something to initialize count (an init task in gc), and a package
// waits for the real imports of the packages it imports. Package a imports
// unicode, which has init work: a waits for it. Package b imports
// unicode/utf8, which has nothing to initialize: b only waits for reg, the
// registry that a and b fill. gc runs reg, b, then a.
package main

import (
	"fmt"

	_ "github.com/traefik/yaegi/v2/vm/testdata/initreg/b"

	_ "github.com/traefik/yaegi/v2/vm/testdata/initreg/a"

	"github.com/traefik/yaegi/v2/vm/testdata/initreg/reg"
)

func main() {
	fmt.Println(reg.Names())
}
