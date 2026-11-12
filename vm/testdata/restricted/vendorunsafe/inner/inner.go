// Package inner, a package of the plugin module, imports syscall: the
// restrictions apply to every package run from source, not only the root.
package inner

import "syscall"

func Pid() int { return syscall.Getpid() }
