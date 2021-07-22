// +build go1.16

// fs.FS is only available from go 1.16 onwards, we have this thin wrapper so
// that it's easier for the 1.15 and below fallback code to polyfill it.
// Once 1.15 and below are no longer supported we can drop this thin wrapper
// and use fs.FS directly.

package fs

import (
	actualFs "io/fs"
	"os"
)

// FS We use a type alias to make it easier for the pre-go1.16
// code to fullfil this local type.
type FS = actualFs.FS

// RealFS complies with the fs.FS interface.
// We use this rather than os.DirFS as DirFS has no concept of
// what the current working directory is, whereas this simple
// passthru to os.Open knows about working dir automagically.
type RealFS struct{}

// Open complies with the fs.FS interface.
func (dir RealFS) Open(name string) (actualFs.File, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

var (
	// ReadDir is an alias to the real implementation. Once the need for backwards compat goes away, so can this.
	ReadDir = actualFs.ReadDir
	// Stat is an alias to the real implementation. Once the need for backwards compat goes away, so can this.
	Stat = actualFs.Stat
	// ReadFile is an alias to the real implementation. Once the need for backwards compat goes away, so can this.
	ReadFile = actualFs.ReadFile
)
