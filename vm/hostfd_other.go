//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package vm

import "os"

// On these systems, a restricted plugin opens files as Go does: the checks
// of hostfd_walk.go are written for the unix systems that have a directory
// of descriptors (/dev/fd, /proc/self/fd) and are tested there.

func openNoAlias(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func checkRoot(*os.Root) error { return nil }
