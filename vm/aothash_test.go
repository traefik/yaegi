package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/internal/srchash"
)

// aotHashPlugin writes a plugin that vendors example.com/aotlib2 (lib.go:
// A returns V, which is "src") with the given vendor/modules.txt, and
// registers an AOT copy of aotlib2 whose A returns "aot-a", with the hash
// of these sources. extra, when set, then changes the vendored package.
func aotHashPlugin(t *testing.T, modulesTxt string, extra func(dir, lib string)) string {
	t.Helper()
	dir := t.TempDir()
	lib := filepath.Join(dir, "vendor", "example.com", "aotlib2")
	os.MkdirAll(lib, 0o755)
	os.WriteFile(filepath.Join(lib, "lib.go"), []byte("package aotlib2\n\nvar V = \"src\"\n\nfunc A() string { return V }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "vendor", "modules.txt"), []byte(modulesTxt), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/plug\n\ngo 1.22\n\nrequire example.com/aotlib2 v1.0.0\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "plug.go"), []byte(`package plug

import (
	"context"
	"io"
	"net/http"

	"example.com/aotlib2"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type P struct{}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return P{}, nil
}

func (P) ServeHTTP(w http.ResponseWriter, r *http.Request) { io.WriteString(w, aotlib2.A()) }
`), 0o644)
	h, err := srchash.Dir(lib)
	if err != nil {
		t.Fatal(err)
	}
	RegisterAOT(&Package{
		Path: "example.com/aotlib2", AOTPath: "example.com/aotlib2/aotcopy",
		Module: "example.com/aotlib2", Version: "v1.0.0", Sources: map[string]string{"example.com/aotlib2": h},
		Funcs: map[string]reflect.Value{"A": reflect.ValueOf(func() string { return "aot-a" })},
	})
	t.Cleanup(func() {
		regMu.Lock()
		delete(aotPkgs, "example.com/aotlib2")
		delete(aotOrig, "example.com/aotlib2/aotcopy")
		regMu.Unlock()
	})
	if extra != nil {
		extra(dir, lib)
	}
	return dir
}

const aotlib2Modules = "# example.com/aotlib2 v1.0.0\n## explicit; go 1.21\nexample.com/aotlib2\n"

// forkInit is a file of a fork of aotlib2 that go/build compiles (the
// //go:build line controls) but whose "+build ignore" line hid it from the
// source hash: the AOT copy ran instead of the fork's init.
const forkInit = "//go:build go1.1\n// +build ignore\n\npackage aotlib2\n\nfunc init() { V = \"fork\" }\n"

// TestAOTSourceHash: the source hash covers every file the loader
// compiles, so an AOT copy never runs instead of a fork of its package.
// The plugin then runs its own code, and AOTFallback tells why.
func TestAOTSourceHash(t *testing.T) {
	for _, tc := range []struct {
		name       string
		modulesTxt string
		extra      func(dir, lib string)
		goList     bool
		want, why  string // body, part of the fallback reason ("" for the AOT copy)
	}{
		{"same", aotlib2Modules, nil, false, "aot-a", ""},
		{"plus-build-ignore", aotlib2Modules, func(dir, lib string) {
			os.WriteFile(filepath.Join(lib, "fork.go"), []byte(forkInit), 0o644)
		}, false, "fork", "differ"},
		// A vendored package with no "# module" line in modules.txt.
		{"no-module-line", "example.com/aotlib2\n", nil, false, "src", "is not from module example.com/aotlib2"},
		// The go list path (goListInfo).
		{"golist-same", aotlib2Modules, nil, true, "aot-a", ""},
		{"golist-plus-build-ignore", aotlib2Modules, func(dir, lib string) {
			os.WriteFile(filepath.Join(lib, "fork.go"), []byte(forkInit), 0o644)
		}, true, "fork", "differ"},
		// go/build compiles a symbolic link to a Go file: the hash must not
		// skip it.
		{"golist-symlink", aotlib2Modules, func(dir, lib string) {
			os.WriteFile(filepath.Join(dir, "fork.txt"), []byte("package aotlib2\n\nfunc init() { V = \"fork\" }\n"), 0o644)
			if err := os.Symlink(filepath.Join(dir, "fork.txt"), filepath.Join(lib, "fork.go")); err != nil {
				t.Skip(err)
			}
		}, true, "fork", "not a regular file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := aotHashPlugin(t, tc.modulesTxt, tc.extra)
			if tc.goList {
				if _, err := exec.LookPath("go"); err != nil {
					t.Skip("no go command")
				}
				t.Setenv("GOFLAGS", "-mod=vendor")
			}
			// A modules.txt without the module line does not match go.mod:
			// this case is about the hash, not about the vendoring check.
			ignoreVendor := tc.name == "no-module-line"
			p, err := Load(Options{Dir: dir, AOT: true, GoList: tc.goList, IgnoreInconsistentVendoring: ignoreVendor}, "example.com/plug")
			if err != nil {
				t.Fatal(err)
			}
			cfg, _ := createConfig(p)
			hd, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "plug")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			hd.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != tc.want {
				t.Errorf("body %q, want %q (fallback: %v)", got, tc.want, p.M.AOTFallback)
			}
			why := p.M.AOTFallback["example.com/aotlib2"]
			if !strings.Contains(why, tc.why) || (tc.why == "") != (why == "") {
				t.Errorf("fallback reason %q, want one with %q", why, tc.why)
			}
		})
	}
}

// TestAOTSourceHashLoaderFiles: a file that the loader compiles but that
// is not in the source hash is a mismatch, even with the same hash.
func TestAOTSourceHashLoaderFiles(t *testing.T) {
	lib := t.TempDir()
	os.WriteFile(filepath.Join(lib, "lib.go"), []byte("package aotlib2\n"), 0o644)
	os.WriteFile(filepath.Join(lib, "gen.go"), []byte("//go:build ignore\n\npackage main\n"), 0o644)
	h, err := srchash.Dir(lib)
	if err != nil {
		t.Fatal(err)
	}
	p := &Package{Path: "example.com/aotlib2", Module: "example.com/aotlib2", Version: "v1.0.0", Sources: map[string]string{"example.com/aotlib2": h}}
	info := func(files ...string) func(string) (srcInfo, bool) {
		return func(string) (srcInfo, bool) {
			return srcInfo{module: "example.com/aotlib2", version: "v1.0.0", dir: lib, files: files}, true
		}
	}
	if why := aotMismatch(p, info("lib.go"), map[string]*srcHash{}); why != "" {
		t.Errorf("same files: mismatch %q", why)
	}
	if why := aotMismatch(p, info("lib.go", "gen.go"), map[string]*srcHash{}); !strings.Contains(why, "gen.go") {
		t.Errorf("compiled file left out of the hash: mismatch %q, want one about gen.go", why)
	}
}
