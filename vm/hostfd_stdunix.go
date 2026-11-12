//go:build unix && !(linux || darwin || freebsd || netbsd || openbsd)

package vm

import (
	"os"
	"syscall"
)

// ownStdFile returns a duplicate of the host file f for the plugin
// (stdio.go), with syscall.Dup on the other unix systems. When f cannot be
// duplicated, the file is /dev/null.
func ownStdFile(f *os.File, write bool) *os.File {
	if f != nil {
		if rc, err := f.SyscallConn(); err == nil {
			nfd := -1
			rc.Control(func(fd uintptr) {
				syscall.ForkLock.RLock()
				defer syscall.ForkLock.RUnlock()
				if n, err := syscall.Dup(int(fd)); err == nil {
					syscall.CloseOnExec(n)
					nfd = n
				}
			})
			if nfd >= 0 {
				return os.NewFile(uintptr(nfd), f.Name())
			}
		}
	}
	return devNull(f, write)
}
