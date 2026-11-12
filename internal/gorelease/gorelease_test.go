package gorelease

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestVersions(t *testing.T) {
	for _, c := range []struct {
		v       string
		release string
		patch   int
		ok      bool
	}{
		{"go1.26.8", "go1.26", 8, true},
		{"go1.26.0", "go1.26", 0, true},
		{"go1.27rc1", "go1.27", 0, true},
		{"go1.27beta2", "go1.27", 0, true},
		{"go1.26", "go1.26", 0, true},
		{"devel +abc", "devel +abc", 0, false},
		// runtime.Version() of a toolchain built with GOEXPERIMENT
		{"go1.26.9 X:jsonv2", "go1.26", 9, true},
		{"go1.26.9 X:boringcrypto,jsonv2", "go1.26", 9, true},
		{"go1.27rc1 X:jsonv2", "go1.27", 0, true},
	} {
		if got := Release(c.v); got != c.release {
			t.Errorf("Release(%q) = %q, want %q", c.v, got, c.release)
		}
		if p, ok := Patch(c.v); p != c.patch || ok != c.ok {
			t.Errorf("Patch(%q) = %d, %v, want %d, %v", c.v, p, ok, c.patch, c.ok)
		}
	}
	if c, err := Constraint("go1.26.8"); err != nil || c != "go1.26 && !go1.27" {
		t.Errorf("Constraint(go1.26.8) = %q, %v", c, err)
	}
	if _, err := Constraint("go1.20.1"); err == nil {
		t.Error("Constraint accepts a release outside the window")
	}
	for name, want := range map[string]bool{
		"go1_25_data_linux_amd64.go": true,
		"stdtypes/go1_25_std_x.gz":   true,
		"zz_natsigs_go1_25.go":       true,
		"aot/go1_25":                 true,
		"go1_26_bytes.go":            !InWindow("go1.26"),
		"zz_natsigs_go1_27.go":       !InWindow("go1.27"),
		"release_guard.go":           false,
		"data_other.go":              false,
		"zz_natsigs.go":              false,
		"go1_x.go":                   false,
	} {
		if got := OutsideWindow(name); got != want {
			t.Errorf("OutsideWindow(%q) = %v, want %v", name, got, want)
		}
	}
	if p := Prefix("go1.27.3 X:jsonv2"); p != "go1_27" {
		t.Errorf("Prefix = %q", p)
	}
	if _, err := Constraint("devel +abc"); err == nil {
		t.Error("Constraint accepts a devel version")
	}
}

// releaseTags returns the release tags of Go 1.minor, as go/build sets them.
func releaseTags(minor int) map[string]bool {
	tags := map[string]bool{}
	for i := 1; i <= minor; i++ {
		tags[fmt.Sprintf("go1.%d", i)] = true
	}
	return tags
}

// built reports whether a file with the build constraint expr is built by
// Go 1.minor on goos/goarch.
func built(t *testing.T, expr string, minor int, goos, goarch string) bool {
	t.Helper()
	x, err := constraint.Parse("//go:build " + expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	tags := releaseTags(minor)
	tags[goos], tags[goarch] = true, true
	if goos == "darwin" || goos == "linux" {
		tags["unix"] = true
	}
	return x.Eval(func(tag string) bool { return tags[tag] })
}

// TestGuard checks the guard file: built by every release outside the
// window, and, once built, a type error that carries the message. The build with a real newer toolchain is checked by hand (see
// docs/AOTGEN.md): a test cannot download a toolchain.
func TestGuard(t *testing.T) {
	src := Guard("test", "example.com/x/stdtypes", "go generate ./stdtypes")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "release_guard.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name.Name != "stdtypes" {
		t.Errorf("package %s", f.Name.Name)
	}
	expr := buildLine(t, string(src))
	for minor := 20; minor <= 30; minor++ {
		if got, want := built(t, expr, minor, "linux", "amd64"), !InWindow(fmt.Sprintf("go1.%d", minor)); got != want {
			t.Errorf("guard built by go1.%d: %v, want %v", minor, got, want)
		}
	}
	_, err = (&types.Config{}).Check("x", fset, []*ast.File{f}, nil)
	if err == nil || !strings.Contains(err.Error(), "was generated for "+WindowNames()) || !strings.Contains(err.Error(), "go generate ./stdtypes") {
		t.Errorf("the guard type-checks or its error has no message: %v", err)
	}
}

func buildLine(t *testing.T, src string) string {
	t.Helper()
	for _, l := range strings.Split(src, "\n") {
		if expr, ok := strings.CutPrefix(l, "//go:build "); ok {
			return expr
		}
		if strings.HasPrefix(l, "package ") {
			break
		}
	}
	return ""
}

// TestForwardWindow checks the window of YAEGI_FORWARD=1: a newer
// toolchain joins it, Go tip as its release, with the releases between them
// (Filled, without data); an older or unknown version changes nothing.
func TestForwardWindow(t *testing.T) {
	for _, c := range []struct {
		v              string
		window, filled []int
	}{
		{"go1.27.3", []int{26, 27}, nil},
		{"go1.25.1", []int{26, 27}, nil},
		{"go1.28rc1", []int{26, 27, 28}, nil},
		{"devel go1.28-abcdef Mon Oct 26 2026", []int{26, 27, 28}, nil},
		{"devel go1.29-abcdef Mon Oct 26 2026", []int{26, 27, 28, 29}, []int{28}},
		{"devel +abcdef", []int{26, 27}, nil},
	} {
		w, f := forwardWindow([]int{26, 27}, c.v)
		if !slices.Equal(w, c.window) || !slices.Equal(f, c.filled) {
			t.Errorf("forwardWindow(%q) = %v, %v; want %v, %v", c.v, w, f, c.window, c.filled)
		}
	}
}

// TestGeneratedFiles checks the build constraints of the generated data of
// the repository: each release of the window builds its own data files of
// its platform and no guard; a data file of one release is not built by
// the other; the releases before and after the window build no data file
// and every guard. Each release of the window has its data.
func TestGeneratedFiles(t *testing.T) {
	lo, hi := Window[0], Window[len(Window)-1]
	root := filepath.Join("..", "..")
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, dir := range []string{"stdtypes", "stdsyms", "aot/all"} {
		expr := buildLine(t, read(dir+"/release_guard.go"))
		if expr == "" {
			t.Errorf("%s/release_guard.go has no build constraint", dir)
			continue
		}
		for _, w := range Window {
			if built(t, expr, w, runtime.GOOS, runtime.GOARCH) {
				t.Errorf("%s/release_guard.go (%s): built by go1.%d", dir, expr, w)
			}
		}
		if !built(t, expr, lo-1, runtime.GOOS, runtime.GOARCH) || !built(t, expr, hi+1, runtime.GOOS, runtime.GOARCH) {
			t.Errorf("%s/release_guard.go (%s): not built by go1.%d or go1.%d", dir, expr, lo-1, hi+1)
		}
	}
	var data []string
	for _, w := range Window {
		if !HasData(w) {
			continue // YAEGI_FORWARD, a release between the window and tip
		}
		n := 0
		for _, pat := range []string{"stdtypes/go1_%d_data_*.go", "stdsyms/go1_%d_*.go"} {
			m, _ := filepath.Glob(filepath.Join(root, fmt.Sprintf(pat, w)))
			n += len(m)
			data = append(data, m...)
		}
		if n < 100 {
			t.Errorf("go1.%d: only %d generated files", w, n)
		}
	}
	data = append(data, filepath.Join(root, "stdtypes", "data_other.go"))
	for _, f := range data {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(f)
		expr := buildLine(t, string(b))
		if name == "data_other.go" {
			// Built on the other platforms, and on a release outside the
			// window (where the guard stops the build).
			for _, w := range Window {
				if built(t, expr, w, "linux", "amd64") || !built(t, expr, w, "plan9", "386") {
					t.Errorf("%s (%s): wrong constraint for go1.%d", name, expr, w)
				}
			}
			if !built(t, expr, hi+1, "linux", "amd64") {
				t.Errorf("%s (%s): not built by go1.%d", name, expr, hi+1)
			}
			continue
		}
		var minor int
		if _, err := fmt.Sscanf(name, "go1_%d_", &minor); err != nil {
			t.Errorf("%s: no release in the name", name)
			continue
		}
		goos, goarch := runtime.GOOS, runtime.GOARCH
		if p := strings.Split(strings.TrimSuffix(name, ".go"), "_"); len(p) >= 3 {
			if a, o := p[len(p)-1], p[len(p)-2]; strings.Contains(strings.Join(Platforms, " "), o+"/"+a) {
				goos, goarch = o, a // file name suffix _GOOS_GOARCH
			}
		}
		if goos == "windows" && strings.Contains(expr, "!windows") {
			goos = "linux"
		}
		for _, w := range append(slices.Clone(Window), lo-1, hi+1) {
			if got := built(t, expr, w, goos, goarch); got != (w == minor) {
				t.Errorf("%s (%q): built by go1.%d: %v", name, expr, w, got)
			}
		}
	}
}

// TestReleaseCheck checks scripts/release-check.sh, which lets
// scripts/setup.sh stop before the build when the generated data is for
// another Go release (the CI job "next Go" runs setup.sh, then regenerates).
// The release after the window is simulated with its release tag: with it,
// the script names the three guarded packages and the build still stops on
// a guard; with this release, the script passes.
func TestReleaseCheck(t *testing.T) {
	minor, ok := Minor(runtime.Version())
	if !ok {
		t.Skip("devel toolchain")
	}
	root := filepath.Join("..", "..")
	run := func(tags string, name string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = root
		cmd.Env = os.Environ()
		if tags != "" {
			cmd.Env = append(cmd.Env, "GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -tags="+tags))
		}
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Fatalf("%s: %v", name, err)
		}
		return string(out), cmd.ProcessState.ExitCode()
	}
	if out, code := run("", "sh", "scripts/release-check.sh"); code != 0 {
		t.Fatalf("release-check.sh with go1.%d: exit %d\n%s", minor, code, out)
	}
	next := fmt.Sprintf("go1.%d", Window[len(Window)-1]+1)
	out, code := run(next, "sh", "scripts/release-check.sh")
	if code != 2 {
		t.Fatalf("release-check.sh with %s: exit %d, want 2\n%s", next, code, out)
	}
	for _, pkg := range []string{"stdtypes", "stdsyms", "aot/all"} {
		if !strings.Contains(out, "github.com/traefik/yaegi/v2/"+pkg+"\n") {
			t.Errorf("release-check.sh with %s does not name %s:\n%s", next, pkg, out)
		}
	}
	// The guard itself is not loosened: the build still stops.
	out, code = run(next, "go", "build", "./stdsyms")
	if code == 0 || !strings.Contains(out, "was generated for go1.") {
		t.Errorf("go build -tags %s ./stdsyms: exit %d, want the guard error\n%s", next, code, out)
	}
}

// TestCompare checks the order of Go versions: release, then betas, release
// candidates and patch releases.
func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"go1.27rc2", "go1.27.0", -1},
		{"go1.27beta1", "go1.27rc1", -1},
		{"go1.27rc10", "go1.27rc2", 1},
		{"go1.27.10", "go1.27.9", 1},
		{"go1.27", "go1.27.0", 0},
		{"go1.27.3 X:jsonv2", "go1.27.3", 0},
		{"go1.26.10", "go1.27beta1", -1},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
