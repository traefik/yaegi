//go:build linux || darwin

package vm

import (
	"context"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// fileMapPlug locks and maps files with golang.org/x/sys/unix, as badger and
// nutsdb do: allowed on the files and the mappings of the plugin only
// (filemap.go).
const fileMapPlug = `package plug

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

func open(dir string) *os.File {
	f, err := os.OpenFile(filepath.Join(dir, "data"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		panic(err)
	}
	if err := f.Truncate(1 << 12); err != nil {
		panic(err)
	}
	return f
}

// Own locks, maps, writes, syncs and unmaps a file of the plugin, and
// returns what the file holds.
func Own(dir string) string {
	f := open(dir)
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		panic(err)
	}
	b, err := unix.Mmap(int(f.Fd()), 0, 1<<12, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		panic(err)
	}
	copy(b, "mapped")
	if err := unix.Msync(b, unix.MS_SYNC); err != nil {
		panic(err)
	}
	if err := unix.Munmap(b); err != nil {
		panic(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "data"))
	if err != nil {
		panic(err)
	}
	return string(data[:6])
}

// Advise maps 64 MiB of a file of the plugin, and advises and syncs the
// mapping n times, as badger does with its write-ahead log.
func Advise(dir string, n int) error {
	f, err := os.OpenFile(filepath.Join(dir, "big"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(64 << 20); err != nil {
		return err
	}
	b, err := unix.Mmap(int(f.Fd()), 0, 64<<20, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return err
	}
	for range n {
		if err := unix.Madvise(b, unix.MADV_NORMAL); err != nil {
			return err
		}
		if err := unix.Msync(b, unix.MS_ASYNC); err != nil {
			return err
		}
	}
	return unix.Munmap(b)
}

// FlockStdin locks a descriptor that the plugin did not open.
func FlockStdin() error { return unix.Flock(0, unix.LOCK_SH) }

// FlockClosed locks the descriptor of a file of the plugin after Close.
func FlockClosed(dir string) error {
	f := open(dir)
	fd := int(f.Fd())
	f.Close()
	return unix.Flock(fd, unix.LOCK_SH)
}

// MmapExec maps a file of the plugin as code.
func MmapExec(dir string) error {
	f := open(dir)
	defer f.Close()
	_, err := unix.Mmap(int(f.Fd()), 0, 1<<12, unix.PROT_READ|unix.PROT_EXEC, unix.MAP_SHARED)
	return err
}

// MmapFixed maps a file of the plugin at an address it chooses.
func MmapFixed(dir string) error {
	f := open(dir)
	defer f.Close()
	_, err := unix.Mmap(int(f.Fd()), 0, 1<<12, unix.PROT_READ, unix.MAP_SHARED|unix.MAP_FIXED)
	return err
}

// MmapAnon maps memory with no file.
func MmapAnon() error {
	_, err := unix.Mmap(-1, 0, 1<<12, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	return err
}

// MunmapHeap unmaps memory that is not a mapping.
func MunmapHeap() error { return unix.Munmap(make([]byte, 1<<12)) }

// MadviseHeap gives advice on memory that is not a mapping.
func MadviseHeap() error { return unix.Madvise(make([]byte, 1<<12), unix.MADV_DONTNEED) }

// RawMunmapHeap unmaps memory that is not a mapping with a raw system call.
func RawMunmapHeap() error {
	b := make([]byte, 1<<12)
	_, _, errno := unix.Syscall(unix.SYS_MUNMAP, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// RawGetpid makes another system call.
func RawGetpid() uintptr {
	pid, _, _ := unix.Syscall(unix.SYS_GETPID, 0, 0, 0)
	return pid
}
`

// TestFileMap checks the calls of filemap.go in a restricted plugin with
// UseUnsafe: flock, mmap, msync and munmap work on the files and the
// mappings of the plugin, and are refused on anything else. The length of
// a mapping does not count in the budget of Msync and Madvise
// (memRangeFuncs): 1000 of each on 64 MiB cost 2e9 steps before.
func TestFileMap(t *testing.T) {
	if !stdtypes.Available() || natPkg(xsysUnixPath) == nil {
		t.Skip("no embedded standard library types or no golang.org/x/sys/unix table")
	}
	dir := writeXSysPlugin(t, "v0.48.0", "", xsysSources(t), map[string]string{"plug.go": fileMapPlug})
	for _, opts := range []Options{
		{Dir: dir, UseUnsafe: true, Budget: 1e7},
		{Dir: dir, UseUnsafe: true, Budget: 1e7, AOT: true},
	} {
		p, err := Load(opts, "example.com/plug")
		if err != nil {
			t.Fatalf("AOT=%v: %v", opts.AOT, err)
		}
		res, err := p.Call(context.Background(), "Own", t.TempDir())
		if err != nil {
			t.Fatalf("AOT=%v Own: %v", opts.AOT, err)
		}
		if v, _ := res[0].Interface(); v != "mapped" {
			t.Errorf("AOT=%v Own: %v, want mapped", opts.AOT, v)
		}
		res, err = p.Call(context.Background(), "Advise", t.TempDir(), 1000)
		if err != nil {
			t.Fatalf("AOT=%v Advise: %v", opts.AOT, err)
		}
		if v, _ := res[0].Interface(); v != nil {
			t.Errorf("AOT=%v Advise: %v", opts.AOT, v)
		}
		for _, c := range []struct {
			name, want string
			dir        bool
		}{
			{"FlockStdin", "unix.Flock of a file descriptor that is not a file the plugin opened", false},
			{"FlockClosed", "unix.Flock of a file descriptor that is not a file the plugin opened", true},
			{"MmapExec", "unix.Mmap with the protection", true},
			{"MmapFixed", "unix.Mmap with the flags", true},
			{"MmapAnon", "unix.Mmap of a file descriptor that is not a file the plugin opened", false},
			{"MunmapHeap", "unix.Munmap of memory that is not a mapping the plugin made", false},
			{"MadviseHeap", "unix.Madvise of memory that is not a mapping the plugin made", false},
			{"RawMunmapHeap", "unix.Syscall munmap of memory that is not a mapping the plugin made", false},
			{"RawGetpid", "unix.Syscall of the system call", false},
		} {
			var args []any
			if c.dir {
				args = append(args, t.TempDir())
			}
			_, err := p.Call(context.Background(), c.name, args...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("AOT=%v %s: %v, want %q", opts.AOT, c.name, err, c.want)
			}
		}
	}
}
