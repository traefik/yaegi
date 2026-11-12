package vm

import "golang.org/x/sys/unix"

// searchFlag opens a directory for openat without the right to read it.
// openExtra is the flag that os.OpenFile adds (large files on 32-bit
// systems). createKeepsSticky: open keeps the sticky bit of a new file.
// maxLinks is the number of links that openNoAlias follows in the last
// element, the MAXSYMLINKS of the kernel.
const (
	searchFlag        = unix.O_PATH
	openExtra         = unix.O_LARGEFILE
	createKeepsSticky = true
	maxLinks          = 40
)

// isProcFS reports whether the directory fd is in a procfs.
func isProcFS(fd int) bool {
	var s unix.Statfs_t
	err := ignoringEINTR(func() error { return unix.Fstatfs(fd, &s) })
	return err == nil && s.Type == unix.PROC_SUPER_MAGIC
}
