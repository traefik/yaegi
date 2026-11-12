package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHostSnapshot runs the offline check on this checkout: every AOT copy
// must fit the module graph of the host snapshot (Traefik). It failed when
// the copy of github.com/pelletier/go-toml/v2 was from v2.2.4 and Traefik
// required v2.4.3: aot/all did not build in Traefik.
func TestHostSnapshot(t *testing.T) {
	problems, err := runOffline(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestMixedVersions checks the rule on a copy made by hand: a copy that
// imports a native package of its own module fails when the host selects a
// newer version of that module; a complete copy only gets a note.
func TestMixedVersions(t *testing.T) {
	tree := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		p := filepath.Join(tree, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	glue := func(path, mod, ver string) string {
		return "package x\n\nfunc init() {\n\taotvm.RegisterAOT(&aotvm.Package{\n\t\tPath:      \"" + path +
			"\",\n\t\tModule:    \"" + mod + "\",\n\t\tVersion:   \"" + ver + "\",\n\t})\n}\n"
	}
	write("example.com/mixed/zz_aotglue.go", glue("example.com/mixed", "example.com/mixed", "v1.0.0"))
	write("example.com/mixed/a.go", "package mixed\n\nimport _ \"example.com/mixed/sub\"\n")
	write("example.com/whole/zz_aotglue.go", glue("example.com/whole", "example.com/whole", "v1.0.0"))
	write("example.com/whole/a.go", "package whole\n\nimport _ \"github.com/traefik/yaegi/v2/aot/go1_99/example.com/whole/internal\"\n")
	write("_failed/example.com/gone/zz_aotglue.go", glue("example.com/gone", "example.com/gone", "v0.1.0"))

	copies, err := readCopies(tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 2 {
		t.Fatalf("copies: got %d, want 2 (the _failed one is skipped): %+v", len(copies), copies)
	}
	own := map[string]string{"example.com/mixed": "v1.0.0", "example.com/whole": "v1.0.0"}

	// Same versions: nothing to say.
	if p := checkCopies("go1_99", copies, own, map[string]string{"example.com/mixed": "v1.0.0"}); len(p) != 0 {
		t.Errorf("same versions: %v", p)
	}
	// An older host version: MVS keeps yaegi v2's.
	if p := checkCopies("go1_99", copies, own, map[string]string{"example.com/mixed": "v0.9.0"}); len(p) != 0 {
		t.Errorf("older host version: %v", p)
	}
	// A newer host version: the mixed copy fails, the complete one does not.
	host := map[string]string{"example.com/mixed": "v1.2.0", "example.com/whole": "v1.2.0"}
	p := checkCopies("go1_99", copies, own, host)
	if len(p) != 1 || !strings.Contains(p[0], "example.com/mixed@v1.0.0") || !strings.Contains(p[0], "example.com/mixed/sub") {
		t.Errorf("newer host version: got %q, want one problem for example.com/mixed", p)
	}
	// A copy older than yaegi v2's go.mod: not regenerated.
	p = checkCopies("go1_99", copies, map[string]string{"example.com/mixed": "v1.1.0", "example.com/whole": "v1.0.0"}, nil)
	if len(p) != 1 || !strings.Contains(p[0], "scripts/aotgen.sh") {
		t.Errorf("stale copy: got %q", p)
	}
}

// TestRequiresReplace checks that the replacements of the host count: a
// replace that only changes the version gives that version, and a replace
// with another module (a fork) makes the copy fail. Before, requires
// ignored them and the offline check passed for the required version.
func TestRequiresReplace(t *testing.T) {
	host, err := requires("go.mod", []byte(`module host

require (
	example.com/a v1.0.0
	example.com/b v1.0.0
	example.com/c v1.0.0
)

replace example.com/a => example.com/a v1.3.0

replace example.com/b v1.0.0 => github.com/fork/b v1.0.1

replace example.com/c v0.9.0 => ./c
`))
	if err != nil {
		t.Fatal(err)
	}
	if host["example.com/a"] != "v1.3.0" {
		t.Errorf("version replace: got %q, want v1.3.0", host["example.com/a"])
	}
	if host["example.com/c"] != "v1.0.0" {
		t.Errorf("replace of another version: got %q, want v1.0.0", host["example.com/c"])
	}
	copies := []Copy{{Path: "example.com/b", Module: "example.com/b", Version: "v1.0.0"}}
	p := checkCopies("go1_99", copies, map[string]string{"example.com/b": "v1.0.0"}, host)
	if len(p) != 1 || !strings.Contains(p[0], "github.com/fork/b@v1.0.1") {
		t.Errorf("forked module: got %q, want one problem naming the fork", p)
	}
}

// TestLsRemoteCommit checks the choice of the line of git ls-remote: it
// matches patterns by their tail, and refs/heads/backport/master came first.
func TestLsRemoteCommit(t *testing.T) {
	const (
		backport  = "1111111111111111111111111111111111111111"
		master    = "2222222222222222222222222222222222222222"
		tagObj    = "3333333333333333333333333333333333333333"
		tagCommit = "4444444444444444444444444444444444444444"
	)
	out := backport + "\trefs/heads/backport/master\n" + master + "\trefs/heads/master\n" +
		tagObj + "\trefs/tags/v3.0.0\n" + tagCommit + "\trefs/tags/v3.0.0^{}\n"
	for ref, want := range map[string]string{"master": master, "v3.0.0": tagCommit, master: master} {
		if got := lsRemoteCommit(out, ref); got != want {
			t.Errorf("lsRemoteCommit(%q) = %s, want %s", ref, got, want)
		}
	}
}

// TestHostModFile checks the go.mod of the throwaway module of -build: the
// local replacements of the host and the modules they serve are dropped,
// yaegi v2 is required and replaced by the checkout.
func TestHostModFile(t *testing.T) {
	snap := []byte(`module github.com/traefik/traefik/v3

go 1.27.0

require (
	github.com/pelletier/go-toml/v2 v2.4.3
	github.com/traefik/traefik/dynamic/ext v0.0.0-00010101000000-000000000000
)

replace github.com/traefik/traefik/dynamic/ext => ./pkg/config/dynamic/ext

replace github.com/gorilla/mux => github.com/containous/mux v0.0.0-20250523120546-41b6ec3aed59
`)
	out, err := hostModFile(snap, "/src/yaegi")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"module example.com/hostgraph", "github.com/pelletier/go-toml/v2 v2.4.3",
		"github.com/traefik/yaegi/v2 v0.0.0", "github.com/traefik/yaegi/v2 => /src/yaegi", "github.com/containous/mux"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "dynamic/ext") {
		t.Errorf("local replacement kept:\n%s", s)
	}
}
