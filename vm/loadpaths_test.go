package vm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadErrorPaths checks that a load error does not show the host
// directories: the type error of a plugin named the file by its absolute
// path (where the host keeps its plugins). The file is now named from the
// plugin directory, and the error is still a *LoadError when it was one.
func TestLoadErrorPaths(t *testing.T) {
	gp := t.TempDir()
	dir := filepath.Join(gp, "src", "example.com", "bad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package bad\n\nvar X = undefinedThing\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(Options{GoPath: gp, Dir: dir}, "example.com/bad")
	if err == nil {
		t.Fatal("Load: no error")
	}
	msg := err.Error()
	real, _ := filepath.EvalSymlinks(gp)
	for _, p := range []string{gp, real} {
		if strings.Contains(msg, p) {
			t.Errorf("the error shows the host directory %s: %s", p, msg)
		}
	}
	if want := "." + string(filepath.Separator) + "bad.go:3:9: undefined: undefinedThing"; !strings.Contains(msg, want) {
		t.Errorf("error %q, want it to name %q", msg, want)
	}

	// A *LoadError is still found, with its full message.
	o := Options{Dir: dir, Root: gp}
	le := o.hidePaths(&LoadError{Msg: "vm load: " + filepath.Join(dir, "x.go") + ": bad", Err: ErrCanceled})
	var got *LoadError
	if !errors.As(le, &got) || !errors.Is(le, ErrCanceled) {
		t.Fatalf("hidePaths(*LoadError) = %T %v", le, le)
	}
	if s := le.Error(); s != "vm load: ."+string(filepath.Separator)+"x.go: bad" {
		t.Errorf("message %q", s)
	}
	if got.Msg != "vm load: "+filepath.Join(dir, "x.go")+": bad" {
		t.Errorf("the *LoadError in the chain: %q", got.Msg)
	}
	// A longer name that starts with the directory is not cut.
	if s := o.hidePaths(errors.New(dir + "2/y.go")).Error(); s != dir+"2/y.go" && !strings.HasPrefix(s, "$ROOT") {
		t.Errorf("a sibling directory became %q", s)
	}
	// Nor a longer name that ends with the directory (/usr/go and /go):
	// a path that merely has the directory inside is not the directory.
	for _, msg := range []string{"x" + dir + "/y.go", "a-" + dir, "~" + dir + "/"} {
		if s := o.hidePaths(errors.New(msg)).Error(); s != msg {
			t.Errorf("%q became %q", msg, s)
		}
	}
	// Two paths one after the other, with no separator between them, are
	// both replaced.
	if s := o.hidePaths(errors.New(dir + ":" + dir + "/y.go (" + dir + ")")).Error(); s != ".:./y.go (.)" {
		t.Errorf("two paths: %q", s)
	}
}
