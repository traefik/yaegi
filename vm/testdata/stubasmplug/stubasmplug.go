// Package stubasmplug reaches functions without a Go body that the VM
// cannot run, as golang.org/x/sys/unix on darwin (a system call through the
// C library) and wazero (assembly of its compiler) do: refused at load time
// by default, loaded with Options.StubAssembly.
package stubasmplug

import (
	"syscall"
	_ "unsafe" // go:linkname
)

//go:linkname syscall_syscall6 syscall.syscall6
func syscall_syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)

// addAsm is written in assembly in a real package.
func addAsm(a, b int) int

// Probe makes a system call through the C library, as x/sys/cpu does in its
// init on darwin.
func Probe() bool {
	r1, _, e := syscall_syscall6(0, 0, 0, 0, 0, 0, 0)
	return e == syscall.ENOSYS && r1 == ^uintptr(0)
}

// Add calls the assembly function.
func Add() int { return addAsm(1, 2) }

// Value returns the assembly function as a value, without calling it.
func Value() bool {
	f := addAsm
	return f != nil
}
