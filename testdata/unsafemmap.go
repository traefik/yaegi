//go:build linux || darwin

// A file mapped in memory, as badger (with ristretto) and nutsdb do: the
// directory is locked with flock, the file is mapped with mmap, plain data
// with sync/atomic fields is laid over the mapped bytes, and the mapping is
// synced, and unmapped with a raw system call.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

type header struct {
	count atomic.Uint32
	flags uint32
	size  atomic.Uint64
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func munmap(b []byte) error {
	_, _, errno := unix.Syscall(unix.SYS_MUNMAP, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func main() {
	dir, err := os.MkdirTemp("", "unsafemmap")
	check(err)
	defer os.RemoveAll(dir)

	// The lock of the directory, as badger's dirLock.
	d, err := os.Open(dir)
	check(err)
	check(unix.Flock(int(d.Fd()), unix.LOCK_EX|unix.LOCK_NB))

	// A second lock of the same directory fails, as for a second process.
	d2, err := os.Open(dir)
	check(err)
	fmt.Println("second lock:", unix.Flock(int(d2.Fd()), unix.LOCK_EX|unix.LOCK_NB) == unix.EWOULDBLOCK)
	check(d2.Close())

	f, err := os.OpenFile(filepath.Join(dir, "data"), os.O_RDWR|os.O_CREATE, 0o600)
	check(err)
	const size = 1 << 16
	check(f.Truncate(size))
	b, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	check(err)
	check(unix.Madvise(b, unix.MADV_RANDOM))

	h := (*header)(unsafe.Pointer(&b[0]))
	h.count.Add(3)
	h.flags = 7
	h.size.Store(size)
	copy(b[64:], "hello")
	check(unix.Msync(b, unix.MS_SYNC))

	// The bytes are in the file.
	data, err := os.ReadFile(filepath.Join(dir, "data"))
	check(err)
	fmt.Println(data[0], data[4], string(data[64:69]))

	// Grow the file and the mapping, with munmap and mmap again (mremap on
	// linux: unsafemremap.go).
	check(f.Truncate(2 * size))
	check(munmap(b))
	b, err = unix.Mmap(int(f.Fd()), 0, 2*size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	check(err)
	h = (*header)(unsafe.Pointer(&b[0]))
	h.count.Add(1)
	b[size+1] = 9
	fmt.Println(len(b), h.count.Load(), h.flags, h.size.Load(), string(b[64:69]), b[size+1])

	check(munmap(b))
	check(f.Close())
	check(unix.Flock(int(d.Fd()), unix.LOCK_UN))
	check(d.Close())

	data, err = os.ReadFile(filepath.Join(dir, "data"))
	check(err)
	fmt.Println(len(data), data[0], data[size+1])
}
