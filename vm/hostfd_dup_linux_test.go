package vm

import "golang.org/x/sys/unix"

// dupTo makes the descriptor to a copy of from (dup2).
func dupTo(from, to int) error { return unix.Dup3(from, to, 0) }
