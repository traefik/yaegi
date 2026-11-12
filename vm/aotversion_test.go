package vm_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/traefik/yaegi/v2/aot/all"
	"github.com/traefik/yaegi/v2/vm"
)

const castPath = "github.com/spf13/cast"

// castPlugin writes a plugin that vendors github.com/spf13/cast (the module
// yaegi v2 has an AOT copy of) with the given modules.txt header line, and
// calls call in ServeHTTP. edit, when set, changes the vendored sources.
func castPlugin(t *testing.T, header, call string, edit func(dir string)) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", castPath).Output()
	if err != nil {
		t.Skipf("go list -m %s: %v", castPath, err)
	}
	src := strings.TrimSpace(string(out))
	if src == "" {
		// go list -m gives no directory for a module that is not in the
		// module cache: the reads below failed with "open : no such file".
		t.Fatalf("%s is not in the module cache: run go mod download (or scripts/setup.sh)", castPath)
	}
	dir := t.TempDir()
	vend := filepath.Join(dir, "vendor", filepath.FromSlash(castPath))
	for _, sub := range []string{"", "internal"} {
		ents, err := os.ReadDir(filepath.Join(src, sub))
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Join(vend, sub), 0o755)
		for _, e := range ents {
			if e.IsDir() || strings.HasSuffix(e.Name(), "_test.go") || e.Name() == "go.mod" || e.Name() == "go.sum" {
				continue
			}
			b, err := os.ReadFile(filepath.Join(src, sub, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(vend, sub, e.Name()), b, 0o644)
		}
	}
	if edit != nil {
		edit(vend)
	}
	os.WriteFile(filepath.Join(dir, "vendor", "modules.txt"), []byte(header+"\n## explicit; go 1.21.0\n"+castPath+"\n"+castPath+"/internal\n"), 0o644)
	// go.mod says what the header says (version and replacement): the
	// plugin is consistently vendored, as go build requires.
	f := strings.Fields(strings.TrimPrefix(header, "# "))
	goMod := "module example.com/plug\n\ngo 1.22\n\nrequire " + f[0] + " " + f[1] + "\n"
	if len(f) >= 5 && f[2] == "=>" {
		goMod += "\nreplace " + f[0] + " " + f[1] + " => " + f[3] + " " + f[4] + "\n"
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644)
	os.WriteFile(filepath.Join(dir, "plug.go"), []byte(`package plug

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cast"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type P struct{}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	return P{}, nil
}

func (P) ServeHTTP(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `+call+`) }
`), 0o644)
	return dir
}

// fork makes the vendored cast a fork: ToString wraps its result, and a new
// function ToFoo exists only in the fork.
func fork(dir string) {
	f := filepath.Join(dir, "zz_generated.go")
	b, _ := os.ReadFile(f)
	s := strings.Replace(string(b), "func ToString(i any) string {",
		"func ToFoo() string { return \"foo\" }\n\nfunc ToString(i any) string { return \"FORK<\" + toString0(i) + \">\" }\n\nfunc toString0(i any) string {", 1)
	os.WriteFile(f, []byte(s), 0o644)
}

// TestAOTVendoredVersion: an AOT copy runs a plugin's package only when the
// plugin vendors the same module version with the same sources. A fork, an
// other version or a replaced module is interpreted from the plugin's
// vendor directory, so the plugin runs its own code; before, -aot silently
// ran yaegi v2's copy, and a function of the fork only failed at request time
// ("native func github.com/spf13/cast.ToFoo not in registry").
func TestAOTVendoredVersion(t *testing.T) {
	const same = "# " + castPath + " v1.10.0"
	for _, tc := range []struct {
		name, header, call string
		edit               func(string)
		want               string
		aot                bool   // the AOT copy is used
		why                string // part of the fallback reason
	}{
		{"same", same, "cast.ToString(42)", nil, "42", true, ""},
		{"fork", same, "cast.ToString(42)", fork, "FORK<42>", false, "differ"},
		{"fork-only-func", same, "cast.ToFoo()", fork, "foo", false, "differ"},
		{"other-version", "# " + castPath + " v1.9.2", "cast.ToString(42)", nil, "42", false, "v1.9.2"},
		{"replaced", same + " => example.com/castfork v1.0.0", "cast.ToString(42)", nil, "42", false, "replaces"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, m := serve(t, castPlugin(t, tc.header, tc.call, tc.edit))
			if got != tc.want {
				t.Errorf("body %q, want %q", got, tc.want)
			}
			if aot := vm.ModeFor(m.Modes, m.AOT, castPath) == vm.ModeAOT; aot != tc.aot {
				t.Errorf("AOT copy used: %v, want %v (fallback: %v)", aot, tc.aot, m.AOTFallback)
			}
			if why := m.AOTFallback[castPath]; !strings.Contains(why, tc.why) || (tc.why == "") != (why == "") {
				t.Errorf("fallback reason %q, want one with %q", why, tc.why)
			}
		})
	}
}
