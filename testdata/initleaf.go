// The packages are initialized in the order of gc: sorted by import path,
// each one when the packages it imports are initialized. Only the packages
// that have something to initialize count (an init task in gc: init
// functions, variables that are not static data, or an import that has an
// init task). Package a imports unicode/utf8 and package b imports z, which
// have nothing to initialize: a and b do not wait for them, and gc runs a,
// b, then c.
package main

import (
	_ "github.com/traefik/yaegi/v2/vm/testdata/initleaf/c"

	_ "github.com/traefik/yaegi/v2/vm/testdata/initleaf/b"

	"github.com/traefik/yaegi/v2/vm/testdata/initleaf/a"

	"github.com/traefik/yaegi/v2/vm/testdata/initleaf/z"
)

func main() {
	println("main", a.N, z.L[1], z.S.N, z.P.N, z.F())
}
