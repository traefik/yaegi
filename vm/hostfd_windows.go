package vm

import (
	"os"
	"syscall"
)

// ownStdFile returns a duplicate of the handle of the host file f, with the
// same name, for the plugin (stdio.go). When f cannot be duplicated, the
// file is NUL.
func ownStdFile(f *os.File, write bool) *os.File {
	if f != nil {
		p, err := syscall.GetCurrentProcess()
		if err == nil {
			var h syscall.Handle
			if syscall.DuplicateHandle(p, syscall.Handle(f.Fd()), p, &h, 0, false, syscall.DUPLICATE_SAME_ACCESS) == nil {
				return os.NewFile(uintptr(h), f.Name())
			}
		}
	}
	return devNull(f, write)
}
