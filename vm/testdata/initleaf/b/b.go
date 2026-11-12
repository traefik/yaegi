// Package b imports a package that has no init work and no imports
// (testdata/initleaf.go).
package b

import "github.com/traefik/yaegi/v2/vm/testdata/initleaf/z"

func init() { println("init b", z.Z, len(z.L)) }
