package vm_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/internal/gorelease"
	"github.com/traefik/yaegi/v2/stdtypes"
	"github.com/traefik/yaegi/v2/vm"
)

// TestSupportWindow: one yaegi v2 version supports two Go
// releases, N-1 and N (README, "Supported Go releases"; gorelease.Window).
// Everything that names them must name the same ones, so that a move to a
// new release is done in full: the go line of go.mod (N-1, the oldest), the
// build guards of the generated packages, the generated files of each
// release (stdtypes, stdsyms, aot, vm/zz_natsigs), GO_VERSION (N) and
// GO_VERSION_PREV (N-1) of the CI workflow. The toolchain that runs the test
// must be in the window, and the embedded standard library types and the
// AOT copies it builds must be for its release.
//
// The CI job "next Go" sets YAEGI_FORWARD=1: the release of the newer
// toolchain is then in the window (gorelease), with its data only, so
// go.mod, GO_VERSION and the files of each release are not compared there.
func TestSupportWindow(t *testing.T) {
	forward := os.Getenv("YAEGI_FORWARD") == "1"
	toolchain := gorelease.Release(gorelease.StripExperiments(runtime.Version()))
	if _, ok := gorelease.Minor(toolchain); !ok {
		t.Skipf("toolchain %s is not a Go release", runtime.Version())
	}
	if !gorelease.InWindow(toolchain) {
		t.Errorf("toolchain %s is not in the window of yaegi v2 (%s)", toolchain, gorelease.WindowNames())
	}
	window := gorelease.Window
	first := fmt.Sprintf("go1.%d", window[0])
	last := fmt.Sprintf("go1.%d", window[len(window)-1])
	type check struct{ what, got, want string }
	var checks []check

	if !forward {
		gomod := readRepoFile(t, "go.mod")
		if m := regexp.MustCompile(`(?m)^go (1\.\d+)(\.\d+)?$`).FindStringSubmatch(gomod); m != nil {
			checks = append(checks, check{"go line of go.mod", "go" + m[1], first})
		} else {
			t.Error("go.mod has no go line")
		}
		ci := readRepoFile(t, ".github/workflows/ci.yml")
		for _, v := range []struct{ name, want string }{{"GO_VERSION", last}, {"GO_VERSION_PREV", first}} {
			if m := regexp.MustCompile(`(?m)^\s+` + v.name + `: "(1\.\d+)\.\d+"$`).FindStringSubmatch(ci); m != nil {
				checks = append(checks, check{v.name + " of .github/workflows/ci.yml", "go" + m[1], v.want})
			} else {
				t.Errorf(".github/workflows/ci.yml has no %s", v.name)
			}
		}
		for _, w := range window {
			prefix := fmt.Sprintf("go1_%d", w)
			for _, pattern := range []string{"stdtypes/" + prefix + "_data_packages.go", "stdsyms/" + prefix + "_*.go", "aot/all/" + prefix + ".go", "vm/zz_natsigs_" + prefix + ".go"} {
				if m, _ := filepath.Glob(filepath.Join("..", filepath.FromSlash(pattern))); len(m) == 0 {
					t.Errorf("no %s: generate the data of go1.%d with its toolchain (docs/DEVELOPMENT.md, \"Following a new Go release\")", pattern, w)
				}
			}
		}
	}

	// No data of a release that left the window is left behind (the
	// generators remove it).
	for _, pattern := range []string{"stdtypes/go1_*", "stdsyms/go1_*", "stdsyms/sideeffect/go1_*", "aot/go1_*", "aot/all/go1_*", "vm/zz_natsigs*"} {
		m, _ := filepath.Glob(filepath.Join("..", filepath.FromSlash(pattern)))
		for _, f := range m {
			if gorelease.OutsideWindow(f) || filepath.Base(f) == "zz_natsigs.go" {
				t.Errorf("%s is for a release outside the window %s: remove it (docs/DEVELOPMENT.md, \"Following a new Go release\")", f, gorelease.WindowNames())
			}
		}
	}

	guard := regexp.MustCompile(`(?m)^//go:build !(go1\.\d+) \|\| (go1\.\d+)$`)
	next := fmt.Sprintf("go1.%d", window[len(window)-1]+1)
	for _, f := range []string{"stdtypes/release_guard.go", "stdsyms/release_guard.go", "aot/all/release_guard.go"} {
		if m := guard.FindStringSubmatch(readRepoFile(t, f)); m != nil {
			checks = append(checks, check{f + " (oldest)", m[1], first}, check{f + " (after the newest)", m[2], next})
		} else {
			t.Errorf("%s has no release build constraint", f)
		}
	}

	if stdtypes.Available() {
		u, err := stdtypes.Load()
		if err != nil {
			t.Fatal(err)
		}
		checks = append(checks, check{"stdtypes data", gorelease.Release(u.GoVersion), toolchain})
	}

	versions := vm.AOTGoVersions()
	if len(versions) == 0 {
		t.Error("no AOT copies are registered")
	}
	for _, v := range versions {
		checks = append(checks, check{"AOT copies made with " + v, gorelease.Release(gorelease.StripExperiments(v)), toolchain})
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s is for %s, want %s (window %s, toolchain %s): move every part to the window (docs/DEVELOPMENT.md, \"Following a new Go release\")", c.what, c.got, c.want, gorelease.WindowNames(), toolchain)
		}
	}
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}
