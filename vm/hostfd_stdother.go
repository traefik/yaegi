//go:build !unix && !windows

package vm

import "os"

// ownStdFile returns the host file: these systems have no dup.
func ownStdFile(f *os.File, write bool) *os.File { return f }
