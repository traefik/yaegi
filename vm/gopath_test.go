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

// writeFiles writes files (slash paths relative to root -> content).
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// goPathLayout writes a GOPATH with a local plugin without go.mod, as
// Traefik reads ./plugins-local: <gp>/src/github.com/acme/demo imports a
// sibling package of the GOPATH (github.com/acme/shared) and a package that
// is both in its own vendor directory and in the GOPATH (the vendored copy
// wins). The GOPATH is below a project with its own go.mod. It returns the
// GOPATH and the plugin directory.
func goPathLayout(t *testing.T) (gp, dir string) {
	t.Helper()
	project := t.TempDir()
	gp = filepath.Join(project, "plugins-local")
	writeFiles(t, project, map[string]string{
		"go.mod": "module github.com/someone/project\n\ngo 1.22\n",
		"plugins-local/src/github.com/acme/demo/demo.go": `package demo

import (
	"github.com/acme/shared"
	"github.com/other/lib"
)

func Name() string { return shared.Name() + "+" + lib.Name() }
`,
		"plugins-local/src/github.com/acme/demo/vendor/github.com/other/lib/lib.go": "package lib\n\nfunc Name() string { return \"vendored\" }\n",
		"plugins-local/src/github.com/other/lib/lib.go":                             "package lib\n\nfunc Name() string { return \"gopath\" }\n",
		"plugins-local/src/github.com/acme/shared/shared.go": `package shared

import "github.com/other/lib"

func Name() string { return "shared(" + lib.Name() + ")" }
`,
	})
	return gp, filepath.Join(gp, "src", "github.com", "acme", "demo")
}

// callName calls the Name function of p and returns its string result.
func callName(t *testing.T, p *Plugin) string {
	t.Helper()
	res, err := p.Call(context.Background(), "Name")
	if err != nil {
		t.Fatal(err)
	}
	v, err := res[0].Interface()
	if err != nil {
		t.Fatal(err)
	}
	return v.(string)
}

// TestGoPathMode checks Options.GoPath: a local
// plugin without go.mod loads in GOPATH mode, with the imports resolved as
// in Go's GOPATH mode (the vendor directory of the importing package, then
// <GoPath>/src). Without GoPath, it is refused (no go.mod), and the go list
// loader refuses GOPATH mode.
func TestGoPathMode(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	gp, dir := goPathLayout(t)
	src := filepath.Join(gp, "src")

	p, err := Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	// demo sees its vendored lib, shared (outside of demo) the GOPATH one.
	if got, want := callName(t, p), "shared(gopath)+vendored"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	// AOT mode does not change it: the GOPATH packages have no module.
	p, err = Load(Options{Dir: dir, GoPath: gp, AOT: true}, "github.com/acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := callName(t, p), "shared(gopath)+vendored"; got != want {
		t.Errorf("AOT: Name() = %q, want %q", got, want)
	}

	// Without GoPath: no go.mod (with Root), or the go.mod of the project.
	if _, err := Load(Options{Dir: dir, Root: src}, "github.com/acme/demo"); !errors.Is(err, errNoGoMod) {
		t.Errorf("without GoPath: err = %v, want no go.mod", err)
	}
	if _, err := Load(Options{Dir: dir}, "github.com/acme/demo"); err == nil {
		t.Error("without GoPath nor Root: the plugin loaded")
	}

	// The go list loader refuses GOPATH mode.
	_, err = Load(Options{Dir: dir, GoPath: gp, GoList: true}, "github.com/acme/demo")
	if !errors.Is(err, errNoGoMod) || !strings.Contains(err.Error(), "needs the default loader") {
		t.Errorf("GoList: err = %v, want GOPATH mode needs the default loader", err)
	}

	// Dir must be in <GoPath>/src.
	if _, err := Load(Options{Dir: dir, Root: gp, GoPath: filepath.Join(gp, "src", "github.com")}, "github.com/acme/demo"); err == nil || !strings.Contains(err.Error(), "is not in the root directory") {
		t.Errorf("Dir outside GoPath: err = %v", err)
	}
}

// TestGoPathEscape checks that GOPATH mode reads nothing outside of
// <GoPath>/src: an import through a symbolic link that leaves it, a
// relative import and an import with a vendor element are refused.
func TestGoPathEscape(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	gp, dir := goPathLayout(t)
	outside := t.TempDir()
	writeFiles(t, outside, map[string]string{"evil/evil.go": "package evil\n\nfunc Name() string { return \"evil\" }\n"})
	link := filepath.Join(gp, "src", "github.com", "evil")
	if err := os.Symlink(filepath.Join(outside, "evil"), link); err != nil {
		t.Skipf("no symbolic links: %v", err)
	}
	cases := map[string]string{
		`"github.com/evil"`: "is outside of",
		`"../shared"`:       `invalid path element ".."`,
		`"github.com/acme/demo/vendor/github.com/other/lib"`: "vendor element",
	}
	for imp, want := range cases {
		writeFiles(t, dir, map[string]string{"demo.go": "package demo\n\nimport x " + imp + "\n\nfunc Name() string { return x.Name() }\n"})
		_, err := Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("import %s: err = %v, want an error with %q", imp, err, want)
		}
	}
}

// TestGoPathLinks checks that GOPATH mode reads every file with the rule of
// the module files, with <GoPath>/src in place of the module
// (openModuleFile): a plugin whose files are links inside <GoPath>/src (a
// Kubernetes ConfigMap volume) loads, and a Go file that is a link to a file
// outside of <GoPath>/src, directly or through a link to a directory, is
// refused.
func TestGoPathLinks(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	gp, dir := goPathLayout(t)
	shared := filepath.Join(gp, "src", "github.com", "acme", "shared")
	// shared as a ConfigMap volume: shared.go -> ..data/shared.go, and
	// ..data -> a timestamped directory, all in <GoPath>/src.
	os.RemoveAll(shared)
	ts := filepath.Join(shared, "..2026_10_22_10_00_00.123456789")
	writeFiles(t, ts, map[string]string{"shared.go": "package shared\n\nfunc Name() string { return \"linked\" }\n"})
	if err := os.Symlink("..2026_10_22_10_00_00.123456789", filepath.Join(shared, "..data")); err != nil {
		t.Skipf("no symbolic links: %v", err)
	}
	if err := os.Symlink(filepath.Join("..data", "shared.go"), filepath.Join(shared, "shared.go")); err != nil {
		t.Fatal(err)
	}
	p, err := Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := callName(t, p), "linked+vendored"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}

	// A link that leaves <GoPath>/src is refused: the file itself, or the
	// directory it goes through.
	outside := t.TempDir()
	writeFiles(t, outside, map[string]string{"shared.go": "package shared\n\nfunc Name() string { return \"evil\" }\n"})
	for _, link := range []struct{ name, target string }{
		{filepath.Join(shared, "shared.go"), filepath.Join(outside, "shared.go")},
		{filepath.Join(shared, "..data"), outside},
	} {
		if err := os.Remove(link.name); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(link.target, link.name); err != nil {
			t.Fatal(err)
		}
		_, err := Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
		if err == nil || !strings.Contains(err.Error(), "is outside the module") {
			t.Errorf("%s -> %s: err = %v, want refused as outside", link.name, link.target, err)
		}
	}

	// The reads before the load (the "// yaegi:tags" lines of the plugin
	// directory, the headers that go/build reads) follow the same rule: a
	// file that go/build leaves out (//go:build ignore) but that is a link
	// out of <GoPath>/src refuses the plugin, as in module mode. Before,
	// these reads used os.Open: the plugin loaded with the tags of the file
	// outside (Name() returned "tagged").
	os.RemoveAll(shared)
	writeFiles(t, outside, map[string]string{"tags.go": "//go:build ignore\n\n// yaegi:tags evil\n\npackage demo\n"})
	writeFiles(t, dir, map[string]string{
		"demo.go":   "package demo\n\nfunc Name() string { return base() }\n",
		"plain.go":  "//go:build !evil\n\npackage demo\n\nfunc base() string { return \"plain\" }\n",
		"tagged.go": "//go:build evil\n\npackage demo\n\nfunc base() string { return \"tagged\" }\n",
	})
	if err := os.Symlink(filepath.Join(outside, "tags.go"), filepath.Join(dir, "tags.go")); err != nil {
		t.Fatal(err)
	}
	_, err = Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
	if err == nil || !strings.Contains(err.Error(), "tags.go is outside the module") {
		t.Errorf("tags.go -> outside: err = %v, want refused as outside", err)
	}
}

// TestGoPathStdWins checks that a directory of <GoPath>/src with the path of
// a standard library package never replaces it: an import of fmt is the
// standard library, and the plugin package itself cannot be fmt (go build in
// GOPATH mode builds GOROOT's fmt, never <GoPath>/src/fmt).
func TestGoPathStdWins(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	gp, dir := goPathLayout(t)
	src := filepath.Join(gp, "src")
	writeFiles(t, src, map[string]string{
		"fmt/fmt.go":                   "package fmt\n\nfunc Sprint(a ...any) string { return \"shadow\" }\n",
		"github.com/acme/demo/demo.go": "package demo\n\nimport \"fmt\"\n\nfunc Name() string { return fmt.Sprint(\"std\") }\n",
	})
	p, err := Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	if got := callName(t, p); got != "std" {
		t.Errorf("Name() = %q, want std (the standard library fmt)", got)
	}
	_, err = Load(Options{Dir: filepath.Join(src, "fmt"), GoPath: gp}, "fmt")
	if err == nil || !strings.Contains(err.Error(), "is in the standard library") {
		t.Errorf("Load fmt: err = %v, want refused as a standard library package", err)
	}
}

// TestGoPathKeepsModuleMode checks that a plugin with a go.mod keeps module
// mode with Options.GoPath: its imports are not read from the GOPATH.
func TestGoPathKeepsModuleMode(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	gp, dir := goPathLayout(t)
	writeFiles(t, dir, map[string]string{"go.mod": "module github.com/acme/demo\n\ngo 1.22\n"})
	_, err := Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
	if err == nil || !strings.Contains(err.Error(), "cannot find package github.com/acme/shared: not in module github.com/acme/demo") {
		t.Errorf("err = %v, want github.com/acme/shared not in module", err)
	}
	writeFiles(t, dir, map[string]string{"demo.go": "package demo\n\nimport \"github.com/other/lib\"\n\nfunc Name() string { return lib.Name() }\n"})
	p, err := Load(Options{Dir: dir, GoPath: gp}, "github.com/acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	if got := callName(t, p); got != "vendored" {
		t.Errorf("Name() = %q, want vendored", got)
	}
}
