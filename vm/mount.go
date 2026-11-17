package vm

// Options.Mounts: the directories of the host that a restricted plugin can
// reach by path.
//
// Without Mounts, a restricted plugin reaches the whole file system, except
// the descriptors of the host (hostfd.go): it can read, write, rename and
// remove any file that the host process can. With Mounts, every function of
// the plugin that takes a path finds the mount that holds it, and acts on
// it through the os.Root of the mount (os.OpenRoot): the kernel resolves the
// path inside the directory, so "..", a symbolic link (absolute or
// relative) and a change of the tree during the call cannot leave it. A
// path is made absolute (filepath.Abs, against the working directory of the
// process) and cleaned before the mount is chosen; the plugin sees each
// directory at its own path, so a file name that the plugin gets back
// (File.Name, the paths of filepath.Walk) is a path it can use again.
//
// A path outside the mounts fails with an *os.PathError whose error wraps
// fs.ErrPermission; so does a write in a read-only mount. A rename or a
// link from one mount to another fails the same way.
//
// The functions that take a path: os.Open, OpenFile, Create, ReadFile,
// WriteFile, Stat, Lstat, ReadDir, Mkdir, MkdirAll, MkdirTemp, CreateTemp,
// Remove, RemoveAll, Rename, Link, Symlink, Readlink, Chmod, Chown, Lchown,
// Chtimes, Truncate, DirFS, OpenRoot and OpenInRoot; their copies in
// io/ioutil (ReadFile, WriteFile, ReadDir, TempFile, TempDir); Walk,
// WalkDir, Glob and EvalSymlinks of path/filepath; http.Dir, http.FileServer,
// http.NewFileTransport and http.ServeFile of net/http; go/parser.ParseFile
// without source; syscall.Statfs (statfs_unix.go).
//
// The native functions that open a path by themselves cannot go through the
// mounts: a static call of one of mountRefused panics with a *RefusedError
// (refusedCallHook). Not covered: other native code that opens a path (the
// AOT copies, a method of a native value called through an interface), and
// the paths of unix sockets (net.Dial("unix", path)), which are addresses of
// the network.

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Mount is a directory of the host that a restricted plugin can reach
// (Options.Mounts).
type Mount struct {
	// Dir is the directory. The plugin sees it at the same path, made
	// absolute. It must exist when the plugin first uses it, and it must not
	// hold /proc or the descriptor directory /dev/fd (as os.OpenRoot in a
	// plugin, hostfd.go).
	Dir string
	// ReadOnly refuses the writes: a file opened for writing, a new or
	// removed entry, a change of mode, owner or times.
	ReadOnly bool
}

// mountError is the error of a path that the mounts refuse. It is
// fs.ErrPermission for errors.Is.
type mountError string

func (e mountError) Error() string        { return string(e) }
func (e mountError) Is(target error) bool { return target == fs.ErrPermission }

const (
	errNotMounted    = mountError("not in a directory mounted for the plugin")
	errReadOnlyMount = mountError("read-only mount of the plugin")
	errCrossMount    = mountError("not in one directory mounted for the plugin")
)

// mountRefused are the native functions that open a path by themselves:
// with Mounts, a call panics (refusedCallHook).
// The functions that read files of the host that the plugin does not name
// (time.LoadLocation, crypto/x509.SystemCertPool, os/user) stay.
var mountRefused = map[string]bool{
	"crypto/tls.LoadX509KeyPair":           true,
	"text/template.ParseFiles":             true,
	"text/template.ParseGlob":              true,
	"(*text/template.Template).ParseFiles": true,
	"(*text/template.Template).ParseGlob":  true,
	"html/template.ParseFiles":             true,
	"html/template.ParseGlob":              true,
	"(*html/template.Template).ParseFiles": true,
	"(*html/template.Template).ParseGlob":  true,
	"archive/zip.OpenReader":               true,
	"go/parser.ParseDir":                   true,
	"debug/buildinfo.ReadFile":             true,
	"debug/elf.Open":                       true,
	"debug/macho.Open":                     true,
	"debug/macho.OpenFat":                  true,
	"debug/pe.Open":                        true,
	"debug/plan9obj.Open":                  true,
	"os.CopyFS":                            true,
}

// mountDir is one mount, with its os.Root, opened at the first use.
type mountDir struct {
	dir  string // absolute and clean
	ro   bool
	once sync.Once
	root *os.Root
	err  error
}

// open returns the os.Root of the mount.
func (d *mountDir) open() (*os.Root, error) {
	d.once.Do(func() {
		r, err := os.OpenRoot(d.dir)
		if err == nil {
			if err = checkRoot(r); err != nil {
				r.Close()
			}
		}
		if err != nil {
			d.err = err
			return
		}
		d.root = r
	})
	return d.root, d.err
}

// mountTable is the mounts of a machine, the longest directory first.
type mountTable struct {
	dirs []*mountDir
}

// newMountTable returns the table of mounts. A directory that is not a
// valid path is left out: no path of the plugin is in it.
func newMountTable(mounts []Mount) *mountTable {
	t := &mountTable{}
	for _, mt := range mounts {
		abs, err := filepath.Abs(mt.Dir)
		if mt.Dir == "" || err != nil {
			continue
		}
		t.dirs = append(t.dirs, &mountDir{dir: filepath.Clean(abs), ro: mt.ReadOnly})
	}
	slices.SortStableFunc(t.dirs, func(a, b *mountDir) int { return len(b.dir) - len(a.dir) })
	return t
}

// find returns the mount that holds the path name, and the path relative
// to it ("." for the directory of the mount).
func (t *mountTable) find(name string) (*mountDir, string, bool) {
	if name == "" {
		return nil, "", false
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		return nil, "", false
	}
	abs = filepath.Clean(abs)
	for _, d := range t.dirs {
		if abs == d.dir {
			return d, ".", true
		}
		prefix := d.dir
		if !os.IsPathSeparator(prefix[len(prefix)-1]) {
			prefix += string(filepath.Separator)
		}
		if strings.HasPrefix(abs, prefix) {
			return d, abs[len(prefix):], true
		}
	}
	return nil, "", false
}

// resolve returns the os.Root and the relative path of name, or the error
// of op: name is outside the mounts, or write is true in a read-only
// mount.
func (t *mountTable) resolve(op, name string, write bool) (*mountDir, *os.Root, string, error) {
	d, rel, ok := t.find(name)
	if !ok {
		if name == "" {
			return nil, nil, "", &os.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
		}
		return nil, nil, "", &os.PathError{Op: op, Path: name, Err: errNotMounted}
	}
	if write && d.ro {
		return nil, nil, "", &os.PathError{Op: op, Path: name, Err: errReadOnlyMount}
	}
	r, err := d.open()
	if err != nil {
		return nil, nil, "", &os.PathError{Op: op, Path: name, Err: err}
	}
	return d, r, rel, nil
}

// named gives the error of an os.Root call the path of the plugin, as the
// error of the os function: os.Root errors name the relative path.
func named(err error, name string) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		pe.Path = name
	}
	return err
}

// stdPath reports whether name is a path of the standard output or error
// of the process: /dev/stdout, /dev/stderr, and the entries 1 and 2 of the
// descriptor directories. Logging plugins write their log there; the plugin
// gets its own copy (stdEntry, hostfd.go), without a mount.
func stdPath(name string) bool {
	p := filepath.ToSlash(filepath.Clean(name))
	if p == "/dev/stdout" || p == "/dev/stderr" {
		return true
	}
	for _, dir := range []string{"/dev/fd", "/proc/self/fd", "/proc/thread-self/fd", "/proc/" + strconv.Itoa(os.Getpid()) + "/fd"} {
		if p == dir+"/1" || p == dir+"/2" {
			return true
		}
	}
	return false
}

// writeFlags are the flags of OpenFile that write.
const writeFlags = os.O_WRONLY | os.O_RDWR | os.O_CREATE | os.O_TRUNC | os.O_APPEND

func (t *mountTable) openFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if _, _, ok := t.find(name); !ok && stdPath(name) {
		return openNoAlias(name, flag, perm) // the plugin's own copy (hostfd.go)
	}
	_, r, rel, err := t.resolve("open", name, flag&writeFlags != 0)
	if err != nil {
		return nil, err
	}
	f, err := r.OpenFile(rel, flag, perm)
	return f, named(err, name)
}

func (t *mountTable) open(name string) (*os.File, error) { return t.openFile(name, os.O_RDONLY, 0) }

func (t *mountTable) create(name string) (*os.File, error) {
	return t.openFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (t *mountTable) readFile(name string) ([]byte, error) {
	f, err := t.open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (t *mountTable) writeFile(name string, data []byte, perm os.FileMode) error {
	f, err := t.openFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}

func (t *mountTable) stat(name string) (os.FileInfo, error) {
	_, r, rel, err := t.resolve("stat", name, false)
	if err != nil {
		return nil, err
	}
	fi, err := r.Stat(rel)
	return fi, named(err, name)
}

func (t *mountTable) lstat(name string) (os.FileInfo, error) {
	_, r, rel, err := t.resolve("lstat", name, false)
	if err != nil {
		return nil, err
	}
	fi, err := r.Lstat(rel)
	return fi, named(err, name)
}

// readDir is os.ReadDir: the entries sorted by name, and the ones read
// before an error.
func (t *mountTable) readDir(name string) ([]os.DirEntry, error) {
	f, err := t.open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dirs, err := f.ReadDir(-1)
	slices.SortFunc(dirs, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return dirs, err
}

// ioutilReadDir is ioutil.ReadDir.
func (t *mountTable) ioutilReadDir(name string) ([]fs.FileInfo, error) {
	dirs, err := t.readDir(name)
	if err != nil {
		return nil, err
	}
	infos := make([]fs.FileInfo, 0, len(dirs))
	for _, d := range dirs {
		fi, err := d.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, fi)
	}
	return infos, nil
}

func (t *mountTable) mkdir(name string, perm os.FileMode) error {
	_, r, rel, err := t.resolve("mkdir", name, true)
	if err != nil {
		return err
	}
	return named(r.Mkdir(rel, perm), name)
}

func (t *mountTable) mkdirAll(name string, perm os.FileMode) error {
	_, r, rel, err := t.resolve("mkdir", name, true)
	if err != nil {
		return err
	}
	return named(r.MkdirAll(rel, perm), name)
}

func (t *mountTable) remove(name string) error {
	_, r, rel, err := t.resolve("remove", name, true)
	if err != nil {
		return err
	}
	return named(r.Remove(rel), name)
}

func (t *mountTable) removeAll(name string) error {
	if name == "" {
		return nil // as os.RemoveAll
	}
	_, r, rel, err := t.resolve("unlinkat", name, true)
	if err != nil {
		return err
	}
	return named(r.RemoveAll(rel), name)
}

// pair resolves the two paths of a rename or a link: both must be in the
// same mount, which must be writable.
func (t *mountTable) pair(op, oldname, newname string) (*os.Root, string, string, error) {
	d1, r, rel1, err := t.resolve(op, oldname, true)
	if err == nil {
		var d2 *mountDir
		var rel2 string
		d2, _, rel2, err = t.resolve(op, newname, true)
		if err == nil && d1 != d2 {
			err = errCrossMount
		}
		if err == nil {
			return r, rel1, rel2, nil
		}
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return nil, "", "", &os.LinkError{Op: op, Old: oldname, New: newname, Err: err}
}

// linkNamed gives the error of an os.Root call with two paths the paths of
// the plugin.
func linkNamed(err error, oldname, newname string) error {
	var le *os.LinkError
	if errors.As(err, &le) {
		le.Old, le.New = oldname, newname
	}
	return named(err, newname)
}

func (t *mountTable) rename(oldname, newname string) error {
	r, rel1, rel2, err := t.pair("rename", oldname, newname)
	if err != nil {
		return err
	}
	return linkNamed(r.Rename(rel1, rel2), oldname, newname)
}

func (t *mountTable) link(oldname, newname string) error {
	r, rel1, rel2, err := t.pair("link", oldname, newname)
	if err != nil {
		return err
	}
	return linkNamed(r.Link(rel1, rel2), oldname, newname)
}

// symlink makes the link newname, whose text is oldname. The text is not a
// path of the plugin: os.Root follows the link inside the mount only.
func (t *mountTable) symlink(oldname, newname string) error {
	_, r, rel, err := t.resolve("symlink", newname, true)
	if err != nil {
		var pe *os.PathError
		errors.As(err, &pe)
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: pe.Err}
	}
	return linkNamed(r.Symlink(oldname, rel), oldname, newname)
}

func (t *mountTable) readlink(name string) (string, error) {
	_, r, rel, err := t.resolve("readlink", name, false)
	if err != nil {
		return "", err
	}
	s, err := r.Readlink(rel)
	return s, named(err, name)
}

func (t *mountTable) chmod(name string, mode os.FileMode) error {
	_, r, rel, err := t.resolve("chmod", name, true)
	if err != nil {
		return err
	}
	return named(r.Chmod(rel, mode), name)
}

func (t *mountTable) chown(name string, uid, gid int) error {
	_, r, rel, err := t.resolve("chown", name, true)
	if err != nil {
		return err
	}
	return named(r.Chown(rel, uid, gid), name)
}

func (t *mountTable) lchown(name string, uid, gid int) error {
	_, r, rel, err := t.resolve("lchown", name, true)
	if err != nil {
		return err
	}
	return named(r.Lchown(rel, uid, gid), name)
}

func (t *mountTable) chtimes(name string, atime, mtime time.Time) error {
	_, r, rel, err := t.resolve("chtimes", name, true)
	if err != nil {
		return err
	}
	return named(r.Chtimes(rel, atime, mtime), name)
}

func (t *mountTable) truncate(name string, size int64) error {
	f, err := t.openFile(name, os.O_WRONLY, 0)
	if err != nil {
		var pe *os.PathError
		if errors.As(err, &pe) {
			pe.Op = "truncate"
		}
		return err
	}
	err = f.Truncate(size)
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}

// tempPattern splits the pattern of CreateTemp and MkdirTemp at its last
// "*", as package os does.
func tempPattern(op, pattern string) (prefix, suffix string, err error) {
	for i := 0; i < len(pattern); i++ {
		if os.IsPathSeparator(pattern[i]) {
			return "", "", &os.PathError{Op: op, Path: pattern, Err: errors.New("pattern contains path separator")}
		}
	}
	if i := strings.LastIndexByte(pattern, '*'); i >= 0 {
		return pattern[:i], pattern[i+1:], nil
	}
	return pattern, "", nil
}

// tempName is the next name of CreateTemp and MkdirTemp.
func tempName(dir, prefix, suffix string) string {
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, prefix+strconv.FormatUint(uint64(rand.Uint32()), 10)+suffix)
}

func (t *mountTable) createTemp(dir, pattern string) (*os.File, error) {
	prefix, suffix, err := tempPattern("createtemp", pattern)
	if err != nil {
		return nil, err
	}
	for range 10000 {
		f, err := t.openFile(tempName(dir, prefix, suffix), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, &os.PathError{Op: "createtemp", Path: filepath.Join(dir, prefix+"*"+suffix), Err: fs.ErrExist}
}

func (t *mountTable) mkdirTemp(dir, pattern string) (string, error) {
	prefix, suffix, err := tempPattern("mkdirtemp", pattern)
	if err != nil {
		return "", err
	}
	for range 10000 {
		name := tempName(dir, prefix, suffix)
		err := t.mkdir(name, 0o700)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return name, nil
	}
	return "", &os.PathError{Op: "mkdirtemp", Path: filepath.Join(dir, prefix+"*"+suffix), Err: fs.ErrExist}
}

// dirFS is os.DirFS: the file system of the os.Root of the mount, below
// dir. A dir outside the mounts gives a file system whose every call
// fails.
func (t *mountTable) dirFS(dir string) fs.FS {
	_, r, rel, err := t.resolve("open", dir, false)
	if err != nil {
		return deniedFS{err}
	}
	if rel == "." {
		return r.FS()
	}
	sub, err := fs.Sub(r.FS(), filepath.ToSlash(rel))
	if err != nil {
		return deniedFS{err}
	}
	return sub
}

// deniedFS is the os.DirFS of a directory outside the mounts.
type deniedFS struct{ err error }

func (d deniedFS) Open(name string) (fs.File, error) {
	return nil, &os.PathError{Op: "open", Path: name, Err: d.unwrap()}
}

func (d deniedFS) unwrap() error {
	var pe *os.PathError
	if errors.As(d.err, &pe) {
		return pe.Err
	}
	return d.err
}

func (t *mountTable) openRoot(name string) (*os.Root, error) {
	_, r, rel, err := t.resolve("open", name, false)
	if err != nil {
		return nil, err
	}
	s, err := r.OpenRoot(rel)
	if err != nil {
		return nil, named(err, name)
	}
	if err := checkRoot(s); err != nil {
		s.Close()
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return s, nil
}

// walk is filepath.Walk on the mounts (the algorithm of path/filepath).
func (t *mountTable) walk(root string, fn filepath.WalkFunc) error {
	info, err := t.lstat(root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = t.walkRec(root, info, fn)
	}
	if err == filepath.SkipDir || err == filepath.SkipAll {
		return nil
	}
	return err
}

func (t *mountTable) walkRec(path string, info fs.FileInfo, fn filepath.WalkFunc) error {
	if !info.IsDir() {
		return fn(path, info, nil)
	}
	dirs, err := t.readDir(path)
	err1 := fn(path, info, err)
	if err != nil || err1 != nil {
		return err1
	}
	for _, d := range dirs {
		name := filepath.Join(path, d.Name())
		fi, err := t.lstat(name)
		if err != nil {
			if err := fn(name, fi, err); err != nil && err != filepath.SkipDir {
				return err
			}
			continue
		}
		if err := t.walkRec(name, fi, fn); err != nil && (!fi.IsDir() || err != filepath.SkipDir) {
			return err
		}
	}
	return nil
}

// walkDir is filepath.WalkDir on the mounts.
func (t *mountTable) walkDir(root string, fn fs.WalkDirFunc) error {
	info, err := t.lstat(root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = t.walkDirRec(root, fs.FileInfoToDirEntry(info), fn)
	}
	if err == filepath.SkipDir || err == filepath.SkipAll {
		return nil
	}
	return err
}

func (t *mountTable) walkDirRec(path string, d fs.DirEntry, fn fs.WalkDirFunc) error {
	if err := fn(path, d, nil); err != nil || !d.IsDir() {
		if err == filepath.SkipDir && d.IsDir() {
			err = nil
		}
		return err
	}
	dirs, err := t.readDir(path)
	if err != nil {
		if err = fn(path, d, err); err != nil {
			if err == filepath.SkipDir && d.IsDir() {
				err = nil
			}
			return err
		}
	}
	for _, d1 := range dirs {
		if err := t.walkDirRec(filepath.Join(path, d1.Name()), d1, fn); err != nil {
			if err == filepath.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}

// glob is filepath.Glob on the mounts. As filepath.Glob, it ignores the
// errors of the file system.
func (t *mountTable) glob(pattern string) ([]string, error) {
	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, err
	}
	if !hasGlobMeta(pattern) {
		if _, err := t.lstat(pattern); err != nil {
			return nil, nil
		}
		return []string{pattern}, nil
	}
	dir, file := filepath.Split(pattern)
	dir = globDir(dir)
	if !hasGlobMeta(dir) {
		return t.globIn(dir, file, nil)
	}
	if dir == pattern {
		return nil, filepath.ErrBadPattern
	}
	dirs, err := t.glob(dir)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, d := range dirs {
		if matches, err = t.globIn(d, file, matches); err != nil {
			return nil, err
		}
	}
	return matches, nil
}

// globDir is cleanGlobPath of path/filepath.
func globDir(dir string) string {
	switch dir {
	case "":
		return "."
	case string(filepath.Separator):
		return dir
	}
	return dir[:len(dir)-1]
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, `*?[\`)
}

func (t *mountTable) globIn(dir, pattern string, matches []string) ([]string, error) {
	fi, err := t.stat(dir)
	if err != nil || !fi.IsDir() {
		return matches, nil
	}
	dirs, _ := t.readDir(dir)
	for _, d := range dirs {
		ok, err := filepath.Match(pattern, d.Name())
		if err != nil {
			return matches, err
		}
		if ok {
			matches = append(matches, filepath.Join(dir, d.Name()))
		}
	}
	return matches, nil
}

// evalSymlinks is filepath.EvalSymlinks on the mounts: each link in a mount
// is read with Readlink of its os.Root. The elements of the path above the
// mounts are taken as they are (the plugin cannot reach them).
func (t *mountTable) evalSymlinks(name string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	vol := filepath.VolumeName(abs)
	sep := string(filepath.Separator)
	resolved := vol + sep
	rest := strings.TrimPrefix(abs[len(vol):], sep)
	links := 0
	for rest != "" {
		var elem string
		elem, rest, _ = strings.Cut(rest, sep)
		switch elem {
		case "", ".":
			continue
		case "..":
			resolved = filepath.Dir(resolved)
			continue
		}
		next := filepath.Join(resolved, elem)
		if _, _, ok := t.find(next); !ok {
			resolved = next
			continue
		}
		fi, err := t.lstat(next)
		if err != nil {
			return "", named(err, name)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			resolved = next
			continue
		}
		if links++; links > 255 {
			return "", &os.PathError{Op: "EvalSymlinks", Path: name, Err: errors.New("too many links")}
		}
		target, err := t.readlink(next)
		if err != nil {
			return "", named(err, name)
		}
		if filepath.IsAbs(target) {
			v := filepath.VolumeName(target)
			resolved = v + sep
			target = strings.TrimPrefix(target[len(v):], sep)
		}
		if rest == "" {
			rest = target
		} else {
			rest = target + sep + rest
		}
	}
	if !filepath.IsAbs(name) {
		// filepath.EvalSymlinks keeps a relative path relative.
		if wd, err := os.Getwd(); err == nil {
			if r, err := filepath.Rel(wd, resolved); err == nil {
				return r, nil
			}
		}
	}
	return filepath.Clean(resolved), nil
}

// parseFile is go/parser.ParseFile: without src, the file is read through
// the mounts.
func (t *mountTable) parseFile(fset *token.FileSet, filename string, src any, mode parser.Mode) (*ast.File, error) {
	if src == nil {
		b, err := t.readFile(filename)
		if err != nil {
			return nil, err
		}
		src = b
	}
	return parser.ParseFile(fset, filename, src, mode)
}

// httpDir is http.Dir on the mounts.
type mountHTTPDir struct {
	t   *mountTable
	dir string
}

func (d mountHTTPDir) Open(name string) (http.File, error) {
	full, err := httpDirPath(d.dir, name)
	if err != nil {
		return nil, err
	}
	f, err := d.t.open(full)
	if err != nil {
		return nil, mapOpenError(err, full, filepath.Separator, d.t.stat)
	}
	return f, nil
}

// mountHTTPDirFS is the fs.FS of http.ServeFile on the mounts.
type mountHTTPDirFS mountHTTPDir

func (d mountHTTPDirFS) Open(name string) (fs.File, error) {
	f, err := mountHTTPDir(d).Open(name)
	if err != nil {
		return nil, err
	}
	return f.(fs.File), nil
}

// installMounts installs the functions of the file on a restricted machine
// with Mounts. set does not replace a function that a Sandbox installed:
// the Sandbox calls the mounts itself (pathFuncs).
func (m *Machine) installMounts(set func(key string, fn any)) {
	t := m.mounts
	set("os.Open", t.open)
	set("os.OpenFile", t.openFile)
	set("os.Create", t.create)
	set("os.ReadFile", t.readFile)
	set("os.WriteFile", t.writeFile)
	set("os.Stat", t.stat)
	set("os.Lstat", t.lstat)
	set("os.ReadDir", t.readDir)
	set("os.Mkdir", t.mkdir)
	set("os.MkdirAll", t.mkdirAll)
	set("os.MkdirTemp", t.mkdirTemp)
	set("os.CreateTemp", t.createTemp)
	set("os.Remove", t.remove)
	set("os.RemoveAll", t.removeAll)
	set("os.Rename", t.rename)
	set("os.Link", t.link)
	set("os.Symlink", t.symlink)
	set("os.Readlink", t.readlink)
	set("os.Chmod", t.chmod)
	set("os.Chown", t.chown)
	set("os.Lchown", t.lchown)
	set("os.Chtimes", t.chtimes)
	set("os.Truncate", t.truncate)
	set("os.DirFS", t.dirFS)
	set("os.OpenRoot", t.openRoot)
	set("os.OpenInRoot", func(dir, name string) (*os.File, error) {
		r, err := t.openRoot(dir)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return r.Open(name)
	})
	set("io/ioutil.ReadFile", t.readFile)
	set("io/ioutil.WriteFile", t.writeFile)
	set("io/ioutil.ReadDir", t.ioutilReadDir)
	set("io/ioutil.TempFile", t.createTemp)
	set("io/ioutil.TempDir", t.mkdirTemp)
	set("path/filepath.Walk", t.walk)
	set("path/filepath.WalkDir", t.walkDir)
	set("path/filepath.Glob", t.glob)
	set("path/filepath.EvalSymlinks", t.evalSymlinks)
	set("go/parser.ParseFile", t.parseFile)
	set("net/http.FileServer", func(root http.FileSystem) http.Handler { return http.FileServer(m.ownHTTPDir(root)) })
	set("net/http.NewFileTransport", func(root http.FileSystem) http.RoundTripper {
		return http.NewFileTransport(m.ownHTTPDir(root))
	})
	set("net/http.ServeFile", func(w http.ResponseWriter, r *http.Request, name string) {
		dir, file := filepath.Split(name)
		http.ServeFileFS(w, r, mountHTTPDirFS{t, dir}, file)
	})
	m.installMountsUnix(set)
}

// pathFuncs are the functions on paths that a Sandbox checks (sandbox.go):
// on the mounts with Mounts, else the functions of a restricted machine
// without Mounts (openNoAlias, and os for the others).
type pathFuncs struct {
	openFile  func(string, int, os.FileMode) (*os.File, error)
	create    func(string) (*os.File, error)
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
	readDir   func(string) ([]os.DirEntry, error)
	stat      func(string) (os.FileInfo, error)
	lstat     func(string) (os.FileInfo, error)
	mkdir     func(string, os.FileMode) error
	mkdirAll  func(string, os.FileMode) error
	remove    func(string) error
	removeAll func(string) error
	rename    func(string, string) error
}

func (m *Machine) pathFuncs() pathFuncs {
	if t := m.mounts; t != nil {
		return pathFuncs{t.openFile, t.create, t.readFile, t.writeFile, t.readDir, t.stat, t.lstat,
			t.mkdir, t.mkdirAll, t.remove, t.removeAll, t.rename}
	}
	return pathFuncs{openFile, createFile, readFile, writeFile, os.ReadDir, os.Stat, os.Lstat,
		os.Mkdir, os.MkdirAll, os.Remove, os.RemoveAll, os.Rename}
}

// ownHTTPDir returns root, or the http.Dir of the plugin when root is an
// http.Dir: on the mounts, or with openNoAlias without them.
func (m *Machine) ownHTTPDir(root http.FileSystem) http.FileSystem {
	d, ok := root.(http.Dir)
	if !ok {
		return root
	}
	if m != nil && m.mounts != nil {
		return mountHTTPDir{m.mounts, string(d)}
	}
	return httpDir(d)
}
