// Package profplug starts the CPU profile and the execution trace, which
// are global to the process, sets the rates of the profiler and the
// GOMAXPROCS of the process. The profiles that are a snapshot stay
// allowed.
package profplug

import (
	"io"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

func Run() {
	pprof.StartCPUProfile(io.Discard)
	trace.Start(io.Discard)
	pprof.WriteHeapProfile(io.Discard)
	pprof.Lookup("goroutine").WriteTo(io.Discard, 0)
}

func Settings() {
	runtime.SetDefaultGOMAXPROCS()
	runtime.SetCPUProfileRate(0)
	runtime.SetBlockProfileRate(0)
	runtime.SetMutexProfileFraction(0)
	trace.NewFlightRecorder(trace.FlightRecorderConfig{}).Start()
	var fr trace.FlightRecorder // a zero FlightRecorder starts too
	fr.Start()
	runtime.StartTrace()
	runtime.ReadTrace()
	runtime.StopTrace()
	trace.Stop()
	pprof.StopCPUProfile()
}
