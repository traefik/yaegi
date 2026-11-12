// The address of a variable given to a system call as a uintptr, as the
// rule 4 of unsafe.Pointer allows (syscall.Syscall(..., uintptr(
// unsafe.Pointer(&ts)), ...)). The kernel writes the variable with gc's
// layout: the VM must give it real memory, not the address of a cell
// (before, the kernel wrote over a cell and the VM stopped with a fault).
// Linux only (onlyGOOS in cmd/runtests).
package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// timespec has the layout of syscall.Timespec on linux/amd64 and arm64,
// but it is a type of the program.
type timespec struct {
	sec  int64
	nsec int64
}

type pair struct {
	tag uint32
	ts  timespec
}

func main() {
	b := []byte("hi from write\n")
	n, _, e := syscall.Syscall(syscall.SYS_WRITE, 1, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	fmt.Println(n, e == 0)

	// a variable of a native struct type
	var ts syscall.Timespec
	_, _, e = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&ts)), 0)
	fmt.Println(e == 0, ts.Sec > 0, ts.Nsec >= 0 && ts.Nsec < 1e9)
	p := &ts.Sec
	sec := ts.Sec
	_, _, e = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&ts)), 0)
	fmt.Println(e == 0, *p >= sec, *p == ts.Sec)
	fmt.Println(unsafe.Pointer(&ts) == unsafe.Pointer(&ts.Sec))

	// a variable of a struct type of the program
	var my timespec
	_, _, e = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&my)), 0)
	fmt.Println(e == 0, my.sec > 0, my.sec >= sec, my.nsec >= 0 && my.nsec < 1e9)

	// the address of a field
	var pr pair
	pr.tag = 7
	_, _, e = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&pr.ts)), 0)
	fmt.Println(e == 0, pr.tag, pr.ts.sec >= sec)

	// an array the kernel fills: the two ends of a pipe
	var fds [2]int32
	_, _, e = syscall.RawSyscall(syscall.SYS_PIPE2, uintptr(unsafe.Pointer(&fds)), 0, 0)
	fmt.Println(e == 0, fds[0] > 2, fds[1] > 2, fds[0] != fds[1])
	msg := []byte("through the pipe")
	syscall.Write(int(fds[1]), msg)
	got := make([]byte, 64)
	k, _ := syscall.Read(int(fds[0]), got)
	fmt.Println(string(got[:k]))
	syscall.Close(int(fds[0]))
	syscall.Close(int(fds[1]))

	// a plain integer
	var x int64
	_, _, e = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&x)), 0)
	fmt.Println(e == 0, x >= sec)
}
