//go:build darwin || freebsd || netbsd || openbsd

package vm

import "golang.org/x/sys/unix"

// dupTo makes the descriptor to a copy of from (dup2).
func dupTo(from, to int) error { return unix.Dup2(from, to) }
