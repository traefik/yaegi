package vm

import "syscall"

// xsysIoctls are the read-only requests of the ioctl functions of the
// golang.org/x/sys/unix subset (xsysunix.go): the terminal attributes and
// the window size.
var xsysIoctls = map[string]uint64{
	"IoctlGetTermios": syscall.TCGETS,
	"IoctlGetWinsize": syscall.TIOCGWINSZ,
}
