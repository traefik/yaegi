// Native boundary of archive/tar: program readers, fs.FileInfo and fs.FS
// types given to archive/tar (docs/BOUNDARY.md).
package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

var mtime = time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

// seekReader is a reader with a working Seek (pointer receiver).
type seekReader struct {
	r     *bytes.Reader
	seeks []string
}

func (s *seekReader) Read(p []byte) (int, error) { return s.r.Read(p) }
func (s *seekReader) Seek(off int64, whence int) (int64, error) {
	s.seeks = append(s.seeks, fmt.Sprintf("%d/%d", off, whence))
	return s.r.Seek(off, whence)
}

// badSeekReader has a Seek that always fails, as os.Stdin.
type badSeekReader struct {
	r     *bytes.Reader
	seeks int
}

func (s *badSeekReader) Read(p []byte) (int, error) { return s.r.Read(p) }
func (s *badSeekReader) Seek(off int64, whence int) (int64, error) {
	s.seeks++
	return 0, errors.New("cannot seek")
}

// genSeekReader is a generic reader with a working Seek.
type genSeekReader[T any] struct{ s *seekReader }

func (g genSeekReader[T]) Read(p []byte) (int, error) { return g.s.Read(p) }
func (g genSeekReader[T]) Seek(off int64, whence int) (int64, error) {
	return g.s.Seek(off, whence)
}

// onlyReader has no Seek.
type onlyReader struct{ r *bytes.Reader }

func (o *onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

// info is a fs.FileInfo with Uname and Gname (value receivers): it is a
// tar.FileInfoNames.
type info struct {
	name string
	size int64
	mode fs.FileMode
}

func (i info) Name() string           { return i.name }
func (i info) Size() int64            { return i.size }
func (i info) Mode() fs.FileMode      { return i.mode }
func (i info) ModTime() time.Time     { return mtime }
func (i info) IsDir() bool            { return i.mode.IsDir() }
func (i info) Sys() any               { return nil }
func (i info) Uname() (string, error) { return "user-" + i.name, nil }
func (i info) Gname() (string, error) { return "group-" + i.name, nil }
func (i info) String() string         { return "info:" + i.name }

// pinfo is a fs.FileInfo with Uname and Gname (pointer receivers).
type pinfo struct{ info }

func (p *pinfo) Uname() (string, error) { return "puser", nil }
func (p *pinfo) Gname() (string, error) { return "", errors.New("no group") }

// ginfo is a generic fs.FileInfo with Uname and Gname.
type ginfo[T any] struct {
	info
	id T
}

func (g ginfo[T]) Uname() (string, error) { return fmt.Sprintf("guser%v", g.id), nil }
func (g ginfo[T]) Gname() (string, error) { return fmt.Sprintf("ggroup%v", g.id), nil }

// plainInfo is a fs.FileInfo without Uname and Gname.
type plainInfo struct{ name string }

func (i plainInfo) Name() string       { return i.name }
func (i plainInfo) Size() int64        { return 3 }
func (i plainInfo) Mode() fs.FileMode  { return 0o640 }
func (i plainInfo) ModTime() time.Time { return mtime }
func (i plainInfo) IsDir() bool        { return false }
func (i plainInfo) Sys() any           { return nil }

// memFS is a program fs.FS (with ReadDir) of files at its root; its
// fs.DirEntry gives program fs.FileInfo values.
type memFS map[string]string

type memFile struct {
	r  *strings.Reader
	fi fs.FileInfo
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.fi, nil }
func (f *memFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *memFile) Close() error               { return nil }

type memEntry struct{ fi fs.FileInfo }

func (e memEntry) Name() string               { return e.fi.Name() }
func (e memEntry) IsDir() bool                { return e.fi.IsDir() }
func (e memEntry) Type() fs.FileMode          { return e.fi.Mode().Type() }
func (e memEntry) Info() (fs.FileInfo, error) { return e.fi, nil }

func (m memFS) stat(name string) fs.FileInfo {
	if name == "." {
		return info{name: ".", mode: fs.ModeDir | 0o755}
	}
	switch {
	case strings.HasPrefix(name, "p"):
		return &pinfo{info{name: name, size: int64(len(m[name])), mode: 0o600}}
	case strings.HasPrefix(name, "g"):
		return ginfo[int]{info{name: name, size: int64(len(m[name])), mode: 0o600}, 7}
	case strings.HasPrefix(name, "n"):
		return plainInfo{name}
	}
	return info{name: name, size: int64(len(m[name])), mode: 0o644}
}

func (m memFS) Open(name string) (fs.File, error) {
	if _, ok := m[name]; !ok && name != "." {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &memFile{strings.NewReader(m[name]), m.stat(name)}, nil
}

func (m memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	var names []string
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	var es []fs.DirEntry
	for _, n := range names {
		es = append(es, memEntry{m.stat(n)})
	}
	return es, nil
}

// archive makes a tar of two big files and a small one.
func archive() []byte {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for _, f := range []struct {
		name string
		size int
	}{{"big1", 3000}, {"big2", 1500}, {"small", 5}} {
		tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(f.size), ModTime: mtime, Format: tar.FormatUSTAR})
		tw.Write(bytes.Repeat([]byte{'x'}, f.size))
	}
	tw.Close()
	return b.Bytes()
}

func listTar(r io.Reader) string {
	tr := tar.NewReader(r)
	var names []string
	for {
		h, err := tr.Next()
		if err != nil {
			if err != io.EOF {
				names = append(names, "err:"+err.Error())
			}
			break
		}
		names = append(names, fmt.Sprintf("%s(%d)", h.Name, h.Size))
	}
	return strings.Join(names, " ")
}

func main() {
	// row: archive/tar (*Reader).Next assert x.(io.Seeker)
	probe("Next/Seeker", func() {
		data := archive()
		s := &seekReader{r: bytes.NewReader(data)}
		out("seekReader: %s seeks=%v", listTar(s), s.seeks)
		bs := &badSeekReader{r: bytes.NewReader(data)}
		out("badSeekReader: %s seeks=%d", listTar(bs), bs.seeks)
		g := genSeekReader[string]{&seekReader{r: bytes.NewReader(data)}}
		out("generic: %s seeks=%v", listTar(g), g.s.seeks)
		out("onlyReader: %s", listTar(&onlyReader{bytes.NewReader(data)}))
		var r io.Reader = &seekReader{r: bytes.NewReader(data)}
		out("iface: %s seeks=%v", listTar(r), r.(*seekReader).seeks)
	})
	// row: archive/tar FileInfoHeader assert x.(archive/tar.FileInfoNames)
	probe("FileInfoHeader", func() {
		fis := []fs.FileInfo{
			info{name: "v", size: 4, mode: 0o644},
			&pinfo{info{name: "p", size: 1, mode: 0o600}},
			ginfo[string]{info{name: "g", mode: fs.ModeDir | 0o755}, "x"},
			plainInfo{"n"},
		}
		var fi fs.FileInfo = info{name: "iface", mode: 0o644}
		fis = append(fis, fi)
		for i, fi := range fis {
			h, err := tar.FileInfoHeader(fi, "")
			if err != nil {
				out("%d: %v", i, err)
				continue
			}
			out("%d: %q %c %o %d uname=%q gname=%q", i, h.Name, h.Typeflag, h.Mode, h.Size, h.Uname, h.Gname)
		}
	})
	// row: archive/tar (*Writer).WriteHeader reflect comparability
	probe("WriteHeader/XGlobalHeader", func() {
		hs := []*tar.Header{
			{Typeflag: tar.TypeXGlobalHeader, Name: "global", PAXRecords: map[string]string{"comment": "hi"}},
			{Typeflag: tar.TypeXGlobalHeader, Name: "global", PAXRecords: map[string]string{"comment": "hi"}, Uname: "bob"},
			{Typeflag: tar.TypeXGlobalHeader, Name: "global", PAXRecords: map[string]string{}, Format: tar.FormatPAX},
			{Typeflag: tar.TypeXGlobalHeader, Name: "global", ModTime: mtime},
		}
		for i, h := range hs {
			var b bytes.Buffer
			tw := tar.NewWriter(&b)
			err := tw.WriteHeader(h)
			tw.Close()
			out("%d: %v %d", i, err, b.Len())
		}
	})
	// row: archive/tar (*Writer).AddFS assert x.(archive/tar.FileInfoNames)
	// row: archive/tar (*Writer).AddFS reflect comparability
	probe("AddFS", func() {
		fsys := memFS{"a.txt": "aaa", "pfile": "pp", "gfile": "gg", "nfile": "nnn"}
		var b bytes.Buffer
		tw := tar.NewWriter(&b)
		err := tw.AddFS(fsys)
		tw.Close()
		out("AddFS: %v", err)
		tr := tar.NewReader(&b)
		for {
			h, err := tr.Next()
			if err != nil {
				out("end: %v", err)
				break
			}
			body, _ := io.ReadAll(tr)
			out("%q %o uname=%q gname=%q %q %v", h.Name, h.Mode, h.Uname, h.Gname, body, h.ModTime.UTC())
		}
	})
}
