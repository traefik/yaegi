//go:build !wasm

package vm

import "unsafe"

// stackBounds returns the bounds [lo, hi) of the stack of the current
// goroutine: the first field of the runtime structure of a goroutine
// (getg) is its stack, two words lo and hi. This has not changed since Go
// 1.4; stackBoundsOK checks it at start-up.
func stackBounds() (lo, hi uintptr) {
	g := getg()
	p := *(*unsafe.Pointer)(unsafe.Pointer(&g))
	s := (*[2]uintptr)(p)
	return s[0], s[1]
}

// stackUsed returns the bytes of stack that the current goroutine uses, or
// 0 when the bounds are not those of the stack of a local variable (the
// guard is then off for this call, never wrong).
func stackUsed() uintptr {
	var x byte
	lo, hi := stackBounds()
	sp := uintptr(unsafe.Pointer(&x))
	if sp < lo || sp >= hi {
		return 0
	}
	return hi - sp
}

// stackBoundsOK reports whether stackBounds looks right: a local variable
// is in the bounds, and the stack is not too large.
func stackBoundsOK() bool {
	var x byte
	lo, hi := stackBounds()
	sp := uintptr(unsafe.Pointer(&x))
	return lo < hi && lo <= sp && sp < hi && hi-lo <= 1<<30
}
