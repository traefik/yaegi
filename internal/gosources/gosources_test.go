package gosources

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestManifest checks the round trip of the manifest file, the order of
// the versions and Same in its three cases.
func TestManifest(t *testing.T) {
	m := Manifest{
		"go1.27.10": {"fmt": "s1:a", "html/template": "s1:c"},
		"go1.27.3":  {"fmt": "s1:a", "html/template": "s1:b"},
		"go1.27.4":  {"fmt": "s1:a", "html/template": "s1:b"},
	}
	b := m.Format()
	if !strings.Contains(string(b), "go1.27.4 fmt s1:a\ngo1.27.4 html/template s1:b\ngo1.27.10 fmt") {
		t.Errorf("Format: versions not in order:\n%s", b)
	}
	back, err := Parse(b)
	if err != nil || !reflect.DeepEqual(back, m) {
		t.Fatalf("Parse(Format) = %v, %v", back, err)
	}
	if same, _, known := m.Same("go1.27.3", "go1.27.4 X:jsonv2"); !same || !known {
		t.Errorf("Same(go1.27.3, go1.27.4): %v %v, want same", same, known)
	}
	if same, changed, known := m.Same("go1.27.3", "go1.27.10"); same || !known || !reflect.DeepEqual(changed, []string{"html/template"}) {
		t.Errorf("Same(go1.27.3, go1.27.10): %v %v %v, want html/template changed", same, changed, known)
	}
	if same, _, known := m.Same("go1.27.3", "go1.27.5"); same || known {
		t.Errorf("Same(go1.27.3, go1.27.5): %v %v, want unknown", same, known)
	}
	for _, bad := range []string{"go1.27.3 fmt\n", "nogo fmt s1:a\n", "go1.27.3 fmt s1:a\ngo1.27.3 fmt s1:b\n"} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q): no error", bad)
		}
	}
}

// TestStdCopiesAndRecord checks the list of the copied standard packages of
// a tree and Record, which hashes them in a GOROOT and prunes the older
// patch releases of the release.
func TestStdCopiesAndRecord(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "aot", "go1_27")
	goroot := filepath.Join(dir, "goroot")
	for _, f := range []string{
		"aot/go1_27/fmt/fmt.go", "aot/go1_27/internal/fmtsort/s.go", "aot/go1_27/all/all.go",
		"aot/go1_27/gopkg.in/yaml.v3/y.go", "aot/go1_27/_failed/github.com/x/y.go", "aot/go1_27/_failed/z/z.go", "goroot/src/fmt/print.go", "goroot/src/fmt/print_test.go",
	} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pkgs, err := StdCopies(tree)
	if err != nil || !reflect.DeepEqual(pkgs, []string{"fmt", "internal/fmtsort"}) {
		t.Fatalf("StdCopies = %v, %v", pkgs, err)
	}
	path := filepath.Join(dir, "gosources.txt")
	if err := os.WriteFile(path, Manifest{"go1.27.2": {"fmt": "s1:x"}, "go1.27.5": {"fmt": "s1:y"}, "go1.25.1": {"fmt": "s1:z"}}.Format(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Record(path, "go1.27.3", goroot, pkgs, true); err != nil {
		t.Fatal(err)
	}
	m, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if m["go1.27.2"] != nil || m["go1.25.1"] != nil || m["go1.27.5"] == nil {
		t.Errorf("Record with prune kept %v", m)
	}
	if e := m["go1.27.3"]; !strings.HasPrefix(e["fmt"], "s1:") || e["internal/fmtsort"] != Absent {
		t.Errorf("entry of go1.27.3: %v", e)
	}
}

// TestLess checks the order of the versions: betas, then release
// candidates, then patch releases, as aotVersionError in vm. runUpdate
// skips the versions older than the copies, so go1.27.0 must come after
// go1.27rc2.
func TestLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"go1.27rc2", "go1.27.0", true},
		{"go1.27.0", "go1.27rc2", false},
		{"go1.27rc1", "go1.27rc2", true},
		{"go1.27rc10", "go1.27rc2", false},
		{"go1.27beta1", "go1.27rc1", true},
		{"go1.27rc1", "go1.27beta2", false},
		{"go1.27beta2", "go1.27.0", true},
		{"go1.27.0", "go1.27.1", true},
		{"go1.27.9", "go1.27.10", true},
		{"go1.26.10", "go1.27rc1", true},
		{"go1.27.0", "go1.27", false},
		{"go1.27", "go1.27.0", true}, // the same version: ordered as strings
		{"go1.27rc1", "go1.27", true},
		{"go1.27.4 X:jsonv2", "go1.27.5", true},
	} {
		if got := Less(c.a, c.b); got != c.want {
			t.Errorf("Less(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
