//go:build freebsd || netbsd || openbsd

package vm

// createKeepsSticky is false: open drops the sticky bit of a new file, as
// os/sticky_bsd.go says. maxLinks is the number of links that openNoAlias
// follows in the last element, the MAXSYMLINKS of sys/param.h.
const (
	searchFlag        = 0
	openExtra         = 0
	createKeepsSticky = false
	maxLinks          = 32
)

func isProcFS(int) bool { return false }
