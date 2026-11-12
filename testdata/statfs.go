package main

// syscall.Statfs is in the read-only syscall subset of a restricted plugin
// with useUnsafe: prometheus/procfs checks with it that
// /proc is a procfs. It gives the statistics of the file system of a path,
// as os.Stat gives the metadata of a file.

import (
	"fmt"
	"os"
	"syscall"
)

func main() {
	var st syscall.Statfs_t
	err := syscall.Statfs(os.TempDir(), &st)
	// The fields differ per system (openbsd has F_bsize and no Type):
	// only check that the call filled the struct.
	fmt.Println("statfs:", err, st != syscall.Statfs_t{})
	var st2 syscall.Statfs_t
	err = syscall.Statfs(os.TempDir(), &st2)
	fmt.Println("again:", err, st2 != syscall.Statfs_t{})
	err = syscall.Statfs("/no/such/dir", &st)
	fmt.Println("missing:", err == syscall.ENOENT)
}
