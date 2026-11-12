//go:build !wasm

package vm

// getg returns the address of the runtime structure of the current
// goroutine (getg_*.s). It is mostly a key; the VM only reads the stack
// bounds through it (stackguard_g.go).
// The runtime reuses the structure of a goroutine that has ended, so a key
// is only valid while its goroutine runs interpreted code (see enterG).
func getg() uintptr
