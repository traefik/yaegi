package vm

import "syscall"

// sysMremap is the mremap system call of linux, and mremapMayMove its flag
// MREMAP_MAYMOVE (filemap_unix.go).
const (
	sysMremap     = syscall.SYS_MREMAP
	mremapMayMove = 1
)
