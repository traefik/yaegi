// A uintptr made from the address of a value that lives in cells of the VM
// (a struct with a pointer field), given to a native function that uses it
// as a number (sync/atomic): the program gets the pointer back with
// unsafe.Pointer(u), as with gc. Only a system call (syscall.Syscall) uses
// such an address as memory, and refuses a cell (testdata/syscalladdr.go).
package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

type node struct {
	n    int
	next *node
}

func main() {
	a := &node{n: 1}
	b := &node{n: 2, next: a}
	var head uintptr
	atomic.StoreUintptr(&head, uintptr(unsafe.Pointer(a)))
	fmt.Println((*node)(unsafe.Pointer(atomic.LoadUintptr(&head))).n)

	swapped := atomic.CompareAndSwapUintptr(&head, uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)))
	h := (*node)(unsafe.Pointer(atomic.LoadUintptr(&head)))
	fmt.Println(swapped, h.n, h.next.n)

	// a local variable, and the address of a field
	var local node
	local.n = 3
	old := atomic.SwapUintptr(&head, uintptr(unsafe.Pointer(&local)))
	fmt.Println(old == uintptr(unsafe.Pointer(b)), (*node)(unsafe.Pointer(atomic.LoadUintptr(&head))).n)
	var field uintptr
	atomic.StoreUintptr(&field, uintptr(unsafe.Pointer(&local.next)))
	*(**node)(unsafe.Pointer(atomic.LoadUintptr(&field))) = b
	fmt.Println(local.next.n)
}
