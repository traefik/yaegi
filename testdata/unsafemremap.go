//go:build linux

// A mapped file grown with mremap, as ristretto (the cache of badger) does on
// linux: the Data of the slice header becomes the new address.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"unsafe"

	"golang.org/x/sys/unix"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func mremap(data []byte, size int) ([]byte, error) {
	const MREMAP_MAYMOVE = 0x1
	header := (*reflect.SliceHeader)(unsafe.Pointer(&data))
	addr, _, errno := unix.Syscall6(unix.SYS_MREMAP, header.Data, uintptr(header.Len), uintptr(size), uintptr(MREMAP_MAYMOVE), 0, 0)
	if errno != 0 {
		return nil, errno
	}
	header.Data = addr
	header.Cap = size
	header.Len = size
	return data, nil
}

func main() {
	dir, err := os.MkdirTemp("", "unsafemremap")
	check(err)
	defer os.RemoveAll(dir)
	f, err := os.OpenFile(filepath.Join(dir, "data"), os.O_RDWR|os.O_CREATE, 0o600)
	check(err)
	const size = 1 << 16
	check(f.Truncate(size))
	b, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	check(err)
	copy(b, "hello")
	b[size-1] = 1

	for i := 2; i <= 4; i++ {
		check(f.Truncate(int64(i * size)))
		b, err = mremap(b, i*size)
		check(err)
		b[i*size-1] = byte(i)
	}
	fmt.Println(len(b), cap(b), string(b[:5]), b[size-1], b[2*size-1], b[4*size-1])

	_, _, errno := unix.Syscall(unix.SYS_MUNMAP, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
	if errno != 0 {
		panic(errno)
	}
	check(f.Close())
	data, err := os.ReadFile(filepath.Join(dir, "data"))
	check(err)
	fmt.Println(len(data), string(data[:5]), data[3*size-1], data[4*size-1])
}
