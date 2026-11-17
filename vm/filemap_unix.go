//go:build unix

package vm

import (
	"fmt"
	"reflect"
	"syscall"
	"unsafe"
)

// fileMapFuncs adds to funcs, the golang.org/x/sys/unix subset of a
// restricted machine (xsysSubset), the calls of filemap.go on the files and
// the mappings of the plugin. all is the table of the host.
func (m *Machine) fileMapFuncs(all *Package, funcs map[string]reflect.Value) {
	if flock, ok := funcOf[func(fd, how int) error](all, "Flock"); ok {
		funcs["Flock"] = reflect.ValueOf(func(fd, how int) error {
			if !m.ownFD(fd) {
				panic(&RefusedError{What: "unix.Flock of a file descriptor that is not a file the plugin opened"})
			}
			return flock(fd, how)
		})
	}
	if mmap, ok := funcOf[func(fd int, offset int64, length, prot, flags int) ([]byte, error)](all, "Mmap"); ok {
		funcs["Mmap"] = reflect.ValueOf(func(fd int, offset int64, length, prot, flags int) ([]byte, error) {
			m.checkMmap(fd, prot, flags)
			b, err := mmap(fd, offset, length, prot, flags)
			if err == nil {
				m.addMapping(b)
			}
			return b, err
		})
	}
	if munmap, ok := funcOf[func([]byte) error](all, "Munmap"); ok {
		funcs["Munmap"] = reflect.ValueOf(func(b []byte) error {
			m.fmaps.mu.Lock()
			defer m.fmaps.mu.Unlock()
			if len(b) == 0 || m.mappingLocked(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b))) == nil {
				panic(&RefusedError{What: "unix.Munmap of memory that is not a mapping the plugin made"})
			}
			err := munmap(b)
			if err == nil {
				m.setMappingLocked(uintptr(unsafe.Pointer(&b[0])), nil)
			}
			return err
		})
	}
	for _, name := range []string{"Madvise", "Msync"} {
		fn, ok := funcOf[func([]byte, int) error](all, name)
		if !ok {
			continue
		}
		funcs[name] = reflect.ValueOf(func(b []byte, flags int) error {
			if len(b) > 0 && !m.inMapping(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b))) {
				panic(&RefusedError{What: "unix." + name + " of memory that is not a mapping the plugin made"})
			}
			return fn(b, flags)
		})
	}
	if _, ok := funcOf[func(trap, a1, a2, a3 uintptr) (uintptr, uintptr, syscall.Errno)](all, "Syscall"); ok {
		funcs["Syscall"] = reflect.ValueOf(func(trap, a1, a2, a3 uintptr) (uintptr, uintptr, syscall.Errno) {
			return m.mapSyscall("unix.Syscall", trap, a1, a2, a3, 0, 0, 0)
		})
	}
	if _, ok := funcOf[func(trap, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, uintptr, syscall.Errno)](all, "Syscall6"); ok {
		funcs["Syscall6"] = reflect.ValueOf(func(trap, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, uintptr, syscall.Errno) {
			return m.mapSyscall("unix.Syscall6", trap, a1, a2, a3, a4, a5, a6)
		})
	}
}

// checkMmap refuses an Mmap that is not a mapping of a file of the plugin,
// readable or writable, shared or private.
func (m *Machine) checkMmap(fd, prot, flags int) {
	if !m.ownFD(fd) {
		panic(&RefusedError{What: "unix.Mmap of a file descriptor that is not a file the plugin opened"})
	}
	if prot&^(syscall.PROT_READ|syscall.PROT_WRITE) != 0 {
		panic(&RefusedError{What: fmt.Sprintf("unix.Mmap with the protection %#x (only PROT_READ and PROT_WRITE)", prot)})
	}
	if flags&^(syscall.MAP_SHARED|syscall.MAP_PRIVATE) != 0 {
		panic(&RefusedError{What: fmt.Sprintf("unix.Mmap with the flags %#x (only MAP_SHARED and MAP_PRIVATE)", flags)})
	}
}

// mapSyscall is unix.Syscall and unix.Syscall6 of the subset: munmap of a
// mapping of the plugin, and mremap of one on linux (sysMremap).
func (m *Machine) mapSyscall(what string, trap, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, uintptr, syscall.Errno) {
	switch {
	case trap == syscall.SYS_MUNMAP:
		m.fmaps.mu.Lock()
		defer m.fmaps.mu.Unlock()
		if m.mappingLocked(a1, a2) == nil {
			panic(&RefusedError{What: what + " munmap of memory that is not a mapping the plugin made"})
		}
		r1, r2, errno := syscall.Syscall(syscall.SYS_MUNMAP, a1, a2, 0)
		if errno == 0 {
			m.setMappingLocked(a1, nil)
		}
		return r1, r2, errno
	case sysMremap != 0 && trap == sysMremap:
		if a4 != mremapMayMove || a5 != 0 {
			panic(&RefusedError{What: fmt.Sprintf("%s mremap with the flags %#x (only MREMAP_MAYMOVE)", what, a4)})
		}
		m.fmaps.mu.Lock()
		defer m.fmaps.mu.Unlock()
		if m.mappingLocked(a1, a2) == nil {
			panic(&RefusedError{What: what + " mremap of memory that is not a mapping the plugin made"})
		}
		r1, r2, errno := syscall.Syscall6(sysMremap, a1, a2, a3, mremapMayMove, 0, 0)
		if errno == 0 {
			m.setMappingLocked(a1, nil)
			if a3 > 0 {
				// memory of the kernel, outside the Go heap
				p := *(*unsafe.Pointer)(unsafe.Pointer(&r1))
				m.setMappingLocked(r1, unsafe.Slice((*byte)(p), a3))
			}
		}
		return r1, r2, errno
	}
	panic(&RefusedError{What: fmt.Sprintf("%s of the system call %d (only munmap and mremap of a mapping the plugin made)", what, trap)})
}
