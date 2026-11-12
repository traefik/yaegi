// Native boundary of os: program types given to os (docs/BOUNDARY.md).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
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

// tmo is an error with a Timeout method (value receiver).
type tmo struct{ t bool }

func (e tmo) Error() string { return fmt.Sprintf("tmo(%v)", e.t) }
func (e tmo) Timeout() bool { return e.t }

// ptmo is an error with a Timeout method (pointer receiver).
type ptmo struct{}

func (e *ptmo) Error() string { return "ptmo" }
func (e *ptmo) Timeout() bool { return true }

// gtmo is a generic error with a Timeout method.
type gtmo[T any] struct{ v T }

func (e gtmo[T]) Error() string { return fmt.Sprintf("gtmo(%v)", e.v) }
func (e gtmo[T]) Timeout() bool { return true }

// plain is an error without Timeout.
type plain struct{}

func (plain) Error() string { return "plain" }

// connReader is a reader with a SyscallConn method.
type connReader struct {
	s     string
	calls *int
}

func (r *connReader) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}
func (r *connReader) SyscallConn() (syscall.RawConn, error) {
	*r.calls++
	return nil, errors.New("no raw conn")
}

// plainReader is a reader without SyscallConn.
type plainReader struct{ s string }

func (r *plainReader) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

// connWriter is a writer with a SyscallConn method.
type connWriter struct {
	sb    strings.Builder
	calls *int
}

func (w *connWriter) Write(p []byte) (int, error) { return w.sb.Write(p) }
func (w *connWriter) SyscallConn() (syscall.RawConn, error) {
	*w.calls++
	return nil, errors.New("no raw conn")
}

// mkTree makes a temporary directory with some files.
func mkTree() string {
	dir, err := os.MkdirTemp("", "boundary-os")
	if err != nil {
		panic(err)
	}
	for _, n := range []string{"b.txt", "a.txt", "sub/c.txt", "sub/deep/d.txt"} {
		p := filepath.Join(dir, n)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(n), 0o644)
	}
	return dir
}

func main() {
	// row: os ReadDir,(*File).ReadDir,(*File).Readdir,(*File).Readdirnames assert x.(T), T unnamed
	// row: os ReadDir,(*File).ReadDir,(*File).Readdir,(*File).Readdirnames panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("readdir", func() {
		dir := mkTree()
		defer os.RemoveAll(dir)
		es, err := os.ReadDir(dir)
		for _, e := range es {
			out("ReadDir: %s %v", e.Name(), e.IsDir())
		}
		out("ReadDir err: %v", err)
		f, err := os.Open(dir)
		if err != nil {
			panic(err)
		}
		names, err := f.Readdirnames(-1)
		sort.Strings(names)
		out("Readdirnames: %v %v", names, err)
		f.Seek(0, io.SeekStart)
		infos, err := f.Readdir(-1)
		var in []string
		for _, i := range infos {
			in = append(in, fmt.Sprintf("%s:%v", i.Name(), i.IsDir()))
		}
		sort.Strings(in)
		out("Readdir: %v %v", in, err)
		f.Seek(0, io.SeekStart)
		ents, err := f.ReadDir(1)
		out("File.ReadDir(1): %d %v", len(ents), err)
		f.Close()
		_, err = os.ReadDir(filepath.Join(dir, "missing"))
		out("missing: %v", errors.Is(err, os.ErrNotExist))
	})
	// row: os Chdir,Getwd,(*File).Chdir assert x.(T), T unnamed
	// row: os Chdir,Getwd,(*File).Chdir panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("Chdir", func() {
		dir := mkTree()
		defer os.RemoveAll(dir)
		real, _ := filepath.EvalSymlinks(dir)
		old, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		defer os.Chdir(old)
		out("Chdir: %v", os.Chdir(filepath.Join(dir, "sub")))
		wd, err := os.Getwd()
		out("Getwd: %v %v", wd == filepath.Join(real, "sub") || wd == filepath.Join(dir, "sub"), err)
		f, err := os.Open(filepath.Join(dir, "sub", "deep"))
		if err != nil {
			panic(err)
		}
		out("File.Chdir: %v", f.Chdir())
		f.Close()
		wd, err = os.Getwd()
		out("Getwd: %v %v", filepath.Base(wd), err)
		out("Chdir missing: %v", os.Chdir(filepath.Join(dir, "missing")) != nil)
	})
	// row: os RemoveAll,(*Root).RemoveAll assert x.(T), T unnamed
	// row: os RemoveAll,(*Root).RemoveAll panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("RemoveAll", func() {
		dir := mkTree()
		defer os.RemoveAll(dir)
		root, err := os.OpenRoot(dir)
		if err != nil {
			panic(err)
		}
		out("Root.RemoveAll: %v", root.RemoveAll("sub"))
		root.Close()
		_, err = os.Stat(filepath.Join(dir, "sub"))
		out("sub gone: %v", errors.Is(err, os.ErrNotExist))
		out("RemoveAll: %v", os.RemoveAll(dir))
		_, err = os.Stat(dir)
		out("dir gone: %v", errors.Is(err, os.ErrNotExist))
	})
	// row: os (*File).ReadFrom,(*File).WriteTo assert x.(syscall.Conn)
	// row: os (*File).ReadFrom,(*File).WriteTo assert x.(interface{Network() internal/poll.String; PollFD() *internal/poll.FD})
	probe("ReadFrom/WriteTo", func() {
		dir := mkTree()
		defer os.RemoveAll(dir)
		f, err := os.Create(filepath.Join(dir, "out.txt"))
		if err != nil {
			panic(err)
		}
		calls := 0
		n, err := f.ReadFrom(&connReader{"conn data ", &calls})
		out("ReadFrom conn: %d %v", n, err)
		n, err = f.ReadFrom(&plainReader{"plain data"})
		out("ReadFrom plain: %d %v", n, err)
		n, err = io.Copy(f, &connReader{" copy", &calls})
		out("Copy conn: %d %v", n, err)
		f.Close()
		f, err = os.Open(filepath.Join(dir, "out.txt"))
		if err != nil {
			panic(err)
		}
		w := &connWriter{calls: &calls}
		n, err = f.WriteTo(w)
		out("WriteTo conn: %d %v %q", n, err, w.sb.String())
		f.Close()
		f, _ = os.Open(filepath.Join(dir, "a.txt"))
		var sb strings.Builder
		n, err = f.WriteTo(&sb)
		out("WriteTo builder: %d %v %q", n, err, sb.String())
		f.Close()
		_ = calls // whether SyscallConn is called depends on the system
	})
	// row: os IsTimeout assert x.(os.timeout)
	// row: os (*SyscallError).Timeout assert x.(os.timeout)
	probe("Timeout", func() {
		for _, e := range []error{tmo{true}, tmo{false}, &ptmo{}, gtmo[int]{1}, plain{}, os.ErrDeadlineExceeded} {
			se := os.NewSyscallError("call", e).(*os.SyscallError)
			pe := &os.PathError{Op: "open", Path: "p", Err: e}
			le := &os.LinkError{Op: "link", Old: "a", New: "b", Err: e}
			out("%v: IsTimeout %v, SyscallError.Timeout %v, IsTimeout(SyscallError) %v, PathError %v, LinkError %v, wrapped %v",
				e, os.IsTimeout(e), se.Timeout(), os.IsTimeout(se), os.IsTimeout(pe), os.IsTimeout(le), os.IsTimeout(fmt.Errorf("w: %w", e)))
		}
		out("nil: %v", os.IsTimeout(nil))
	})
}
