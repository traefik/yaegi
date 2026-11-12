//go:build linux || darwin || freebsd || netbsd || openbsd

package vm

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// fdTree makes the tree of TestOpenNoAlias in dir: a file, a directory, and
// links to them, dangling, relative, absolute and in a loop.
func fdTree(t *testing.T, dir string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "f"), []byte("data"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "d", "sub", "g"), []byte("sub data"), 0o644))
	must(os.Symlink("f", filepath.Join(dir, "lf")))
	must(os.Symlink("d", filepath.Join(dir, "ld")))
	must(os.Symlink("nothing", filepath.Join(dir, "dangle")))
	must(os.Symlink("loop2", filepath.Join(dir, "loop1")))
	must(os.Symlink("loop1", filepath.Join(dir, "loop2")))
	must(os.Symlink("d/../f", filepath.Join(dir, "lrel")))
	must(os.Symlink("sub/g", filepath.Join(dir, "d", "lsub")))
	must(os.Symlink(filepath.Join(dir, "f"), filepath.Join(dir, "labs")))
	must(os.Symlink(filepath.Join(dir, "d")+"/", filepath.Join(dir, "ldslash")))
}

// TestOpenNoAlias checks that openNoAlias (hostfd_walk.go) gives the result
// of os.OpenFile for ordinary paths: the same error (text included), the
// same file, the same mode of a created file, the same append mode and the
// same poller behaviour of a FIFO. Each case runs on its own copy of the
// tree, with the working directory in it, so that relative paths and the
// files that a case creates are the same for both.
func TestOpenNoAlias(t *testing.T) {
	cases := []struct {
		name string
		flag int
		perm os.FileMode
	}{
		{"f", os.O_RDONLY, 0},
		{"./f", os.O_RDONLY, 0},
		{"d/../f", os.O_RDONLY, 0},
		{"d//sub//g", os.O_RDONLY, 0},
		{"d/", os.O_RDONLY, 0},
		{"d/.", os.O_RDONLY, 0},
		{"d/..", os.O_RDONLY, 0},
		{"d", os.O_RDONLY, 0},
		{"d", os.O_WRONLY, 0},   // EISDIR
		{"f/", os.O_RDONLY, 0},  // ENOTDIR
		{"f/x", os.O_RDONLY, 0}, // ENOTDIR
		{"lf/", os.O_RDONLY, 0}, // ENOTDIR
		{"", os.O_RDONLY, 0},
		{".", os.O_RDONLY, 0},
		{"..", os.O_RDONLY, 0},
		{"/", os.O_RDONLY, 0},
		{"nothing", os.O_RDONLY, 0},
		{"nothing/x", os.O_RDONLY, 0},
		{"f\x00", os.O_RDONLY, 0},
		{"lf", os.O_RDONLY, 0},
		{"ld", os.O_RDONLY, 0},
		{"ld/", os.O_RDONLY, 0},
		{"ld/sub/g", os.O_RDONLY, 0},
		{"d/lsub", os.O_RDONLY, 0},
		{"lrel", os.O_RDONLY, 0},
		{"labs", os.O_RDONLY, 0},
		{"ldslash", os.O_RDONLY, 0},
		{"ldslash/sub/g", os.O_RDONLY, 0},
		{"dangle", os.O_RDONLY, 0},
		{"dangle", os.O_WRONLY | os.O_CREATE, 0o644}, // creates "nothing"
		{"dangle", os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0o644},
		{"loop1", os.O_RDONLY, 0},
		{"loop1", os.O_WRONLY | os.O_CREATE, 0o644},
		{"f", os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0o644},
		{"lf", os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0o644},
		{"new", os.O_WRONLY | os.O_CREATE, 0o640},
		{"new", os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0o600 | os.ModeSticky},
		{"new/", os.O_WRONLY | os.O_CREATE, 0o644},
		{"d/new", os.O_RDWR | os.O_CREATE | os.O_TRUNC, 0o644},
		{"f", os.O_WRONLY | os.O_TRUNC, 0},
		{"f", os.O_WRONLY | os.O_APPEND, 0},
		{"lf", os.O_RDWR | os.O_APPEND, 0},
	}
	type result struct {
		err  string
		data string
		mode os.FileMode
		// after an open for writing: the content of the tree
		tree string
	}
	// run opens name in a fresh tree with open, writes "+" when the file is
	// writable, and returns what it saw.
	run := func(t *testing.T, open func(string, int, os.FileMode) (*os.File, error), name string, flag int, perm os.FileMode) result {
		t.Helper()
		dir := t.TempDir()
		fdTree(t, dir)
		t.Chdir(dir)
		var r result
		f, err := open(name, flag, perm)
		if err != nil {
			r.err = strings.ReplaceAll(err.Error(), dir, "DIR")
			// The errors of os.OpenFile are *PathError with the name given.
			var pe *os.PathError
			if !errors.As(err, &pe) || pe.Path != name {
				t.Errorf("%q: error %#v is not a PathError with the name", name, err)
			}
			return r
		}
		defer f.Close()
		if f.Name() != name {
			t.Errorf("%q: name %q", name, f.Name())
		}
		fi, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		r.mode = fi.Mode()
		if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
			if _, err := f.Write([]byte("+")); err != nil {
				r.err = "write: " + err.Error()
			}
			if _, err := f.WriteAt([]byte("@"), 0); err != nil {
				r.err += " writeat: " + err.Error()
			}
		}
		if flag&os.O_WRONLY == 0 && !fi.IsDir() {
			b, err := io.ReadAll(f)
			if err != nil {
				t.Fatal(err)
			}
			r.data = string(b)
		}
		if fi.IsDir() && name != ".." { // the parent of the tree differs
			names, err := f.Readdirnames(-1)
			if err != nil {
				t.Fatal(err)
			}
			r.data = strings.Join(names, " ")
		}
		var sb strings.Builder
		filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, p)
			if fi.Mode().IsRegular() {
				b, _ := os.ReadFile(p)
				sb.WriteString(rel + ":" + string(b) + ":" + fi.Mode().String() + "\n")
			} else {
				sb.WriteString(rel + ":" + fi.Mode().String() + "\n")
			}
			return nil
		})
		r.tree = sb.String()
		return r
	}
	for _, c := range cases {
		t.Run(strings.ReplaceAll(c.name, "/", "|")+"/"+os.FileMode(c.flag).String(), func(t *testing.T) {
			want := run(t, os.OpenFile, c.name, c.flag, c.perm)
			got := run(t, openNoAlias, c.name, c.flag, c.perm)
			if got != want {
				t.Errorf("openNoAlias(%q, %#x, %v):\n got %+v\nwant %+v", c.name, c.flag, c.perm, got, want)
			}
		})
	}
}

// TestOpenNoAliasFIFO checks that a FIFO opened by openNoAlias goes to the
// poller of Go when os.OpenFile's does: a read deadline works, or is refused,
// the same way.
func TestOpenNoAliasFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := func(open func(string, int, os.FileMode) (*os.File, error)) string {
		f, err := open(fifo, os.O_RDWR, 0) // O_RDWR never blocks on a FIFO
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
			return "deadline: " + err.Error()
		}
		_, err = f.Read(make([]byte, 1))
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			return "read: " + err.Error()
		}
		return "timeout"
	}
	if got, want := deadline(openNoAlias), deadline(os.OpenFile); got != want {
		t.Errorf("FIFO: openNoAlias %q, os.OpenFile %q", got, want)
	}
}

// TestOpenNoAliasFDs checks that openNoAlias leaks no descriptor: after an
// error, after a link, and after the file is closed, the process has the
// same descriptors as before.
func TestOpenNoAliasFDs(t *testing.T) {
	dir := t.TempDir()
	fdTree(t, dir)
	openFDs := func() string {
		names, err := os.ReadDir("/dev/fd")
		if err != nil {
			t.Skip("no /dev/fd:", err)
		}
		var s []string
		for _, n := range names {
			s = append(s, n.Name())
		}
		return strings.Join(s, " ")
	}
	before := openFDs()
	for _, name := range []string{"f", "lf", "lrel", "labs", "ldslash/sub/g", "d/", "loop1", "f/x", "nothing", "dangle"} {
		f, err := openNoAlias(filepath.Join(dir, name), os.O_RDONLY, 0)
		if err == nil {
			f.Close()
		}
		if after := openFDs(); after != before {
			t.Errorf("%s: descriptors %q, before %q", name, after, before)
		}
	}
}

// TestOpenNoAliasLoop checks the limit of links of openNoAlias: a chain of
// maxLinks links opens, one more is ELOOP, as for os.OpenFile.
func TestOpenNoAliasLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := "f"
	for i := 1; i <= maxLinks+1; i++ {
		name := "l" + string(rune('0'+i%10)) + strings.Repeat("x", i/10)
		if err := os.Symlink(prev, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		prev = name
		f, err := openNoAlias(filepath.Join(dir, name), os.O_RDONLY, 0)
		if err == nil {
			f.Close()
		}
		g, gerr := os.OpenFile(filepath.Join(dir, name), os.O_RDONLY, 0)
		if gerr == nil {
			g.Close()
		}
		if i <= maxLinks && (err != nil || gerr != nil) {
			t.Errorf("%d links: %v, os.OpenFile %v", i, err, gerr)
		}
		if i > maxLinks && (!errors.Is(err, syscall.ELOOP) || !errors.Is(gerr, syscall.ELOOP)) {
			t.Errorf("%d links: %v, os.OpenFile %v, want ELOOP", i, err, gerr)
		}
	}
}
