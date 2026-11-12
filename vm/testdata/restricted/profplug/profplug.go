// Package profplug starts the CPU profile and the execution trace, which
// are global to the process. The profiles that are a snapshot stay
// allowed.
package profplug

import (
	"io"
	"runtime/pprof"
	"runtime/trace"
)

func Run() {
	pprof.StartCPUProfile(io.Discard)
	trace.Start(io.Discard)
	pprof.WriteHeapProfile(io.Discard)
	pprof.Lookup("goroutine").WriteTo(io.Discard, 0)
}
