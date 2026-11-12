package linkrulesplug

import "unsafe"

// memmove has two parameters where runtime.memmove has three: the VM does
// not give its helper for it.
//
//go:linkname memmove runtime.memmove
func memmove(dst, src unsafe.Pointer)

// Move calls the function: refused at load time, a stub with
// Options.StubAssembly.
func Move() {
	var a, b [2]byte
	memmove(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]))
}
