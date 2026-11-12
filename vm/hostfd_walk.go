//go:build linux || darwin || freebsd || netbsd || openbsd

package vm

// openNoAlias and the checks of os.Root (hostfd.go) on the systems where
// they are written and tested. The other systems open as Go does
// (hostfd_other.go).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// errFDEntry, errProcMem and errFDRoot are the errors of openNoAlias and of
// the checks of an os.Root.
var (
	errFDEntry = &RefusedError{What: "opening a descriptor of the process (/dev/fd/N, /proc/self/fd/N)"}
	errProcMem = &RefusedError{What: "opening /proc/<pid>/mem"}
	errFDRoot  = &RefusedError{What: "an os.Root that holds /dev/fd or /proc"}
)

// devIno is a file of the system.
type devIno struct{ dev, ino uint64 }

func statKey(st *unix.Stat_t) devIno { return devIno{uint64(st.Dev), uint64(st.Ino)} }

func fstatKey(fd int) (devIno, error) {
	var st unix.Stat_t
	err := ignoringEINTR(func() error { return unix.Fstat(fd, &st) })
	return statKey(&st), err
}

func ignoringEINTR(fn func() error) error {
	for {
		if err := fn(); err != syscall.EINTR {
			return err
		}
	}
}

// fdDirs are the directories whose entries are the descriptors of the
// process on the systems other than linux: /dev/fd, and /proc/self/fd
// when it exists. On linux the entries are links, which openNoAlias reads
// as text.
var fdDirs = sync.OnceValue(func() map[devIno]bool {
	dirs := map[devIno]bool{}
	for _, p := range []string{"/dev/fd", "/proc/self/fd"} {
		var st unix.Stat_t
		if unix.Stat(p, &st) == nil {
			dirs[statKey(&st)] = true
		}
	}
	return dirs
})

// openNoAlias is os.OpenFile, except that it never opens a descriptor of
// the process, nor /proc/<pid>/mem on linux (hostfd.go).
func openNoAlias(name string, flag int, perm os.FileMode) (*os.File, error) {
	if name == "" {
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.ENOENT}
	}
	fd, std, err := openatNoAlias(name, flag, syscallMode(perm))
	if err == nil && std > 0 {
		fd, err = dupStd(std)
	}
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	if std > 0 {
		// A duplicate of the standard output or error, as ownStdFile: its
		// flags are the ones of the file it duplicates and must not change
		// (newOpenedFile would make a pipe or a terminal non-blocking for
		// the host too).
		return os.NewFile(uintptr(fd), name), nil
	}
	return newOpenedFile(fd, name, flag), nil
}

// stdEntry returns 1 or 2 when the element file of the directory pfd is
// the standard output or the standard error of the process: /dev/stdout
// and /dev/stderr lead there through a link, and /dev/fd/1,
// /proc/self/fd/2 or /proc/<pid>/fd/1 name them. openNoAlias then gives a
// duplicate of the os.Stdout or os.Stderr of the plugin (dupStd), so the
// plugin can write and close it without closing the descriptor of the
// host. Logging plugins write their log to /dev/stdout to reach the log of
// the container. It returns -1 for any other file: the other descriptors
// of the process stay out of reach. Go follows no link with O_NOFOLLOW,
// and O_CREATE|O_EXCL fails on a file that exists: these open the entry
// itself as before.
func stdEntry(pfd int, file string, flag int, mustDir bool) int {
	if file != "1" && file != "2" || mustDir || flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL {
		return -1
	}
	k, err := fstatKey(pfd)
	if err != nil {
		return -1
	}
	if runtime.GOOS == "linux" {
		// The entries are links, not followed with O_NOFOLLOW. The directory
		// must be the one of this process (/proc/self/fd, /proc/<pid>/fd,
		// /dev/fd): procfs gives another directory to each process, and
		// keeps the inode of a directory while pfd holds it open. Another
		// process, or /proc/thread-self/fd, is read as text as before.
		if flag&unix.O_NOFOLLOW != 0 || !isProcFS(pfd) {
			return -1
		}
		var st unix.Stat_t
		if unix.Stat("/proc/self/fd", &st) != nil || statKey(&st) != k {
			return -1
		}
	} else if !fdDirs()[k] {
		return -1
	}
	if file == "1" {
		return 1
	}
	return 2
}

// dupStd returns a new descriptor, closed on exec, for the standard output
// (n 1) or the standard error (n 2) of the plugin that runs on this
// goroutine (stdEntry): the file that its os.Stdout or os.Stderr variable
// holds (SetStdio, ownStdio). With Options.Stdout and Options.Stderr set,
// that file is the pipe to the writer of the host, so the lines reach the
// writer as the ones of fmt.Println do, and never the descriptor 1 or 2 of
// the host behind it. Without a plugin on the goroutine, or when the
// variable holds nothing, it is the descriptor n of the process, as
// os.Stdout starts. A closed file gives ENOENT, as the magic link of linux
// of a closed descriptor.
func dupStd(n int) (int, error) {
	fd := -1
	dup := func(from uintptr) error {
		return ignoringEINTR(func() error {
			var err error
			fd, err = unix.FcntlInt(from, unix.F_DUPFD_CLOEXEC, 3)
			return err
		})
	}
	m := curMachine()
	if m == nil {
		return fd, dup(uintptr(n))
	}
	key := "os.Stdout"
	if n == 2 {
		key = "os.Stderr"
	}
	h := m.varHooks[key]
	if h == nil {
		return fd, dup(uintptr(n))
	}
	p, ok := h().Interface().(**os.File)
	if !ok || p == nil || *p == nil {
		return -1, syscall.ENOENT
	}
	sc, err := (*p).SyscallConn()
	if err != nil {
		return -1, syscall.ENOENT
	}
	var derr error
	if err := sc.Control(func(from uintptr) { derr = dup(from) }); err != nil {
		return -1, syscall.ENOENT
	}
	return fd, derr
}

// syscallMode is the syscallMode of package os.
func syscallMode(i os.FileMode) uint32 {
	o := uint32(i.Perm())
	if i&os.ModeSetuid != 0 {
		o |= unix.S_ISUID
	}
	if i&os.ModeSetgid != 0 {
		o |= unix.S_ISGID
	}
	if i&os.ModeSticky != 0 {
		o |= unix.S_ISVTX
	}
	return o
}

// openatNoAlias opens name with flag, one parent directory at a time: the
// kernel resolves the directory of the last element (links included), the
// directory is checked (checkParent), and the last element is opened in it
// with O_NOFOLLOW. When the last element is a symbolic link, its target is
// resolved again the same way, from that directory when it is relative.
// The kernel never gives a file that it reached through a link of the last
// element, so a magic link of linux is only read as text. (With a final
// slash, the kernel follows a link to a directory: that directory is
// closed, and the link is resolved here. Such a directory is only ever a
// directory, which openDir opens as a parent anyway.) When the last
// element is the standard output or error (stdEntry), it opens nothing
// and returns std 1 or 2: openNoAlias duplicates the file of the plugin.
func openatNoAlias(name string, flag int, mode uint32) (int, int, error) {
	base := unix.AT_FDCWD // the directory that a relative path starts from
	defer func() {
		if base != unix.AT_FDCWD {
			unix.Close(base)
		}
	}()
	p := name
	for links := 0; ; links++ {
		dir, file, mustDir := splitLast(p)
		pfd, err := openDir(base, dir)
		if err != nil {
			return -1, 0, err
		}
		if n := stdEntry(pfd, file, flag, mustDir); n >= 0 {
			unix.Close(pfd)
			return -1, n, nil
		}
		if err := checkParent(pfd, file); err != nil {
			unix.Close(pfd)
			return -1, 0, err
		}
		f := flag | unix.O_NOFOLLOW | unix.O_CLOEXEC | openExtra
		entry := file
		if mustDir {
			// The final slash goes to the kernel, which gives the errors of
			// os.OpenFile (ENOTDIR, EISDIR with O_CREATE). It follows a
			// link then, O_NOFOLLOW or not: such a file is closed and the
			// link is resolved here (sameEntry).
			entry += "/"
		}
		// Go sets the sticky bit of a new file itself on the systems where
		// open ignores it (os/sticky_bsd.go): the entry must not exist yet.
		setSticky := false
		if flag&os.O_CREATE != 0 && mode&unix.S_ISVTX != 0 && !createKeepsSticky {
			var st unix.Stat_t
			setSticky = unix.Fstatat(pfd, file, &st, unix.AT_SYMLINK_NOFOLLOW) == syscall.ENOENT
		}
		var fd int
		err = ignoringEINTR(func() error {
			var err error
			fd, err = unix.Openat(pfd, entry, f, mode)
			return err
		})
		if err == nil {
			if !mustDir || sameEntry(pfd, file, fd) {
				if setSticky {
					var st unix.Stat_t
					if unix.Fstat(fd, &st) == nil {
						unix.Fchmod(fd, uint32(st.Mode)&0o7777|unix.S_ISVTX)
					}
				}
				unix.Close(pfd)
				return fd, 0, nil
			}
			// The kernel followed a link to a directory.
			unix.Close(fd)
			err = syscall.ELOOP
		} else if flag&unix.O_NOFOLLOW != 0 || flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL {
			// A link of the last element: the open fails (ELOOP, EMLINK on
			// FreeBSD, EFTYPE on NetBSD). It is not followed with
			// O_NOFOLLOW, nor with O_CREATE|O_EXCL, as in Go.
			unix.Close(pfd)
			return -1, 0, err
		}
		target, lerr := readlinkat(pfd, file)
		if lerr != nil {
			unix.Close(pfd)
			return -1, 0, err // not a link: the error of the open
		}
		if links >= maxLinks {
			unix.Close(pfd)
			return -1, 0, syscall.ELOOP
		}
		if mustDir {
			target += "/"
		}
		if base != unix.AT_FDCWD {
			unix.Close(base)
		}
		if strings.HasPrefix(target, "/") {
			unix.Close(pfd)
			base = unix.AT_FDCWD
		} else {
			base = pfd // a relative link starts from its directory
		}
		p = target
	}
}

// sameEntry reports whether the open file fd is the entry file of the
// directory pfd itself, and not the target of a link there: a link has an
// inode of its own.
func sameEntry(pfd int, file string, fd int) bool {
	var lst, st unix.Stat_t
	if err := ignoringEINTR(func() error { return unix.Fstatat(pfd, file, &lst, unix.AT_SYMLINK_NOFOLLOW) }); err != nil {
		return false
	}
	if err := ignoringEINTR(func() error { return unix.Fstat(fd, &st) }); err != nil {
		return false
	}
	return statKey(&lst) == statKey(&st)
}

// splitLast splits p into the directory and the last element. mustDir
// reports a final slash. The root directory is "/" and ".".
func splitLast(p string) (dir, file string, mustDir bool) {
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" {
		return "/", ".", false
	}
	mustDir = len(trimmed) < len(p)
	i := strings.LastIndexByte(trimmed, '/')
	if i < 0 {
		return ".", trimmed, mustDir
	}
	dir = trimmed[:i]
	if dir == "" {
		dir = "/"
	}
	return dir, trimmed[i+1:], mustDir
}

// openDir opens the directory dir (relative to base) for openat. It needs
// only the right to search it, as the lookup of the kernel.
func openDir(base int, dir string) (int, error) {
	var fd int
	err := ignoringEINTR(func() error {
		var err error
		fd, err = unix.Openat(base, dir, unix.O_DIRECTORY|unix.O_CLOEXEC|searchFlag, 0)
		return err
	})
	if err != nil && searchFlag != 0 && err != syscall.ENOENT && err != syscall.ENOTDIR {
		// An older system without the flag.
		err = ignoringEINTR(func() error {
			var err error
			fd, err = unix.Openat(base, dir, unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_RDONLY, 0)
			return err
		})
	}
	return fd, err
}

// readlinkat returns the target of the link name in the directory dirfd.
func readlinkat(dirfd int, name string) (string, error) {
	for size := 256; ; size *= 2 {
		buf := make([]byte, size)
		var n int
		err := ignoringEINTR(func() error {
			var err error
			n, err = unix.Readlinkat(dirfd, name, buf)
			return err
		})
		if err != nil {
			return "", err
		}
		if n < size {
			return string(buf[:n]), nil
		}
	}
}

// checkParent refuses to open the element file of the directory pfd when
// it is a descriptor of the process (an entry of fdDirs), or the memory of
// a process on linux.
func checkParent(pfd int, file string) error {
	if file == "." || file == ".." {
		return nil
	}
	if runtime.GOOS == "linux" {
		if file == "mem" && isProcFS(pfd) {
			return errProcMem
		}
		return nil
	}
	k, err := fstatKey(pfd)
	if err != nil {
		return err
	}
	if fdDirs()[k] {
		return errFDEntry
	}
	return nil
}

// newOpenedFile makes the *os.File of the descriptor fd that openNoAlias
// opened, as os.OpenFile does: a pipe, a terminal or a device goes to the
// poller of Go (not a regular file or a directory, and not a FIFO on
// darwin). os.NewFile reads O_APPEND from the descriptor, so WriteAt is
// refused as in Go.
func newOpenedFile(fd int, name string, flag int) *os.File {
	if flag&unix.O_NONBLOCK == 0 {
		var st unix.Stat_t
		if unix.Fstat(fd, &st) == nil {
			switch st.Mode & unix.S_IFMT {
			case unix.S_IFREG, unix.S_IFDIR:
			case unix.S_IFIFO:
				if runtime.GOOS != "darwin" {
					unix.SetNonblock(fd, true)
				}
			default:
				unix.SetNonblock(fd, true)
			}
		}
	}
	return os.NewFile(uintptr(fd), name)
}

// checkRoot refuses a root whose tree holds the descriptors of the
// process (/dev/fd) or, on linux, /proc (hostfd.go): its directory is one
// of refusedRoots, or in a procfs (isProcFS, whatever the mount: a
// container can have the procfs of the host at another path, with another
// device). The checks are on the directory of the root itself, which
// never changes, not on a name.
func checkRoot(r *os.Root) error {
	d, err := r.Open(".")
	if err != nil {
		return err
	}
	defer d.Close()
	rc, err := d.SyscallConn()
	if err != nil {
		return err
	}
	var k devIno
	var proc bool
	var kerr error
	if err := rc.Control(func(fd uintptr) {
		k, kerr = fstatKey(int(fd))
		proc = isProcFS(int(fd))
	}); err != nil {
		return err
	}
	if kerr != nil {
		return kerr
	}
	if proc || refusedRoots()[k] {
		return errFDRoot
	}
	return nil
}

// refusedRoots are the directories that hold the descriptors of the
// process (/dev/fd, /proc/self/fd; /proc on linux), and their parents.
var refusedRoots = sync.OnceValue(func() map[devIno]bool {
	dirs := map[devIno]bool{}
	held := []string{"/dev/fd", "/proc/self/fd"}
	if runtime.GOOS == "linux" {
		held = []string{"/proc"}
	}
	for _, p := range held {
		p, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		for {
			var st unix.Stat_t
			if unix.Stat(p, &st) == nil {
				dirs[statKey(&st)] = true
			}
			parent := filepath.Dir(p)
			if parent == p {
				break
			}
			p = parent
		}
	}
	return dirs
})

// ownStdFile returns a duplicate of the host file f (os.Stdin, os.Stdout,
// os.Stderr or a file of Options.Stdout), with the same name, for the
// plugin (stdio.go). When f cannot be duplicated (a closed descriptor),
// the file is /dev/null.
func ownStdFile(f *os.File, write bool) *os.File {
	if f != nil {
		if rc, err := f.SyscallConn(); err == nil {
			nfd := -1
			rc.Control(func(fd uintptr) {
				nfd, _ = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 3)
			})
			if nfd >= 0 {
				return os.NewFile(uintptr(nfd), f.Name())
			}
		}
	}
	return devNull(f, write)
}
