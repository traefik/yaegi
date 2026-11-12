package vm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// rootDirLayout writes a local plugin with no go.mod of its own below a
// directory that has the go.mod of an unrelated project, as when Traefik
// starts in the directory of a Go project and reads
// ./plugins-local/src/github.com/acme/demo. It returns the plugin
// directory and <project>/plugins-local/src.
func rootDirLayout(t *testing.T) (dir, src string) {
	t.Helper()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module github.com/someone/project\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src = filepath.Join(project, "plugins-local", "src")
	dir = filepath.Join(src, "github.com", "acme", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte("package demo\n\nfunc Name() string { return \"demo\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, src
}

// TestRootStopsGoModSearch checks Options.Root:
// with it, a plugin without a go.mod of its own is refused with "no go.mod
// ... up to <Root>", by both loaders. Before, the search for go.mod went up
// to / and took the go.mod of the project above the plugin: "cannot find
// package github.com/acme/demo: not in module github.com/someone/project".
func TestRootStopsGoModSearch(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir, src := rootDirLayout(t)
	for _, goList := range []bool{false, true} {
		_, err := Load(Options{Dir: dir, Root: src, GoList: goList}, "github.com/acme/demo")
		if !errors.Is(err, errNoGoMod) || !strings.Contains(fullLoadMsg(err), "up to "+src) || strings.Contains(err.Error(), src) {
			t.Errorf("GoList %v: err = %v, want no go.mod in %s or its parents up to %s", goList, err, dir, src)
		}
	}

	// With its own go.mod, the plugin loads with Root.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/acme/demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, goList := range []bool{false, true} {
		if _, err := Load(Options{Dir: dir, Root: src, GoList: goList}, "github.com/acme/demo"); err != nil {
			t.Errorf("GoList %v: %v", goList, err)
		}
	}

	// Dir must be in Root.
	other := t.TempDir()
	if _, err := Load(Options{Dir: dir, Root: other}, "github.com/acme/demo"); err == nil || !strings.Contains(err.Error(), "is not in the root directory") {
		t.Errorf("Dir outside Root: err = %v", err)
	}
}

// TestNoRootGoModAbove checks the error without Options.Root, when the only
// go.mod is above the plugin directory and does not hold the plugin package:
// the message says which go.mod was used and that
// the plugin may miss its own. Go also walks up to that go.mod, so the load
// still fails; only the message changes.
func TestNoRootGoModAbove(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir, src := rootDirLayout(t)
	_, err := Load(Options{Dir: dir}, "github.com/acme/demo")
	gomod := filepath.Join(filepath.Dir(filepath.Dir(src)), "go.mod")
	if err == nil || !strings.Contains(err.Error(), "not in module github.com/someone/project") || !strings.Contains(err.Error(), "the go.mod used is "+gomod) {
		t.Errorf("err = %v, want the go.mod used (%s) in the message", err, gomod)
	}
}

// TestRootSymlinks checks Options.Root when Root goes through a symbolic
// link and Dir is resolved, or the reverse (t.TempDir is below /tmp on
// macOS, a link to /private/tmp): Dir is in Root, the search for go.mod
// stops at Root, and the "not in module" message does not say that the
// go.mod is above the plugin directory. Before, Dir was refused as "not in
// the root directory".
func TestRootSymlinks(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir, src := rootDirLayout(t)
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	realSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		t.Fatal(err)
	}
	if realDir == dir {
		t.Skip("no symbolic link in the path of t.TempDir")
	}
	cases := []Options{{Dir: realDir, Root: src}, {Dir: dir, Root: realSrc}}
	for _, opts := range cases {
		_, err := Load(opts, "github.com/acme/demo")
		if !errors.Is(err, errNoGoMod) || !strings.Contains(fullLoadMsg(err), "up to "+realSrc) || strings.Contains(err.Error(), realSrc) {
			t.Errorf("Dir %s, Root %s: err = %v, want no go.mod up to %s", opts.Dir, opts.Root, err, realSrc)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/acme/demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, opts := range cases {
		if _, err := Load(opts, "github.com/acme/demo"); err != nil {
			t.Errorf("Dir %s, Root %s: %v", opts.Dir, opts.Root, err)
		}
		_, err := Load(opts, "github.com/acme/other")
		if err == nil || !strings.Contains(err.Error(), "not in module github.com/acme/demo") || strings.Contains(err.Error(), "above the plugin directory") {
			t.Errorf("Dir %s, Root %s: err = %v, want not in module, without the go.mod above the plugin directory", opts.Dir, opts.Root, err)
		}
	}
}

// TestRootGoWork checks that go list (Options.GoList) does not read a
// go.work file above Options.Root: a workspace of the project Traefik starts
// in, which does not list the plugin module, refused the plugin ("main
// module (github.com/someone/project) does not contain package ...").
func TestRootGoWork(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir, src := rootDirLayout(t)
	project := filepath.Dir(filepath.Dir(src))
	if err := os.WriteFile(filepath.Join(project, "go.work"), []byte("go 1.22\n\nuse .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/acme/demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Options{Dir: dir, GoList: true}, "github.com/acme/demo"); err == nil {
		t.Error("without Root, go list read the go.work above the plugin and the load passed")
	}
	if _, err := Load(Options{Dir: dir, Root: src, GoList: true}, "github.com/acme/demo"); err != nil {
		t.Errorf("with Root: %v", err)
	}
}

// fullLoadMsg returns the message of a load error with the host
// directories, which Error hides (loadpaths.go): the error in the chain.
func fullLoadMsg(err error) string {
	var h *pathsHidden
	if errors.As(err, &h) {
		return h.err.Error()
	}
	return err.Error()
}
