// Package z has no imports and only static data: gc has nothing to run for
// it (no init task), and the packages that import it do not wait for it
// (testdata/initleaf.go).
package z

// T is a struct type.
type T struct{ N int }

const Z = 1

// L, S and P are static data in gc.
var (
	L = []string{"x", "y"}
	S = T{N: 2}
	P = &S
	F = func() int { return 3 }
)
