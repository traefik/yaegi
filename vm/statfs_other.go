//go:build !unix || solaris || illumos || netbsd

package vm

import "reflect"

// lentFstatfs and installUnix: package syscall has no Statfs_t on windows,
// solaris, illumos and netbsd (Statvfs), so the subset has no Fstatfs and no Statfs there.
func lentFstatfs(m *Machine) reflect.Value { return reflect.Value{} }

func (c *fsCaps) installUnix(m *Machine, readOnly func(op, name string) error) {}
