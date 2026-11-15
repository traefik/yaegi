// Package hostmemunsafeplug, loaded with UseUnsafe, writes to the bytes of
// a host []byte variable through the paths that unsafe opens: a plain-data
// view and runtime.memmove. Each write reports "refused" or "done", as
// hostmemplug does; the same writes to its own []byte are "done".
package hostmemunsafeplug

import (
	"fmt"
	"io"
	"net"
	"strings"
	"unsafe"
)

//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)

func try(w io.Writer, name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok && strings.Contains(err.Error(), "memory of the host") {
				fmt.Fprintf(w, "%s: refused\n", name)
				return
			}
			fmt.Fprintf(w, "%s: panic %v\n", name, r)
		}
	}()
	f()
	fmt.Fprintf(w, "%s: done\n", name)
}

// Run writes the results to w.
func Run(w io.Writer) {
	host := net.IPv4bcast
	own := []byte{1, 2, 3, 4}
	try(w, "view store", func() { *(*uint32)(unsafe.Pointer(&host[0])) = 0 })
	try(w, "view store own", func() { *(*uint32)(unsafe.Pointer(&own[0])) = 0 })
	// A read through a view of the host bytes stays allowed.
	try(w, "view load", func() { own[0] = byte(*(*uint32)(unsafe.Pointer(&host[0]))) })
	try(w, "memmove", func() { memmove(unsafe.Pointer(&host[0]), unsafe.Pointer(&own[0]), 4) })
	try(w, "memmove own", func() { memmove(unsafe.Pointer(&own[0]), unsafe.Pointer(&host[0]), 4) })
}
