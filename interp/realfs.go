package interp

import (
	"io/fs"
	"os"
)

// realFS complies with the fs.FS interface (go 1.16 onwards)
// We use this rather than os.DirFS as DirFS has no concept of
// what the current working directory is, whereas this simple
// passthru to os.Open knows about working dir automagically.
type realFS struct{}

// Open complies with the fs.FS interface.
func (dir realFS) Open(name string) (fs.File, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}
