//go:build wasm

package vm

// wasm has no getg that gives the runtime structure (getg_other.go): the
// guard of stackguard.go is off.

func stackUsed() uintptr  { return 0 }
func stackBoundsOK() bool { return false }
