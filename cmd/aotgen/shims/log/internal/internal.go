// Package internal is the aotgen shim of log/internal. In the standard
// library, package log sets DefaultOutput in its init function. The copy of
// log/internal is another package that the native log never sets, so the
// default slog handler of the log/slog copy called a nil function. This
// version writes to the default log.Logger through its public API.
package internal

import (
	"log"
	"runtime"
)

// DefaultOutput writes data with the default log.Logger, as the function that
// package log puts here in the standard library. pc is the caller of the slog
// function: it gives the file and line when the logger flags ask for them.
var DefaultOutput = func(pc uintptr, data []byte) error {
	return log.Output(callDepth(pc), string(data))
}

// callDepth returns the calldepth of log.Output that reports the frame of pc
// (1 is the caller of log.Output: DefaultOutput). The log package has no
// public API that takes a pc, so pc is looked for on the current stack: the
// frame of the slog call is still there when the default handler runs. When
// pc is 0, or not on the stack (a Record handled later or on another
// goroutine), the line is this function's; Go prints the file and line of
// pc itself.
func callDepth(pc uintptr) int {
	if pc == 0 || log.Flags()&(log.Lshortfile|log.Llongfile) == 0 {
		return 1
	}
	var pcs [64]uintptr
	// pcs[0] is DefaultOutput (skip runtime.Callers and callDepth).
	n := runtime.Callers(2, pcs[:])
	for i, p := range pcs[:n] {
		if p == pc {
			return i + 1
		}
	}
	return 1
}
