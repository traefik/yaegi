package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeHostBuilds builds the native host of a small vendored plugin and
// runs it. The template of the native host (nativeMain) is Go code in a
// string: a mistake there makes every native build fail, and every plugin of
// the catalog is then classified against a "build" failure.
func TestNativeHostBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	mc, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Skip("no go command:", err)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSpace(string(mc)), mapstructurePath+"@"+mapstructureVersion)); err != nil {
		t.Skip("mapstructure is not in the module cache (scripts/setup.sh)")
	}
	wd, _ := os.Getwd()
	real := findRoot(wd)
	tmp := t.TempDir()
	if err := os.Symlink(filepath.Join(real, "vmhost"), filepath.Join(tmp, "vmhost")); err != nil {
		t.Fatal(err)
	}
	old := root
	root = tmp
	defer func() { root = old }()

	dir := filepath.Join(tmp, "plugsrc")
	files := map[string]string{
		"go.mod":             "module example.com/nativetest\n\ngo 1.22\n",
		"vendor/modules.txt": "",
		"plug.go": `package nativetest

import (
	"context"
	"net/http"
)

type Config struct{ Prefix string }

func CreateConfig() *Config { return &Config{Prefix: "X-Default"} }

func New(ctx context.Context, next http.Handler, c *Config, name string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(c.Prefix+"-Name", name)
		next.ServeHTTP(w, r)
	}), nil
}
`,
	}
	for name, body := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := &plugin{name: "nativetest", dir: dir, modDir: dir, m: manifest{Import: "example.com/nativetest", BasePkg: "nativetest"}}
	out := filepath.Join(tmp, "out")
	os.MkdirAll(out, 0o755)
	exe, err := buildNative(p, out)
	if err != nil {
		t.Fatalf("native build: %v", err)
	}
	td := filepath.Join(out, "testdata.json")
	os.WriteFile(td, []byte(`{"prefix":"X-Test"}`), 0o644)
	got, err := exec.Command(exe, "-import", "example.com/nativetest", "-basepkg", "nativetest", "-testdata", td, "-dir", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("native host: %v\n%s", err, got)
	}
	if !strings.Contains(string(got), `X-Test-Name: ["`) {
		t.Errorf("the configuration of testData did not reach New:\n%s", got)
	}
}
