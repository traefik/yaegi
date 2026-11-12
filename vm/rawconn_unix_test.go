//go:build unix

package vm

import "syscall"

// peekWaitall are the flags of the refused Recvfrom of TestRawConnSubset.
const peekWaitall = syscall.MSG_PEEK | syscall.MSG_WAITALL
