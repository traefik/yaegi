package vm

import (
	"errors"
	"math/bits"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
)

// The guard of the twin reflect against values nested very deep.
//
// AOT copies (yaml, toml, xml, gob, asn1, go-cmp, cast, slog, templates,
// fmt called by another AOT copy...) walk a value through the twin reflect
// and recurse once per level on the host stack. A plugin value nested about
// 1M times in interfaces made them reach the stack limit of the goroutine:
// a fatal error that ends the host, which nothing can recover. gc ends the
// same way, but there it is the program's own process. The walkers of fmt
// and json are bounded before the walk (deepprint.go); for the others,
// RV.Elem checks how much of the goroutine stack is used, and past
// stackGuardMax it panics with errStackTooDeep: the plugin gets a panic it
// can recover, and the host goes on (fail closed).

// errStackTooDeep is the panic of RV.Elem when the goroutine stack is
// almost full (see stackGuardMax).
var errStackTooDeep = errors.New("vm: a value is nested too deep: the walk of it almost filled the goroutine stack")

// stackGuardMax is the stack use, in bytes, past which RV.Elem panics: the
// largest stack that gc allows, less 1/128 for the deferred calls that run
// during the panic. gc doubles a stack when it grows and stops past the
// limit of runtime/debug.SetMaxStack: 1e9 bytes on 64-bit platforms (250e6
// on 32-bit) unless the host changed it, so the largest stack is 512 MiB
// (128 MiB on 32-bit), and the guard is at 508 MiB (127 MiB). It is near
// the limit on purpose: a walk that ends below the limit must not fail
// (the AOT copy of xml uses about 500 MiB for a value nested 1M times).
// setStackGuard reads the limit of the host at start-up and at each load
// (the host sets it in its main, after the start-up of this package).
var (
	stackGuardMax atomic.Uintptr
	stackGuardMu  sync.Mutex
)

// defaultMaxStack is the limit of runtime/debug.SetMaxStack that the gc
// runtime starts with; the runtime never grows a stack past twice it.
const defaultMaxStack = 1000000000 >> (2 * (1 - strconv.IntSize/64))

// setStackGuard sets stackGuardMax from the limit of the host
// (runtime/debug.SetMaxStack). The read sets the limit twice: to twice the
// default for a moment, which the runtime caps anyway, then back (under a
// lock: two reads at the same time would leave the wrong limit).
func setStackGuard() {
	stackGuardMu.Lock()
	defer stackGuardMu.Unlock()
	limit := debug.SetMaxStack(2 * defaultMaxStack)
	debug.SetMaxStack(limit)
	limit = min(max(limit, 1), 2*defaultMaxStack)
	largest := uintptr(1) << (bits.Len(uint(limit)) - 1) // the largest stack: a power of two
	stackGuardMax.Store(largest / 128 * 127)
}

// stackGuardOK is false when the stack bounds read from the goroutine
// structure look wrong (a new layout in a Go release): then the guard is off.
var stackGuardOK = stackBoundsOK()

func init() { setStackGuard() }

// checkStack panics with errStackTooDeep when the current goroutine uses
// more than stackGuardMax bytes of stack.
func checkStack() {
	if stackGuardOK && stackUsed() > stackGuardMax.Load() {
		panic(errStackTooDeep)
	}
}
