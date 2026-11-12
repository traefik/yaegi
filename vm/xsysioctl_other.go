//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package vm

// xsysIoctls: no ioctl in the golang.org/x/sys/unix subset of the other
// systems (xsysunix.go).
var xsysIoctls map[string]uint64
