package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// writeTree writes the files of tree (slash paths) below dir.
func writeTree(t *testing.T, dir string, tree map[string]string) {
	t.Helper()
	for name, data := range tree {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// sameDir reports whether a and b are the same directory, without their
// symbolic links (the temporary directory of macOS is below a link).
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// TestFindModule: findModule walks up the parent directories to the
// nearest go.mod, as the go command does, also through directories that do
// not exist, and stops at Options.Root (limit) when it is set.
func TestFindModule(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"go.mod":        "module example.com/m\n\ngo 1.22\n",
		"a/b/c.go":      "package b\n",
		"n/go.mod":      "module example.com/m/n\n\ngo 1.22\n",
		"n/deep/d/x.go": "package d\n",
	})
	for _, c := range []struct {
		dir, limit string
		root, path string // want; root "" for an error
		noGoMod    bool   // the error wraps errNoGoMod
	}{
		{"a/b", "", ".", "example.com/m", false},
		{".", "", ".", "example.com/m", false},
		{"a/b/missing/dir", "", ".", "example.com/m", false},
		{"n/deep/d", "", "n", "example.com/m/n", false}, // the nearest go.mod
		{"a/b", ".", ".", "example.com/m", false},       // limit is the module root
		{"a/b", "a", "", "", true},                      // the search stops at limit
		{"a/b", "a/b", "", "", true},
		{"a", "n", "", "", false}, // dir is not below limit
	} {
		dir := filepath.Join(root, filepath.FromSlash(c.dir))
		limit := ""
		if c.limit != "" {
			limit = filepath.Join(root, filepath.FromSlash(c.limit))
		}
		gotRoot, gotPath, _, err := findModule(dir, limit)
		switch {
		case c.root == "":
			if err == nil {
				t.Errorf("findModule(%s, %q) = %s, want an error", c.dir, c.limit, gotRoot)
			} else if errors.Is(err, errNoGoMod) != c.noGoMod {
				t.Errorf("findModule(%s, %q): %v, errNoGoMod %v", c.dir, c.limit, err, c.noGoMod)
			}
		case err != nil:
			t.Errorf("findModule(%s, %q): %v", c.dir, c.limit, err)
		case !sameDir(t, gotRoot, filepath.Join(root, c.root)) || gotPath != c.path:
			t.Errorf("findModule(%s, %q) = %s %s, want %s %s", c.dir, c.limit, gotRoot, gotPath, c.root, c.path)
		}
	}
}

// TestSrcLoaderVendoredModule: the toolchain-free loader reads a
// dependency of a module from its vendor directory, the packages of the
// module from the module, and refuses a dependency that is not vendored.
func TestSrcLoaderVendoredModule(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"go.mod":                        "module example.com/plug\n\ngo 1.22\n\nrequire example.com/dep v1.0.0\n",
		"plug.go":                       "package plug\n\nimport (\n\t\"example.com/dep\"\n\t\"example.com/plug/inner\"\n)\n\nfunc Y() int { return dep.X() + inner.Z }\n",
		"inner/in.go":                   "package inner\n\nconst Z = 2\n",
		"vendor/modules.txt":            "# example.com/dep v1.0.0\n## explicit; go 1.21\nexample.com/dep\n",
		"vendor/example.com/dep/dep.go": "package dep\n\nfunc X() int { return 1 }\n",
	})
	std, err := stdtypes.Load()
	if err != nil {
		t.Fatal(err)
	}
	l, err := newSrcLoader(context.Background(), root, "", "", "example.com/plug", std, newLoadBudget(DefaultLoadLimits))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"example.com/plug":       ".",
		"example.com/plug/inner": "inner",
		"example.com/dep":        "vendor/example.com/dep",
	} {
		d, err := l.dirOf(path)
		if err != nil || !sameDir(t, d, filepath.Join(root, filepath.FromSlash(want))) {
			t.Errorf("dirOf(%s) = %s, %v, want %s", path, d, err, want)
		}
	}
	if !l.isOwn("example.com/plug/inner") || l.isOwn("example.com/dep") {
		t.Error("isOwn is wrong for the module package or the dependency")
	}
	if _, err := l.dirOf("example.com/other"); err == nil || !strings.Contains(err.Error(), "not in module example.com/plug nor in its vendor directory") {
		t.Errorf("dirOf of a package that is not vendored: %v", err)
	}
	if got := l.goVersionOf("example.com/dep"); got != "go1.21" {
		t.Errorf("goVersionOf(example.com/dep) = %s, want the go line of modules.txt", got)
	}

	r, err := loadSource(context.Background(), root, "", "", "example.com/plug", DefaultLoadLimits, Options{}.loadPolicy())
	if err != nil {
		t.Fatal(err)
	}
	var depFile string
	for _, p := range r.prog.AllPackages() {
		if p.Pkg.Path() == "example.com/dep" {
			if fn := p.Func("X"); fn != nil {
				depFile = r.prog.Fset.Position(fn.Pos()).Filename
			}
		}
	}
	if !strings.Contains(filepath.ToSlash(depFile), "/vendor/example.com/dep/") {
		t.Errorf("example.com/dep.X comes from %q, want the vendor directory", depFile)
	}
}
