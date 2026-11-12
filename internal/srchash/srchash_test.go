package srchash

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDir: the hash covers the files `go mod vendor` copies, whatever
// their build constraints, and only them.
func TestDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) { os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644) }
	write("a.go", "package a\n")
	write("a_linux.go", "//go:build linux\n\npackage a\n")
	h1, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Not copied by go mod vendor: no change.
	write("a_test.go", "package a\n")
	write("gen.go", "//go:build ignore\n\npackage main\n")
	write("README.md", "doc")
	if h, _ := Dir(dir); h != h1 {
		t.Errorf("tests, ignored files and other files change the hash")
	}
	// A file of another platform or with a tag is copied: it counts.
	write("a_windows.go", "//go:build windows && !ignore\n\npackage a\n")
	h2, _ := Dir(dir)
	if h2 == h1 {
		t.Errorf("a file for another platform does not change the hash")
	}
	write("a.go", "package a\n\nfunc F() {}\n")
	h3, _ := Dir(dir)
	if h3 == h2 {
		t.Errorf("an edited file does not change the hash")
	}
	// Old "+build" lines count only without a //go:build line, as in
	// go/build: this file is compiled, so it is in the hash.
	write("fork.go", "//go:build go1.1\n// +build ignore\n\npackage a\n\nfunc init() {}\n")
	h4, _ := Dir(dir)
	if h4 == h3 {
		t.Errorf("a compiled file with a false +build line does not change the hash")
	}
	os.Remove(filepath.Join(dir, "fork.go"))
	// Without a //go:build line, a false "+build" line followed by a blank
	// line leaves the file out (go mod vendor does not copy it).
	write("old.go", "// +build ignore\n\npackage main\n")
	if h, _ := Dir(dir); h != h3 {
		t.Errorf("a file with only a false +build line changes the hash")
	}
	// Not followed by a blank line, it is not a build constraint.
	write("old.go", "// +build ignore\npackage a\n")
	if h, _ := Dir(dir); h == h3 {
		t.Errorf("a +build line in the package doc comment leaves the file out")
	}
	os.Remove(filepath.Join(dir, "old.go"))
	// A //go:build line after a /* */ comment still controls.
	write("blk.go", "/* x */\n//go:build ignore\n\npackage main\n")
	if h, _ := Dir(dir); h != h3 {
		t.Errorf("a //go:build ignore line after a block comment does not leave the file out")
	}
	os.Remove(filepath.Join(dir, "blk.go"))
	// A symbolic link to a Go file is an error: go/build compiles the file
	// it points to.
	write("target.txt", "package a\n")
	if err := os.Symlink(filepath.Join(dir, "target.txt"), filepath.Join(dir, "link.go")); err != nil {
		t.Skip(err)
	}
	if _, err := Dir(dir); err == nil {
		t.Errorf("a symbolic link to a Go file gives no error")
	}
}
