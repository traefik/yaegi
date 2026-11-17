//go:build unix && !linux

package vm

// sysMremap: only linux has mremap (filemap_unix.go).
const (
	sysMremap     = 0
	mremapMayMove = 1
)
