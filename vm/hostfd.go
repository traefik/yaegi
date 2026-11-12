package vm

// The descriptors of the host and a restricted plugin.
//
// A restricted plugin (Load without Unrestricted, a Sandbox, RunMain with
// Restricted) must not get a file descriptor that the host owns: a file,
// a pipe, a socket, a listener that systemd gives to Traefik. os.NewFile
// panics (newfile.go). The other ways to the descriptors of the host are
// closed here:
//
//   - The filesystem names the descriptors of the process. On darwin and
//     the BSDs, opening /dev/fd/N (or /dev/stdin, a link to fd/0) is a dup
//     of the descriptor N, a socket too. On linux, /proc/self/fd/N,
//     /proc/<pid>/fd/N, /proc/thread-self/fd/N and /dev/fd/N are "magic"
//     links that open the file or the pipe behind N again. The functions
//     of the plugin that open a path (os.Open, os.OpenFile, os.Create,
//     os.ReadFile, os.WriteFile, the same in io/ioutil, the Open and
//     ReadFile methods of os.DirFS, http.Dir and its uses in http.FileServer,
//     http.NewFileTransport and http.ServeFile) open it with openNoAlias
//     (hostfd_walk.go). A path check would not hold: links, "..", relative
//     paths and a link that the plugin changes between the check and the
//     open all lead there. openNoAlias opens the parent directory, checks
//     that directory (not its name) with fstat, and opens the last element
//     relative to it with O_NOFOLLOW; a symbolic link in the last element is
//     read with readlinkat and resolved again the same way. So the kernel
//     never follows a link in the last element: the magic links of linux
//     are read as text ("pipe:[123]", "socket:[456]", a path), which opens
//     no descriptor; and on the other systems an entry of the descriptor
//     directory (/dev/fd, compared by device and inode) is refused. On
//     linux, /proc/<pid>/mem is refused too: it writes the memory of the
//     host. The standard output and error are the exception (stdEntry):
//     /dev/stdout, /dev/stderr, and the entries 1 and 2 of /dev/fd and of
//     /proc/self/fd (or /proc/<pid>/fd of the process) give a duplicate of
//     the os.Stdout or os.Stderr of the plugin (dupStd): the pipe to the
//     writer of Options.Stdout and Options.Stderr, or without them its
//     own copy of the descriptor 1 or 2. Logging plugins write their log
//     to /dev/stdout.
//   - os.OpenRoot, os.OpenInRoot and (*os.Root).OpenRoot: an os.Root
//     resolves names itself (os/root_openat.go). On linux it reads every
//     link as text, so the descriptors are out of reach, but a root that
//     holds /proc (the root directory, /proc and the directories of procfs)
//     reaches /proc/self/mem, and on the other systems a root that holds
//     /dev/fd (/, /dev, /dev/fd) reaches its entries. Such roots are
//     refused when they are opened; the directory of a root never changes,
//     so the check holds for its whole life.
//   - net.FileConn, net.FileListener and net.FilePacketConn return an
//     error: the file of a plugin can only be a socket when it is a
//     descriptor of the host (the host stdin as a socket under inetd or
//     systemd, a /dev/fd entry). Libraries of the catalog name them only
//     after os.NewFile (gin RunFd, caddy listeners), which panics already.
//   - os.Stdin, os.Stdout and os.Stderr of the plugin are its own
//     duplicates of the host files (ownStdFile): Close, Fd and SyscallConn
//     act on the duplicate. The writes go to the same open file of the
//     host, at once, in the order of the calls (stdio.go).
//
// Limits (docs/SANDBOX.md): native code that opens a path by itself (the
// AOT copies, for example) is not checked, and neither are the functions
// that act on a path without opening it.
//
// Unrestricted machines keep the functions of Go.

import (
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

// errNetFile refuses the net functions that make a connection of any file.
var errNetFile = &RefusedError{What: "net.FileConn, net.FileListener and net.FilePacketConn (a socket of any file)"}

// guardHostFiles installs, on a restricted machine, the functions of this
// file and the refusal of os.NewFile. An override that is already there (a
// Sandbox, which calls openFile itself) stays.
func (m *Machine) guardHostFiles() {
	if !m.restricted {
		return
	}
	m.refuseNewFile()
	set := func(key string, fn any) {
		if _, ok := m.overrides[key]; !ok {
			m.addOverride(key, fn)
		}
	}
	set("os.Open", func(name string) (*os.File, error) { return openFile(name, os.O_RDONLY, 0) })
	set("os.OpenFile", openFile)
	set("os.Create", createFile)
	set("os.ReadFile", readFile)
	set("os.WriteFile", writeFile)
	set("io/ioutil.ReadFile", readFile)
	set("io/ioutil.WriteFile", writeFile)
	set("os.DirFS", func(dir string) fs.FS { return dirFS(dir) })
	set("os.OpenRoot", openRoot)
	set("os.OpenInRoot", func(dir, name string) (*os.File, error) {
		r, err := openRoot(dir)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return r.Open(name)
	})
	set("net.FileConn", func(*os.File) (net.Conn, error) { return nil, netFileError() })
	set("net.FileListener", func(*os.File) (net.Listener, error) { return nil, netFileError() })
	set("net.FilePacketConn", func(*os.File) (net.PacketConn, error) { return nil, netFileError() })
	set("net/http.FileServer", func(root http.FileSystem) http.Handler { return http.FileServer(ownHTTPDir(root)) })
	set("net/http.NewFileTransport", func(root http.FileSystem) http.RoundTripper {
		return http.NewFileTransport(ownHTTPDir(root))
	})
	set("net/http.ServeFile", func(w http.ResponseWriter, r *http.Request, name string) {
		// As http.ServeFile, which serves file from http.Dir(dir).
		dir, file := filepath.Split(name)
		http.ServeFileFS(w, r, httpDirFS(dir), file)
	})
	m.ownStdio()
}

// netFileError is the error of net.FileConn, net.FileListener and
// net.FilePacketConn in a restricted plugin, as the error of net for a file
// that is not a socket.
func netFileError() error {
	return &net.OpError{Op: "file", Net: "file+net", Err: errNetFile}
}

// openFile is os.OpenFile for a restricted plugin (see the top of the file).
func openFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return openNoAlias(name, flag, perm)
}

// createFile is os.Create for a restricted plugin.
func createFile(name string) (*os.File, error) {
	return openFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// readFile is os.ReadFile for a restricted plugin.
func readFile(name string) ([]byte, error) {
	f, err := openFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// writeFile is os.WriteFile for a restricted plugin.
func writeFile(name string, data []byte, perm os.FileMode) error {
	f, err := openFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}

// dirFS is os.DirFS for a restricted plugin: Open and ReadFile open the
// file with openNoAlias; the other methods open nothing and are the ones of
// os.DirFS.
type dirFS string

func (dir dirFS) native() fs.FS { return os.DirFS(string(dir)) }

// join is the join method of os.DirFS.
func (dir dirFS) join(name string) (string, error) {
	if dir == "" {
		return "", errors.New("os: DirFS with empty root")
	}
	name, err := filepath.Localize(name)
	if err != nil {
		return "", os.ErrInvalid
	}
	if os.IsPathSeparator(dir[len(dir)-1]) {
		return string(dir) + name, nil
	}
	return string(dir) + string(os.PathSeparator) + name, nil
}

func (dir dirFS) Open(name string) (fs.File, error) {
	full, err := dir.join(name)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	f, err := openFile(full, os.O_RDONLY, 0)
	if err != nil {
		if e, ok := err.(*os.PathError); ok {
			e.Path = name // as os.DirFS: the name of the fs, not of the system
		}
		return nil, err
	}
	return f, nil
}

func (dir dirFS) ReadFile(name string) ([]byte, error) {
	full, err := dir.join(name)
	if err != nil {
		return nil, &os.PathError{Op: "readfile", Path: name, Err: err}
	}
	b, err := readFile(full)
	if err != nil {
		if e, ok := err.(*os.PathError); ok {
			e.Path = name
		}
		return nil, err
	}
	return b, nil
}

func (dir dirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return dir.native().(fs.ReadDirFS).ReadDir(name)
}

func (dir dirFS) Stat(name string) (fs.FileInfo, error) {
	return dir.native().(fs.StatFS).Stat(name)
}

func (dir dirFS) Lstat(name string) (fs.FileInfo, error) {
	return dir.native().(fs.ReadLinkFS).Lstat(name)
}

func (dir dirFS) ReadLink(name string) (string, error) {
	return dir.native().(fs.ReadLinkFS).ReadLink(name)
}

// httpDir is http.Dir for a restricted plugin: Open opens the file with
// openNoAlias.
type httpDir string

// ownHTTPDir returns root, or an httpDir when root is an http.Dir.
func ownHTTPDir(root http.FileSystem) http.FileSystem {
	if d, ok := root.(http.Dir); ok {
		return httpDir(d)
	}
	return root
}

// Open is the Open method of http.Dir.
func (d httpDir) Open(name string) (http.File, error) {
	p := path.Clean("/" + name)[1:]
	if p == "" {
		p = "."
	}
	p, err := filepath.Localize(p)
	if err != nil {
		return nil, errors.New("http: invalid or unsafe file path")
	}
	dir := string(d)
	if dir == "" {
		dir = "."
	}
	full := filepath.Join(dir, p)
	f, err := openFile(full, os.O_RDONLY, 0)
	if err != nil {
		return nil, mapOpenError(err, full, filepath.Separator, os.Stat)
	}
	return f, nil
}

// mapOpenError is the function of net/http that http.Dir uses: an error
// because an element of the path is not a directory is fs.ErrNotExist.
func mapOpenError(originalErr error, name string, sep rune, stat func(string) (fs.FileInfo, error)) error {
	if errors.Is(originalErr, fs.ErrNotExist) || errors.Is(originalErr, fs.ErrPermission) {
		return originalErr
	}
	parts := strings.Split(name, string(sep))
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		fi, err := stat(strings.Join(parts[:i+1], string(sep)))
		if err != nil {
			return originalErr
		}
		if !fi.IsDir() {
			return fs.ErrNotExist
		}
	}
	return originalErr
}

// httpDirFS is an fs.FS whose Open is the Open method of httpDir, for
// http.ServeFileFS (the http.ServeFile of a restricted plugin).
type httpDirFS string

func (d httpDirFS) Open(name string) (fs.File, error) {
	f, err := httpDir(d).Open(name)
	if err != nil {
		return nil, err
	}
	return f.(fs.File), nil
}

// openRoot is os.OpenRoot for a restricted plugin: a root that holds /dev/fd
// or /proc is refused (see the top of the file).
func openRoot(name string) (*os.Root, error) {
	r, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	if err := checkRoot(r); err != nil {
		r.Close()
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return r, nil
}

// rootOpenRoot is (*os.Root).OpenRoot for a restricted plugin.
func rootOpenRoot(r *os.Root, name string) (*os.Root, error) {
	s, err := r.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	if err := checkRoot(s); err != nil {
		s.Close()
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	return s, nil
}

// hostFDMethods adds to refused the methods that a restricted machine gets
// instead of the native ones (refusedmeth.go): http.Dir.Open and
// (*os.Root).OpenRoot.
func hostFDMethods(refused map[methodKey]reflect.Value) {
	refused[methodKey{reflect.TypeFor[http.Dir](), "Open"}] = reflect.ValueOf(func(d http.Dir, name string) (http.File, error) {
		return httpDir(d).Open(name)
	})
	refused[methodKey{reflect.TypeFor[*os.Root](), "OpenRoot"}] = reflect.ValueOf(rootOpenRoot)
}
