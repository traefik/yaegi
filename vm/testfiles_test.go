package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testFilesPlugin imports a subpackage of its own module. The plugin
// package and the subpackage each have a test file in package main, which
// go build refuses ("found packages p (p.go) and main (p_test.go)").
const testFilesPlugin = `package p

import (
	"context"
	"net/http"

	"example.com/p/sub"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(sub.Word()))
	}), nil
}
`

// TestIgnoreTestFiles checks Options.IgnoreTestFiles with both loaders:
// without it, a test file with another package name refuses the plugin, as
// with go build; with it, the test files are not read and the plugin runs.
func TestIgnoreTestFiles(t *testing.T) {
	needSource(t)
	dir, pkgPath := writePlugin(t, testFilesPlugin)
	files := map[string]string{
		"p_test.go":       "package main\n\nfunc main() {}\n",
		"sub/sub.go":      "package sub\n\nfunc Word() string { return \"ok\" }\n",
		"sub/sub_test.go": "package main\n\nfunc main() {}\n",
	}
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, golist := range []bool{false, true} {
		name := "default"
		if golist {
			name = "golist"
		}
		t.Run(name, func(t *testing.T) {
			_, err := Load(Options{Dir: dir, GoList: golist}, pkgPath)
			if err == nil || (!golist && !strings.Contains(err.Error(), "found packages")) {
				t.Fatalf("without IgnoreTestFiles: %v, want a found packages error", err)
			}
			p, err := Load(Options{Dir: dir, GoList: golist, IgnoreTestFiles: true}, pkgPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "tests")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != "ok" {
				t.Errorf("body %q, want %q", got, "ok")
			}
		})
	}
}

// TestIgnoreTestFilesSymlink checks IgnoreTestFiles when Options.Dir is a
// symbolic link to the plugin directory: the overlay of the go list loader
// must still list the test files (filepath.WalkDir does not follow a link).
func TestIgnoreTestFilesSymlink(t *testing.T) {
	needSource(t)
	real, pkgPath := writePlugin(t, testFilesPlugin)
	files := map[string]string{
		"p_test.go":       "package main\n\nfunc main() {}\n",
		"sub/sub.go":      "package sub\n\nfunc Word() string { return \"ok\" }\n",
		"sub/sub_test.go": "package main\n\nfunc main() {}\n",
	}
	for name, src := range files {
		path := filepath.Join(real, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, dir); err != nil {
		t.Skip(err)
	}
	for _, golist := range []bool{false, true} {
		name := "default"
		if golist {
			name = "golist"
		}
		t.Run(name, func(t *testing.T) {
			if _, err := Load(Options{Dir: dir, GoList: golist, IgnoreTestFiles: true}, pkgPath); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestIgnoreTestFilesSubdir checks IgnoreTestFiles when Options.Dir is a
// directory of the module, not its root: the test files of the other
// packages of the module (sub, above Dir) are left out too, with both
// loaders.
func TestIgnoreTestFilesSubdir(t *testing.T) {
	needSource(t)
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/p\n\ngo 1.22\n",
		"plugin/p.go":     strings.Replace(testFilesPlugin, "package p", "package plugin", 1),
		"sub/sub.go":      "package sub\n\nfunc Word() string { return \"ok\" }\n",
		"sub/sub_test.go": "package main\n\nfunc main() {}\n",
	}
	for name, src := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "plugin")
	for _, golist := range []bool{false, true} {
		name := "default"
		if golist {
			name = "golist"
		}
		t.Run(name, func(t *testing.T) {
			if _, err := Load(Options{Dir: dir, GoList: golist}, "example.com/p/plugin"); err == nil {
				t.Fatal("without IgnoreTestFiles: loaded, want a found packages error")
			}
			if _, err := Load(Options{Dir: dir, GoList: golist, IgnoreTestFiles: true}, "example.com/p/plugin"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
