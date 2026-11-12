//go:build !unix

package vm

// peekWaitall: package syscall has no MSG_* outside unix, where
// TestRawConnSubset is skipped.
const peekWaitall = 0
