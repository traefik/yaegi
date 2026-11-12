// Native boundary of io/fs: program types given to io/fs (docs/BOUNDARY.md).
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// info is the FileInfo of the program file systems.
type info struct {
	name string
	size int64
	dir  bool
	link bool
}

func (i info) Name() string { return i.name }
func (i info) Size() int64  { return i.size }
func (i info) Mode() fs.FileMode {
	switch {
	case i.dir:
		return fs.ModeDir | 0o755
	case i.link:
		return fs.ModeSymlink | 0o777
	}
	return 0o644
}
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) IsDir() bool        { return i.dir }
func (i info) Sys() any           { return nil }

// tree is the content of a file system: file path -> content.
// A content that starts with "->" is a symbolic link.
type tree map[string]string

func (t tree) isDir(name string) bool {
	if name == "." {
		return true
	}
	for p := range t {
		if strings.HasPrefix(p, name+"/") {
			return true
		}
	}
	return false
}

func (t tree) stat(name string) (info, bool) {
	if c, ok := t[name]; ok {
		return info{name: path.Base(name), size: int64(len(c)), link: strings.HasPrefix(c, "->")}, true
	}
	if t.isDir(name) {
		return info{name: path.Base(name), dir: true}, true
	}
	return info{}, false
}

func (t tree) entries(name string) []fs.DirEntry {
	seen := map[string]bool{}
	var names []string
	for p := range t {
		rel := p
		if name != "." {
			if !strings.HasPrefix(p, name+"/") {
				continue
			}
			rel = p[len(name)+1:]
		}
		first, _, _ := strings.Cut(rel, "/")
		if !seen[first] {
			seen[first] = true
			names = append(names, first)
		}
	}
	sort.Strings(names)
	var es []fs.DirEntry
	for _, n := range names {
		i, _ := t.stat(path.Join(name, n))
		es = append(es, fs.FileInfoToDirEntry(i))
	}
	return es
}

// file is an open regular file.
type file struct {
	i info
	r *strings.Reader
}

func (f *file) Stat() (fs.FileInfo, error) { return f.i, nil }
func (f *file) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *file) Close() error               { return nil }

// dirFile is an open directory with ReadDir.
type dirFile struct {
	i    info
	ents []fs.DirEntry
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return d.i, nil }
func (d *dirFile) Read([]byte) (int, error)   { return 0, errors.New("is a directory") }
func (d *dirFile) Close() error               { return nil }
func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		e := d.ents
		d.ents = nil
		return e, nil
	}
	if len(d.ents) == 0 {
		return nil, io.EOF
	}
	if n > len(d.ents) {
		n = len(d.ents)
	}
	e := d.ents[:n]
	d.ents = d.ents[n:]
	return e, nil
}

// noReadDir is an open directory without ReadDir.
type noReadDir struct{ i info }

func (d noReadDir) Stat() (fs.FileInfo, error) { return d.i, nil }
func (d noReadDir) Read([]byte) (int, error)   { return 0, errors.New("is a directory") }
func (d noReadDir) Close() error               { return nil }

// openFS has only Open; its directories have ReadDir.
type openFS struct {
	t   tree
	log *[]string
}

func (f openFS) Open(name string) (fs.File, error) {
	*f.log = append(*f.log, "Open "+name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	i, ok := f.t.stat(name)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if i.dir {
		return &dirFile{i, f.t.entries(name)}, nil
	}
	return &file{i, strings.NewReader(f.t[name])}, nil
}

// bareFS has only Open; its directories have no ReadDir.
type bareFS struct{ openFS }

func (f bareFS) Open(name string) (fs.File, error) {
	file, err := f.openFS.Open(name)
	if d, ok := file.(*dirFile); ok {
		return noReadDir{d.i}, nil
	}
	return file, err
}

// fullFS has every optional method of io/fs (pointer receivers).
type fullFS struct{ openFS }

func (f *fullFS) ReadDir(name string) ([]fs.DirEntry, error) {
	*f.log = append(*f.log, "ReadDir "+name)
	if !f.t.isDir(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return f.t.entries(name), nil
}
func (f *fullFS) ReadFile(name string) ([]byte, error) {
	*f.log = append(*f.log, "ReadFile "+name)
	c, ok := f.t[name]
	if !ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	return []byte("RF:" + c), nil
}
func (f *fullFS) Stat(name string) (fs.FileInfo, error) {
	*f.log = append(*f.log, "Stat "+name)
	i, ok := f.t.stat(name)
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return i, nil
}
func (f *fullFS) Glob(pattern string) ([]string, error) {
	*f.log = append(*f.log, "Glob "+pattern)
	var m []string
	for p := range f.t {
		if ok, _ := path.Match(pattern, p); ok {
			m = append(m, p)
		}
	}
	sort.Strings(m)
	return m, nil
}
func (f *fullFS) Sub(dir string) (fs.FS, error) {
	*f.log = append(*f.log, "Sub "+dir)
	t := tree{}
	for p, c := range f.t {
		if strings.HasPrefix(p, dir+"/") {
			t[p[len(dir)+1:]] = c
		}
	}
	return openFS{t, f.log}, nil
}
func (f *fullFS) ReadLink(name string) (string, error) {
	*f.log = append(*f.log, "ReadLink "+name)
	c := f.t[name]
	if !strings.HasPrefix(c, "->") {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	return c[2:], nil
}
func (f *fullFS) Lstat(name string) (fs.FileInfo, error) {
	*f.log = append(*f.log, "Lstat "+name)
	return f.Stat(name)
}

// statFS has Stat and ReadDir with value receivers, and no Glob.
type statFS struct{ openFS }

func (f statFS) Stat(name string) (fs.FileInfo, error) {
	*f.log = append(*f.log, "vStat "+name)
	return (&fullFS{f.openFS}).Stat(name)
}
func (f statFS) ReadDir(name string) ([]fs.DirEntry, error) {
	*f.log = append(*f.log, "vReadDir "+name)
	return (&fullFS{f.openFS}).ReadDir(name)
}

// genFS is a generic file system with ReadFile (value receiver).
type genFS[T any] struct {
	openFS
	tag T
}

func (f genFS[T]) ReadFile(name string) ([]byte, error) {
	*f.log = append(*f.log, "genReadFile "+name)
	return []byte(fmt.Sprintf("gen(%v):%s", f.tag, f.t[name])), nil
}

var content = tree{
	"a.txt":         "A",
	"b.md":          "BB",
	"dir/c.txt":     "CCC",
	"dir/sub/d.txt": "DDDD",
	"link":          "->a.txt",
}

// systems returns a fresh value of each program file system.
func systems() []struct {
	name string
	fsys fs.FS
	log  *[]string
} {
	var res []struct {
		name string
		fsys fs.FS
		log  *[]string
	}
	add := func(name string, mk func(o openFS) fs.FS) {
		log := &[]string{}
		res = append(res, struct {
			name string
			fsys fs.FS
			log  *[]string
		}{name, mk(openFS{content, log}), log})
	}
	add("open", func(o openFS) fs.FS { return o })
	add("bare", func(o openFS) fs.FS { return bareFS{o} })
	add("full", func(o openFS) fs.FS { return &fullFS{o} })
	add("stat", func(o openFS) fs.FS { return statFS{o} })
	add("gen", func(o openFS) fs.FS { return genFS[int]{o, 7} })
	return res
}

// show prints the result of one call and the methods it used.
func show(name, call string, log *[]string, res any, err error) {
	out("%s %s: %v err=%v calls=%v", name, call, res, err, *log)
	*log = (*log)[:0]
}

func names(es []fs.DirEntry) []string {
	var n []string
	for _, e := range es {
		n = append(n, fmt.Sprintf("%s:%v", e.Name(), e.IsDir()))
	}
	return n
}

func main() {
	// row: io/fs ReadDir assert x.(io/fs.ReadDirFS)
	// row: io/fs ReadDir assert x.(io/fs.ReadDirFile)
	probe("ReadDir", func() {
		for _, s := range systems() {
			es, err := fs.ReadDir(s.fsys, "dir")
			show(s.name, "ReadDir dir", s.log, names(es), err)
			es, err = fs.ReadDir(s.fsys, ".")
			show(s.name, "ReadDir .", s.log, names(es), err)
			_, err = fs.ReadDir(s.fsys, "nope")
			show(s.name, "ReadDir nope", s.log, nil, err)
		}
	})
	// row: io/fs ReadFile assert x.(io/fs.ReadFileFS)
	probe("ReadFile", func() {
		for _, s := range systems() {
			b, err := fs.ReadFile(s.fsys, "dir/c.txt")
			show(s.name, "ReadFile", s.log, string(b), err)
		}
	})
	// row: io/fs Stat assert x.(io/fs.StatFS)
	probe("Stat", func() {
		for _, s := range systems() {
			i, err := fs.Stat(s.fsys, "b.md")
			show(s.name, "Stat", s.log, fmt.Sprintf("%s %d %v", i.Name(), i.Size(), i.Mode()), err)
			_, err = fs.Stat(s.fsys, "nope")
			show(s.name, "Stat nope", s.log, nil, err)
		}
	})
	// row: io/fs Sub assert x.(io/fs.SubFS)
	probe("Sub", func() {
		for _, s := range systems() {
			sub, err := fs.Sub(s.fsys, "dir")
			show(s.name, "Sub", s.log, fmt.Sprintf("%T", sub) != "", err)
			b, err := fs.ReadFile(sub, "sub/d.txt")
			show(s.name, "Sub ReadFile", s.log, string(b), err)
			_, err = fs.Sub(s.fsys, "../x")
			show(s.name, "Sub bad", s.log, nil, err)
		}
	})
	// row: io/fs ReadLink assert x.(io/fs.ReadLinkFS)
	probe("ReadLink", func() {
		for _, s := range systems() {
			l, err := fs.ReadLink(s.fsys, "link")
			show(s.name, "ReadLink", s.log, l, err)
		}
	})
	// row: io/fs Lstat assert x.(io/fs.ReadLinkFS)
	// row: io/fs Lstat assert x.(io/fs.StatFS)
	probe("Lstat", func() {
		for _, s := range systems() {
			i, err := fs.Lstat(s.fsys, "link")
			show(s.name, "Lstat", s.log, fmt.Sprintf("%s %v", i.Name(), i.Mode()), err)
		}
	})
	// row: io/fs Glob assert *
	probe("Glob", func() {
		for _, s := range systems() {
			m, err := fs.Glob(s.fsys, "*.txt")
			show(s.name, "Glob *.txt", s.log, m, err)
			m, err = fs.Glob(s.fsys, "dir/*/*.txt")
			show(s.name, "Glob dir/*/*.txt", s.log, m, err)
			m, err = fs.Glob(s.fsys, "dir/c.txt")
			show(s.name, "Glob dir/c.txt", s.log, m, err)
			m, err = fs.Glob(s.fsys, "[")
			show(s.name, "Glob [", s.log, m, err)
		}
	})
	// row: io/fs WalkDir assert *
	probe("WalkDir", func() {
		for _, s := range systems() {
			var seen []string
			err := fs.WalkDir(s.fsys, ".", func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					seen = append(seen, p+":err:"+err.Error())
					return nil
				}
				seen = append(seen, fmt.Sprintf("%s:%v", p, d.IsDir()))
				return nil
			})
			show(s.name, "WalkDir", s.log, seen, err)
			err = fs.WalkDir(s.fsys, "nope", func(p string, d fs.DirEntry, err error) error {
				out("%s WalkDir nope: %s %v", s.name, p, err)
				return err
			})
			show(s.name, "WalkDir nope", s.log, nil, err)
		}
	})
	// row: io/fs (*PathError).Timeout assert x.(interface{Timeout() bool})
	probe("PathError.Timeout", func() {
		for _, e := range []error{tmo{true}, tmo{false}, &ptmo{}, plainErr{}} {
			pe := &fs.PathError{Op: "read", Path: "p", Err: e}
			out("%v: %v", pe, pe.Timeout())
		}
	})
}

// tmo is an error with a Timeout method (value receiver).
type tmo struct{ t bool }

func (e tmo) Error() string { return fmt.Sprintf("tmo(%v)", e.t) }
func (e tmo) Timeout() bool { return e.t }

// ptmo is an error with a Timeout method (pointer receiver).
type ptmo struct{}

func (e *ptmo) Error() string { return "ptmo" }
func (e *ptmo) Timeout() bool { return true }

// plainErr is an error without Timeout.
type plainErr struct{}

func (plainErr) Error() string { return "plain" }
