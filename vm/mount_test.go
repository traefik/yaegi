//go:build linux || darwin

package vm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mountPlug calls the functions of mount.go. Each function returns its
// result, or "err: " and whether the error is fs.ErrPermission.
const mountPlug = `package p

import (
	"crypto/tls"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

func res(s string, err error) string {
	if err != nil {
		return "err: " + perm(err)
	}
	return s
}

func perm(err error) string {
	if errors.Is(err, fs.ErrPermission) {
		return "permission"
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "not exist"
	}
	return err.Error()
}

func Read(name string) string {
	b, err := os.ReadFile(name)
	return res(string(b), err)
}

func Write(name, data string) string {
	return res("ok", os.WriteFile(name, []byte(data), 0o600))
}

func Remove(name string) string { return res("ok", os.Remove(name)) }

func Rename(a, b string) string { return res("ok", os.Rename(a, b)) }

func Stat(name string) string {
	fi, err := os.Stat(name)
	if err != nil {
		return res("", err)
	}
	return fi.Name()
}

func Symlink(target, name string) string { return res("ok", os.Symlink(target, name)) }

func Walk(root string) string {
	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		names = append(names, rel)
		return nil
	})
	return res(strings.Join(names, ","), err)
}

func Glob(pattern string) string {
	m, err := filepath.Glob(pattern)
	for i := range m {
		m[i] = filepath.Base(m[i])
	}
	return res(strings.Join(m, ","), err)
}

func Eval(name string) string {
	s, err := filepath.EvalSymlinks(name)
	return res(s, err)
}

func Temp(dir string) string {
	f, err := os.CreateTemp(dir, "x*.tmp")
	if err != nil {
		return res("", err)
	}
	defer f.Close()
	return res(filepath.Dir(f.Name()), nil)
}

func DirFS(dir, name string) string {
	b, err := fs.ReadFile(os.DirFS(dir), name)
	return res(string(b), err)
}

func Serve(dir, path string) string {
	rec := httptest.NewRecorder()
	http.FileServer(http.Dir(dir)).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return strings.TrimSpace(rec.Body.String())
}

func Stdout() string {
	f, err := os.OpenFile("/dev/stdout", os.O_WRONLY, 0)
	if err != nil {
		return res("", err)
	}
	f.Close()
	return "ok"
}

func KeyPair(dir string) string {
	_, err := tls.LoadX509KeyPair(filepath.Join(dir, "a"), filepath.Join(dir, "b"))
	return res("ok", err)
}

func ParseFiles(name string) string {
	_, err := template.ParseFiles(name)
	return res("ok", err)
}
`

// TestMounts checks Options.Mounts: the plugin reaches the files of its
// mounts and nothing else, through "..", symbolic links, a rename or the
// functions of path/filepath and net/http.
func TestMounts(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, mountPlug)
	base := t.TempDir()
	rw, ro, out := filepath.Join(base, "rw"), filepath.Join(base, "ro"), filepath.Join(base, "out")
	for _, d := range []string{rw, ro, out, filepath.Join(rw, "sub")} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{
		filepath.Join(rw, "a.txt"):        "rw a",
		filepath.Join(rw, "sub", "b.txt"): "rw b",
		filepath.Join(ro, "c.txt"):        "ro c",
		filepath.Join(out, "secret"):      "secret",
	} {
		if err := os.WriteFile(name, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(out, "secret"), filepath.Join(rw, "abs-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../out/secret", filepath.Join(rw, "rel-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub/b.txt", filepath.Join(rw, "in-link")); err != nil {
		t.Fatal(err)
	}
	mounts := []Mount{{Dir: rw}, {Dir: ro, ReadOnly: true}}
	for _, aot := range []bool{false, true} {
		p, err := Load(Options{Dir: dir, AOT: aot, Mounts: mounts}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		call := func(name string, args ...any) string {
			t.Helper()
			res, err := p.Call(context.Background(), name, args...)
			if err != nil {
				return "panic: " + err.Error()
			}
			v, _ := res[0].Interface()
			return v.(string)
		}
		for _, c := range []struct {
			got, want string
		}{
			{call("Read", filepath.Join(rw, "a.txt")), "rw a"},
			{call("Read", filepath.Join(ro, "c.txt")), "ro c"},
			{call("Read", filepath.Join(out, "secret")), "err: permission"},
			{call("Read", filepath.Join(rw, "..", "out", "secret")), "err: permission"},
			{call("Read", filepath.Join(rw, "abs-link")), "path escapes from parent"},
			{call("Read", filepath.Join(rw, "rel-link")), "path escapes from parent"},
			{call("Read", filepath.Join(rw, "in-link")), "rw b"},
			{call("Stat", filepath.Join(out, "secret")), "err: permission"},
			{call("Write", filepath.Join(rw, "new.txt"), "new"), "ok"},
			{call("Read", filepath.Join(rw, "new.txt")), "new"},
			{call("Write", filepath.Join(ro, "new.txt"), "new"), "err: permission"},
			{call("Write", filepath.Join(out, "new.txt"), "new"), "err: permission"},
			{call("Remove", filepath.Join(out, "secret")), "err: permission"},
			{call("Remove", filepath.Join(ro, "c.txt")), "err: permission"},
			{call("Rename", filepath.Join(rw, "new.txt"), filepath.Join(out, "new.txt")), "err: permission"},
			{call("Rename", filepath.Join(rw, "new.txt"), filepath.Join(rw, "sub", "new.txt")), "ok"},
			{call("Symlink", "/", filepath.Join(rw, "root")), "ok"},
			{call("Read", filepath.Join(rw, "root", "etc", "hosts")), "path escapes from parent"},
			{call("Walk", filepath.Join(rw, "sub")), ".,b.txt,new.txt"},
			{call("Walk", out), "err: permission"},
			{call("Glob", filepath.Join(rw, "*.txt")), "a.txt"},
			{call("Glob", filepath.Join(out, "*")), ""},
			{call("Eval", filepath.Join(rw, "in-link")), filepath.Join(rw, "sub", "b.txt")},
			{call("Temp", rw), rw},
			{call("Temp", out), "err: permission"},
			{call("DirFS", rw, "sub/b.txt"), "rw b"},
			{call("DirFS", rw, "rel-link"), "path escapes from parent"},
			{call("DirFS", out, "secret"), "err: permission"},
			{call("Serve", rw, "/a.txt"), "rw a"},
			{call("Serve", out, "/secret"), "403 Forbidden"},
			{call("Stdout"), "ok"},
			{call("KeyPair", out), "crypto/tls.LoadX509KeyPair (it opens a path outside Options.Mounts) is not allowed in a plugin"},
			{call("ParseFiles", filepath.Join(rw, "a.txt")), "text/template.ParseFiles (it opens a path outside Options.Mounts) is not allowed in a plugin"},
		} {
			if !strings.Contains(c.got, c.want) || c.want == "" && c.got != "" {
				t.Errorf("AOT %v: %q, want %q", aot, c.got, c.want)
			}
		}
		if _, err := os.Stat(filepath.Join(out, "secret")); err != nil {
			t.Errorf("AOT %v: %v", aot, err)
		}
		if err := os.Rename(filepath.Join(rw, "sub", "new.txt"), filepath.Join(rw, "new.txt")); err != nil {
			t.Fatal(err)
		}
		os.Remove(filepath.Join(rw, "root"))
		p.Close()
	}
}

// TestMountsSandbox checks that a Sandbox checks its paths, then acts
// through the mounts: a WritePaths entry outside the mounts gives nothing.
func TestMountsSandbox(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, mountPlug)
	rw, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(out, "secret"), []byte("secret"), 0o600)
	os.WriteFile(filepath.Join(rw, "a.txt"), []byte("rw a"), 0o600)
	p, err := Load(Options{Dir: dir, Mounts: []Mount{{Dir: rw}}, Sandbox: &Sandbox{ReadPaths: []string{rw, out}}}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, c := range []struct{ name, want string }{
		{filepath.Join(rw, "a.txt"), "rw a"},
		{filepath.Join(out, "secret"), "err: permission"},
	} {
		res, err := p.Call(context.Background(), "Read", c.name)
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := res[0].Interface(); v != c.want {
			t.Errorf("Read %s: %v, want %q", c.name, v, c.want)
		}
	}
}
