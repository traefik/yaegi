//go:build unix && !solaris && !illumos && !netbsd

package vm

import (
	"reflect"
	"syscall"
)

// lentFstatfs is syscall.Fstatfs for the syscall subset: only of a
// descriptor that a RawConn lends to the current goroutine (a socket or a
// file of the plugin), as Read. It writes only to the Statfs_t the plugin
// gives.
func lentFstatfs(m *Machine) reflect.Value {
	return reflect.ValueOf(func(fd int, buf *syscall.Statfs_t) error {
		if fd < 0 || !m.fdLent(uintptr(fd)) {
			panic(&RefusedError{What: "syscall.Fstatfs of a file descriptor that no RawConn callback lends to the plugin"})
		}
		return syscall.Fstatfs(fd, buf)
	})
}

// installUnix adds the checks of the functions of the syscall subset that
// take a path (unsafesyscall.go): syscall.Statfs reads only the statistics
// of the file system of the path, but the path must be readable, as for
// os.Stat.
func (c *fsCaps) installUnix(m *Machine, readOnly func(op, name string) error) {
	m.addOverride("syscall.Statfs", func(path string, buf *syscall.Statfs_t) error {
		if err := readOnly("statfs", path); err != nil {
			return err
		}
		return syscall.Statfs(path, buf)
	})
}
