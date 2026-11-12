//go:build wasm

package vm

import "runtime"

// getg returns a key for the current goroutine: its id, read from the
// header of runtime.Stack ("goroutine 18 [running]:"). wasm has no
// assembly version (getg_*.s). This one is much slower: runtime.Stack walks
// the whole Go stack, so a deep recursion through native callbacks takes
// quadratic time before it stops (but it does stop).
func getg() uintptr {
	var buf [32]byte
	b := buf[:runtime.Stack(buf[:], false)]
	const prefix = len("goroutine ")
	var id uintptr
	for i := prefix; i < len(b) && b[i] >= '0' && b[i] <= '9'; i++ {
		id = id*10 + uintptr(b[i]-'0')
	}
	return id
}
