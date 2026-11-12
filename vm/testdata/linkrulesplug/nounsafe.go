// Package linkrulesplug links functions to the runtime in ways the VM does
// not accept: a //go:linkname in a file that does not import "unsafe"
// (gc refuses to compile it), and a runtime helper declared with another
// signature than the one of the runtime.
package linkrulesplug

// This file does not import "unsafe": the directive does not count.

//go:linkname nanotime runtime.nanotime
func nanotime() int64

// Nano calls the function: refused at load time, a stub with
// Options.StubAssembly.
func Nano() int64 { return nanotime() }
