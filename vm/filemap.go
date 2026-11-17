package vm

// The files and the memory mappings of a restricted plugin with
// Options.UseUnsafe.
//
// Embedded databases (badger, nutsdb) lock their directory with flock and
// map their files with mmap. A restricted plugin with UseUnsafe gets these
// calls of golang.org/x/sys/unix (xsysunix.go) on its own files and its own
// mappings only (filemap_unix.go):
//
//   - Flock: only on a file that the plugin opened (os.Open, os.OpenFile,
//     os.Create) and that is still open. A descriptor of the host, or a
//     number that a closed file of the plugin had, is refused.
//   - Mmap: only on such a file, readable or writable (no PROT_EXEC), with
//     MAP_SHARED or MAP_PRIVATE only (no MAP_FIXED: it would replace memory
//     of the host; no MAP_ANON). A write to a shared mapping is a write to
//     the file, which the plugin can do with the file already.
//   - Munmap, Madvise, Msync: only on a mapping that the plugin made.
//   - Syscall and Syscall6: only munmap of a mapping of the plugin, and on
//     linux mremap of one with MREMAP_MAYMOVE only (ristretto, the cache of
//     badger, makes these two calls itself). Any other system call panics
//     with a *RefusedError.
//
// The memory of a mapping is real bytes: the plugin sees them as plain data
// (byteview.go), as with a []byte of make. As in Go, a read or a write of a
// mapping after Munmap stops the process: the VM does not check it.

import (
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

// fileMaps holds the open files and the mappings of a machine.
type fileMaps struct {
	mu sync.Mutex
	// files are the files that the plugin opened, by descriptor. A weak
	// pointer: a file that the plugin drops is closed by its finalizer.
	files map[int]weak.Pointer[os.File]
	// maps are the mappings of the plugin, by the address of their first
	// byte.
	maps map[uintptr][]byte
	// n is len(maps), read without the lock.
	n atomic.Int32
}

// setMapping records b as the mapping at addr, or removes the mapping at
// addr when b is nil. The caller holds m.fmaps.mu.
func (m *Machine) setMappingLocked(addr uintptr, b []byte) {
	if b == nil {
		delete(m.fmaps.maps, addr)
	} else {
		if m.fmaps.maps == nil {
			m.fmaps.maps = map[uintptr][]byte{}
		}
		m.fmaps.maps[addr] = b
	}
	m.fmaps.n.Store(int32(len(m.fmaps.maps)))
}

// trackOwnFiles makes the functions that open a file record the files of
// the plugin (Flock and Mmap accept only them). It wraps the functions that
// guardHostFiles or a Sandbox installed.
func (m *Machine) trackOwnFiles() {
	if !m.unsafeSyscall || m.realStd {
		return
	}
	wrap := func(key string, open func(f *os.File)) {
		switch fn := m.overrides[key].Interface().(type) {
		case func(string) (*os.File, error):
			m.overrides[key] = reflect.ValueOf(func(name string) (*os.File, error) {
				f, err := fn(name)
				open(f)
				return f, err
			})
		case func(string, int, os.FileMode) (*os.File, error):
			m.overrides[key] = reflect.ValueOf(func(name string, flag int, perm os.FileMode) (*os.File, error) {
				f, err := fn(name, flag, perm)
				open(f)
				return f, err
			})
		}
	}
	for _, key := range []string{"os.Open", "os.OpenFile", "os.Create"} {
		if _, ok := m.overrides[key]; ok {
			wrap(key, m.ownFile)
		}
	}
}

// ownFile records f, a file that the plugin opened (nil: nothing).
func (m *Machine) ownFile(f *os.File) {
	if f == nil {
		return
	}
	fd, ok := fileFD(f)
	if !ok {
		return
	}
	m.fmaps.mu.Lock()
	defer m.fmaps.mu.Unlock()
	if m.fmaps.files == nil {
		m.fmaps.files = map[int]weak.Pointer[os.File]{}
	}
	m.fmaps.files[fd] = weak.Make(f)
}

// fileFD returns the descriptor of f, or false when f is closed. It does not
// change the mode of f, as f.Fd would.
func fileFD(f *os.File) (int, bool) {
	sc, err := f.SyscallConn()
	if err != nil {
		return -1, false
	}
	fd := -1
	if sc.Control(func(x uintptr) { fd = int(x) }) != nil {
		return -1, false
	}
	return fd, fd >= 0
}

// ownFD reports whether fd is the descriptor of a file that the plugin
// opened and that is still open.
func (m *Machine) ownFD(fd int) bool {
	m.fmaps.mu.Lock()
	w, ok := m.fmaps.files[fd]
	m.fmaps.mu.Unlock()
	if !ok {
		return false
	}
	f := w.Value()
	if f == nil {
		return false
	}
	got, ok := fileFD(f)
	return ok && got == fd
}

// addMapping records b, a mapping that the plugin made.
func (m *Machine) addMapping(b []byte) {
	if len(b) == 0 {
		return
	}
	m.fmaps.mu.Lock()
	defer m.fmaps.mu.Unlock()
	m.setMappingLocked(uintptr(unsafe.Pointer(&b[0])), b)
}

// mappingLocked returns the mapping of the plugin that starts at addr and
// has n bytes, or nil. The caller holds m.fmaps.mu.
func (m *Machine) mappingLocked(addr, n uintptr) []byte {
	b, ok := m.fmaps.maps[addr]
	if !ok || uintptr(len(b)) != n {
		return nil
	}
	return b
}

// inMapping reports whether the n bytes at addr are inside one mapping of
// the plugin.
func (m *Machine) inMapping(addr, n uintptr) bool {
	m.fmaps.mu.Lock()
	defer m.fmaps.mu.Unlock()
	for base, b := range m.fmaps.maps {
		if addr >= base && n <= uintptr(len(b)) && addr-base <= uintptr(len(b))-n {
			return true
		}
	}
	return false
}

// mappingAt returns the mapping of the plugin that holds the address addr,
// or nil. The VM finds the memory of a pointer that the plugin makes from
// an address of a mapping (unsafe.Pointer(uintptr), the Data of a slice
// header) there.
func (m *Machine) mappingAt(addr uintptr) []byte {
	if m.fmaps.n.Load() == 0 {
		return nil // most machines: no lock
	}
	m.fmaps.mu.Lock()
	defer m.fmaps.mu.Unlock()
	for base, b := range m.fmaps.maps {
		if addr >= base && addr-base < uintptr(len(b)) {
			return b
		}
	}
	return nil
}
