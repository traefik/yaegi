//go:build unix

package vm

import "syscall"

// openNoBlock is the open flag that makes the open of a FIFO return without
// a writer (see openRegular).
const openNoBlock = syscall.O_NONBLOCK
