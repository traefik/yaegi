//go:build unix

package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/stdtypes"
)

const linkPlugin = `package cm

import (
	"context"
	"fmt"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "configmap") }), nil
}
`

// configMapDir writes a plugin as a Kubernetes ConfigMap volume shows it:
// the files are in a timestamped directory, ..data is a link to that
// directory, and every file of the plugin is a link through ..data
// (main.go -> ..data/main.go).
func configMapDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	ts := filepath.Join(dir, "..2026_10_22_10_00_00.123456789")
	if err := os.Mkdir(ts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..2026_10_22_10_00_00.123456789", filepath.Join(dir, "..data")); err != nil {
		t.Skip(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(ts, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..data", name), filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestLoadConfigMapLayout loads a plugin whose files are all symbolic links
// that stay in the plugin directory, as in a ConfigMap volume (the
// inlinePlugin local plugins of the Traefik Helm chart). Go follows these
// links. Before, the toolchain-free loader refused every file ("main.go is
// a symbolic link"), so such a plugin never loaded.
func TestLoadConfigMapLayout(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := configMapDir(t, map[string]string{
		"go.mod":  "module example.com/cm\n\ngo 1.22\n",
		"main.go": linkPlugin,
	})
	// Options.Dir itself may be a link (the mount point of the volume).
	link := filepath.Join(t.TempDir(), "plugin")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip(err)
	}
	for _, d := range []string{dir, link} {
		for _, goList := range []bool{false, true} {
			if got := serveOnce(t, Options{Dir: d, GoList: goList}, "example.com/cm", "/"); got != "configmap" {
				t.Errorf("Dir %s, GoList %v: body %q, want %q", d, goList, got, "configmap")
			}
		}
	}
}

// TestLoadCachedSymlink checks that LoadCached reads the files of the
// plugin with the rule of the module files: a link that stays in the
// directory (a ConfigMap volume) is followed, a link that leaves it is
// refused. Before, LoadCached read every file through its links.
func TestLoadCachedSymlink(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	files := map[string]string{
		"go.mod":  "module example.com/cm\n\ngo 1.22\n",
		"main.go": linkPlugin,
	}
	p, err := Load(Options{Dir: configMapDir(t, files)}, "example.com/cm")
	if err != nil {
		t.Fatal(err)
	}
	q, err := p.LoadCached(configMapDir(t, files), Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(q)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), q, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Body.String(); got != "configmap" {
		t.Errorf("body %q, want %q", got, "configmap")
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "main.go"), []byte(linkPlugin), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(out, "main.go"), filepath.Join(dir, "main.go")); err != nil {
		t.Skip(err)
	}
	if _, err := p.LoadCached(dir, Options{}); err == nil || !strings.Contains(err.Error(), "outside the module") {
		t.Errorf("link outside: error %v, want outside the module", err)
	}
}

// TestLoadSymlinkBudget checks that the load budget counts the size of the
// target of a link, not the size of the link.
func TestLoadSymlinkBudget(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	dir := configMapDir(t, map[string]string{
		"go.mod":  "module example.com/cm\n\ngo 1.22\n",
		"main.go": linkPlugin,
	})
	// More than the link (14 bytes), less than the file.
	_, err := Load(Options{Dir: dir, LoadLimits: &LoadLimits{MaxTotalSourceBytes: 100}}, "example.com/cm")
	if err == nil || !strings.Contains(err.Error(), "source too large") {
		t.Errorf("error %v, want source too large", err)
	}
}

// TestLoadSymlinkEscapes checks that the links the toolchain-free loader
// follows never read a file outside the plugin module: a link that leaves
// the module (also through a link to a directory, as ..data in a ConfigMap),
// a dangling link, a link loop and a link to a file that is not regular are
// refused.
func TestLoadSymlinkEscapes(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "p.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		setup func(t *testing.T, mod string) error
		want  string
	}{
		{"outside", func(t *testing.T, mod string) error {
			return os.Symlink(filepath.Join(out, "p.go"), filepath.Join(mod, "p.go"))
		}, "outside the module"},
		{"outside through a directory link", func(t *testing.T, mod string) error {
			if err := os.Symlink(out, filepath.Join(mod, "..data")); err != nil {
				return err
			}
			return os.Symlink(filepath.Join("..data", "p.go"), filepath.Join(mod, "p.go"))
		}, "outside the module"},
		{"relative outside", func(t *testing.T, mod string) error {
			rel, err := filepath.Rel(mod, filepath.Join(out, "p.go"))
			if err != nil {
				return err
			}
			return os.Symlink(rel, filepath.Join(mod, "p.go"))
		}, "outside the module"},
		{"dangling", func(t *testing.T, mod string) error {
			return os.Symlink("missing.go", filepath.Join(mod, "p.go"))
		}, "cannot resolve"},
		{"loop", func(t *testing.T, mod string) error {
			if err := os.Symlink("q.go", filepath.Join(mod, "p.go")); err != nil {
				return err
			}
			return os.Symlink("p.go", filepath.Join(mod, "q.go"))
		}, "cannot resolve"},
		{"fifo", func(t *testing.T, mod string) error {
			if err := syscall.Mkfifo(filepath.Join(mod, "fifo"), 0o644); err != nil {
				t.Skip(err)
			}
			return os.Symlink("fifo", filepath.Join(mod, "p.go"))
		}, "not a regular file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mod := t.TempDir()
			if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := c.setup(t, mod); err != nil {
				t.Skip(err)
			}
			if _, err := Load(Options{Dir: mod}, "example.com/p"); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v, want %q", err, c.want)
			}
		})
	}
}

// TestVendorModulesTxtSymlinkInside checks that a vendor/modules.txt that is
// a link to a regular file of the module is read, as Go reads it.
func TestVendorModulesTxtSymlinkInside(t *testing.T) {
	std, err := stdtypes.Load()
	if err != nil {
		t.Skip(err)
	}
	mod := t.TempDir()
	os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644)
	os.MkdirAll(filepath.Join(mod, "vendor"), 0o755)
	os.WriteFile(filepath.Join(mod, "modules.data"), []byte("# example.com/new v1.0.0\n## explicit; go 1.21\nexample.com/new\n"), 0o644)
	if err := os.Symlink(filepath.Join("..", "modules.data"), filepath.Join(mod, "vendor", "modules.txt")); err != nil {
		t.Skip(err)
	}
	l, err := newSrcLoader(context.Background(), mod, "", "", "example.com/p", std, newLoadBudget(DefaultLoadLimits))
	if err != nil {
		t.Fatal(err)
	}
	if got := l.goVersionOf("example.com/new"); got != "go1.21" {
		t.Errorf("goVersionOf = %s, want go1.21", got)
	}
}

// TestLoadRootGoModSymlinkOutside checks that the go.mod of the plugin
// archive itself (the one findModule looks for, from Options.Dir upwards) is
// refused when it is a symbolic link to a file outside the archive, as the
// nested go.mod and the Go files are. Before, findModule read it through the
// link (os.ReadFile).
func TestLoadRootGoModSymlinkOutside(t *testing.T) {
	if !stdtypes.Available() {
		t.Skip("no embedded standard library types")
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := t.TempDir()
	if err := os.WriteFile(filepath.Join(mod, "p.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(out, "go.mod"), filepath.Join(mod, "go.mod")); err != nil {
		t.Skip(err)
	}
	if _, err := Load(Options{Dir: mod}, "example.com/p"); err == nil || !strings.Contains(err.Error(), "outside the module") {
		t.Errorf("error %v, want outside the module", err)
	}
}

// TestOpenRegularFIFO checks that the open of a module file that is a FIFO
// returns at once with an error: openModuleFile checks the file before it
// opens it, but a regular file swapped for a FIFO between the check and the
// open blocked the open (and the load) for ever, until a writer came.
func TestOpenRegularFIFO(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "p.go"), 0o644); err != nil {
		t.Skip(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	done := make(chan error, 1)
	go func() {
		f, err := openRegular(root, "p.go")
		if err == nil {
			f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("error %v, want not a regular file", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the open of a FIFO blocks")
	}
}
