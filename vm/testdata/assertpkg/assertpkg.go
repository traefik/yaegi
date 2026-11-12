// Package assertpkg holds values of struct types without a name, for
// testdata/assertpkgpath.go.
package assertpkg

// X has an unexported field: its type is not the struct{ x int } of
// another package.
var X any = struct{ x int }{}

// Y has only exported fields: its type is the struct{ X int } of every
// package.
var Y any = struct{ X int }{}

// PX and S hold the type of X behind a pointer and in a slice: these types
// have no package of their own.
var (
	PX any = &struct{ x int }{}
	S  any = []struct{ x int }{}
)

// Z has an exported field first: the package is the one of the first
// unexported field.
var Z any = struct {
	A int
	b string
}{}
