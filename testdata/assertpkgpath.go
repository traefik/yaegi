// A failed type assertion between two struct types without a name that
// print the same says "types from different packages" when they come from
// two packages (an unexported field belongs to its package), as gc does
// (GOROOT test fixedbugs/issue18911.go).
package main

import (
	"fmt"

	"github.com/traefik/yaegi/v2/vm/testdata/assertpkg"
)

func try(f func()) {
	defer func() { fmt.Println(recover()) }()
	f()
}

func main() {
	try(func() { _ = assertpkg.X.(struct{ x int }) })
	try(func() {
		_ = assertpkg.Z.(struct {
			A int
			b string
		})
	})
	// a pointer to the struct type, or a slice of it: gc keeps no package
	// in these types ("different scopes")
	try(func() { _ = assertpkg.PX.(*struct{ x int }) })
	try(func() { _ = assertpkg.S.([]struct{ x int }) })
	// same package: the types differ only by their scope
	try(func() { _ = local().(T) })
	// only exported fields: the same type in both packages
	v, ok := assertpkg.Y.(struct{ X int })
	fmt.Println(v, ok)
}

type T struct{ x int }

func local() any {
	type T struct{ x int }
	return T{}
}
