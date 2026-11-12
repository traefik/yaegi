package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// knownAOTFailures are the programs of testdata/aot whose AOT run is known
// not to match `go run` (see docs/AOTGEN.md).
var knownAOTFailures = map[string]string{}

// knownFallbackFailures are the programs of testdata/aot whose fallback
// run (the library interpreted from source in AOT mode, as when a plugin
// vendors another version of it) is known not to match `go run` (see
// "Version check of the non-standard copies" in docs/AOTGEN.md). The reason
// starts with the status of the failure (DIFF, UNSUP, FAIL): runMode checks
// it.
var knownFallbackFailures = map[string]string{
	"validator.go": "UNSUP: golang.org/x/sys/cpu is interpreted and has assembly",
}

func init() {
	// The init of golang.org/x/sys/cpu calls its assembly on x86 (cpuid)
	// and on darwin, but on linux/arm64 it reads the CPU features from
	// /proc/self/auxv in Go: validator.go matches go run there.
	if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
		delete(knownFallbackFailures, "validator.go")
	}
}

// TestAOT runs every program of testdata/aot with the AOT packages and
// compares its output with `go run`.
func TestAOT(t *testing.T) {
	runMode(t, "aot", knownAOTFailures)
}

// TestAOTFallback runs the programs of the non-standard libraries of
// testdata/aot in AOT mode with the library interpreted from source: what a
// plugin gets when it vendors another version of the library, since the
// AOT copy is then not used (vm/aotsource.go).
func TestAOTFallback(t *testing.T) {
	runMode(t, "fallback", knownFallbackFailures)
}

// runMode runs every program of testdata/aot in mode and compares its
// output with `go run`; known lists the programs known not to match.
func runMode(t *testing.T, mode string, known map[string]string) {
	if testing.Short() {
		t.Skip("builds yaegi v2 and runs every AOT program twice")
	}
	root, err := FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	progs, err := Discover(filepath.Join(root, "testdata", "aot"), "")
	if err != nil {
		t.Fatal(err)
	}
	yaegiBin := filepath.Join(t.TempDir(), "yaegi")
	if err := BuildVM(root, yaegiBin); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Root: root, VM: yaegiBin, Modes: []string{"go", mode}, Jobs: max(runtime.NumCPU()/2, 1)}
	seen := map[string]bool{}
	for _, r := range Run(cfg, progs) {
		if r.Mode != mode {
			continue
		}
		name := filepath.Base(r.File)
		seen[name] = true
		t.Run(name, func(t *testing.T) {
			why, isKnown := known[name]
			switch {
			case r.Status == "SKIP" && mode == "fallback":
				t.Skip(r.Detail)
			case r.Status == "MATCH" && isKnown:
				t.Errorf("now matches go run: remove it from the known failures (%s)", why)
			case r.Status != "MATCH" && !isKnown:
				t.Errorf("%s: %s", r.Status, r.Detail)
			case isKnown && !strings.HasPrefix(why, r.Status+":"):
				// The reason of a known failure starts with its status
				// (DIFF, UNSUP, FAIL): a failure that changes kind is a
				// change too, not a known failure.
				t.Errorf("fails with %s, the known failure says %q: %s", r.Status, why, r.Detail)
			case isKnown:
				t.Skipf("known failure: %s", why)
			}
		})
	}
	// A known failure that is not a program any more hides nothing, but
	// the list must stay true.
	for name := range known {
		if !seen[name] {
			t.Errorf("known failure %s is not a program of testdata/aot", name)
		}
	}
}
