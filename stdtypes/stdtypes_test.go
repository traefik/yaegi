package stdtypes

import (
	"fmt"
	"go/types"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/internal/gorelease"
)

// traefikTargets are the GOOS/GOARCH pairs of the Traefik releases
// (linux/arm is arm v6 and v7). Without data, vm.Load refuses every plugin
// there unless the host sets Options.GoList.
var traefikTargets = []string{
	"linux/amd64", "linux/arm64", "linux/386", "linux/arm",
	"linux/ppc64le", "linux/s390x", "linux/riscv64",
	"darwin/amd64", "darwin/arm64",
	"windows/amd64", "windows/arm64", "windows/386",
	"freebsd/amd64", "freebsd/386",
	"openbsd/amd64", "openbsd/386", "openbsd/riscv64",
}

// TestPlatforms checks that each target of the Traefik releases has
// embedded data, and that the data files of each platform are there for
// each Go release of the window. The name of a data file
// (go1_NN_data_GOOS_GOARCH.go) selects its platform and its build
// constraint its release, so a binary only embeds the data of its own
// platform and release.
func TestPlatforms(t *testing.T) {
	for _, p := range traefikTargets {
		if !slices.Contains(Platforms, p) {
			t.Errorf("no stdtypes data for %s, a target of the Traefik releases", p)
		}
	}
	other, err := os.ReadFile("data_other.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range Platforms {
		goos, goarch, _ := strings.Cut(p, "/")
		for _, w := range gorelease.Window {
			if !gorelease.HasData(w) {
				continue // YAEGI_FORWARD, a release between the window and tip
			}
			prefix := fmt.Sprintf("go1_%d_", w)
			for _, f := range []string{prefix + "data_" + goos + "_" + goarch + ".go", prefix + "std_" + goos + "_" + goarch + ".gz"} {
				if _, err := os.Stat(f); err != nil {
					t.Errorf("%s: %v", p, err)
				}
			}
		}
		if !strings.Contains(string(other), "!("+goos+" && "+goarch+")") {
			t.Errorf("data_other.go is also built on %s", p)
		}
	}
}

// TestLoad decodes the embedded data and checks a few types against what
// plugins use.
func TestLoad(t *testing.T) {
	if !Available() {
		t.Skip("no data for this platform")
	}
	t0 := time.Now()
	u, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d packages decoded in %v", len(u.Packages()), time.Since(t0))
	for _, path := range []string{"net/http", "encoding/json", "regexp", "crypto/tls", "internal/poll", "unsafe"} {
		if !u.Has(path) {
			t.Errorf("missing %s", path)
		}
	}
	http, _ := u.Import("net/http")
	h := http.Scope().Lookup("Handler")
	if h == nil || !types.IsInterface(h.Type()) {
		t.Fatalf("net/http.Handler: %v", h)
	}
	req := http.Scope().Lookup("Request").Type().Underlying().(*types.Struct)
	found := false
	for i := 0; i < req.NumFields(); i++ {
		if f := req.Field(i); f.Name() == "TLS" {
			// The *tls.ConnectionState of net/http is the object of crypto/tls.
			tls, _ := u.Import("crypto/tls")
			found = types.Identical(f.Type(), types.NewPointer(tls.Scope().Lookup("ConnectionState").Type()))
		}
	}
	if !found {
		t.Error("net/http.Request.TLS is not *crypto/tls.ConnectionState")
	}
	if _, err := u.Import("github.com/x/y"); err == nil {
		t.Error("non-std import succeeded")
	}
}

// TestInitTask checks the init graph of gc embedded by cmd/stdexport: the
// packages without init work (unicode/utf8, math/bits) are not in it, and
// the imports of a package include the ones that its export data does not
// list (errors imports internal/reflectlite).
func TestInitTask(t *testing.T) {
	if !Available() {
		t.Skip("no data for this platform")
	}
	u, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"unicode/utf8", "math/bits", "unsafe"} {
		if deps, ok := u.InitTask(path); ok {
			t.Errorf("%s has an init task (imports %v)", path, deps)
		}
	}
	for path, want := range map[string]string{"errors": "internal/reflectlite", "strings": "unicode", "fmt": "os", "unicode": ""} {
		deps, ok := u.InitTask(path)
		if !ok {
			t.Errorf("%s has no init task", path)
			continue
		}
		if want != "" && !slices.Contains(deps, want) {
			t.Errorf("%s: init task imports %v, want %s in it", path, deps, want)
		}
		for _, d := range deps {
			if _, ok := u.InitTask(d); !ok {
				t.Errorf("%s: imports %s, which has no init task", path, d)
			}
		}
	}
}

func TestRelease(t *testing.T) {
	for v, want := range map[string]string{
		"go1.26.8": "go1.26", "go1.26": "go1.26", "go1.27rc1": "go1.27", "go1.27beta2": "go1.27", "devel +abc": "devel +abc",
	} {
		if got := Release(v); got != want {
			t.Errorf("Release(%q) = %q, want %q", v, got, want)
		}
	}
}

// TestIsStd: IsStd uses the list of the release, not "no dot in the first
// path element": the packages of a main module "test" are not standard.
func TestIsStd(t *testing.T) {
	for _, p := range []string{"fmt", "net/http", "unsafe", "internal/abi", "vendor/golang.org/x/net/idna", "plugin", "syscall/js"} {
		if !IsStd(p) {
			t.Errorf("IsStd(%q) = false", p)
		}
	}
	for _, p := range []string{"test", "test/a", "repro/lib", "command-line-arguments", "example.com/m", "fmt/x", "cmd/go", ""} {
		if IsStd(p) {
			t.Errorf("IsStd(%q) = true", p)
		}
	}
	if !Available() {
		return
	}
	u, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range u.Packages() {
		if !IsStd(p.Path()) {
			t.Errorf("IsStd(%q) = false for a package of the universe", p.Path())
		}
	}
}
