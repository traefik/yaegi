package vm

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/internal/gorelease"
	"github.com/traefik/yaegi/v2/internal/gosources"
	"github.com/traefik/yaegi/v2/stdtypes"
	"golang.org/x/tools/go/packages"
)

// TestAOTVersionPolicy checks the rules of aotversion.go: copies of another
// Go release are refused, copies of an older patch release are refused
// unless allowed, and copies of the same or a newer patch release are
// accepted.
func TestAOTVersionPolicy(t *testing.T) {
	for _, c := range []struct {
		host, copies string
		allowStale   bool
		want         string // part of the error; "" for no error
	}{
		{"go1.26.8", "go1.26.8", false, ""},
		{"go1.26.0", "go1.26.8", false, ""},
		{"go1.26.9", "go1.26.8", false, "miss the fixes"},
		{"go1.26.9", "go1.26.8", true, ""},
		{"go1.27.0", "go1.27rc2", false, "miss the fixes"},
		{"go1.27rc2", "go1.27.0", false, ""},
		{"go1.27rc2", "go1.27rc1", false, "miss the fixes"},
		{"go1.27rc1", "go1.27beta2", false, "miss the fixes"},
		{"go1.27beta1", "go1.27rc1", false, ""},
		{"go1.27.0", "go1.27", false, ""},
		{"go1.27.2", "go1.26.8", false, "AOT mode needs copies made for go1.27"},
		{"go1.27.2", "go1.26.8", true, "AOT mode needs copies made for go1.27"},
		{"go1.25.3", "go1.26.8", true, "AOT mode needs copies made for go1.25"},
		{"devel go1.27-abcdef", "go1.26.8", true, "AOT mode needs copies made for"},
		// GOEXPERIMENT builds: runtime.Version() has an " X:" suffix.
		{"go1.26.8 X:jsonv2", "go1.26.8", false, ""},
		{"go1.26.8 X:boringcrypto,jsonv2", "go1.26.9", false, ""},
		{"go1.26.9 X:jsonv2", "go1.26.8", false, "miss the fixes"},
		{"go1.26.9 X:jsonv2", "go1.26.8 X:jsonv2", true, ""},
		{"go1.27.0 X:jsonv2", "go1.26.8", false, "AOT mode needs copies made for go1.27"},
	} {
		err := aotVersionError(c.host, c.copies, c.allowStale)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("host %s, copies %s, allowStale %v: %v", c.host, c.copies, c.allowStale, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("host %s, copies %s, allowStale %v: error %v, want %q", c.host, c.copies, c.allowStale, err, c.want)
		}
	}
}

// fakeAOT registers an AOT copy made with Go version v until the end of the
// test, and returns the hooks of a load in a host built with Go version
// host. The test binary may also hold real AOT copies (made with the Go
// version of the repository).
func fakeAOT(t *testing.T, v, host string) *loadHooks {
	const path = "example.com/fakeaot"
	RegisterAOT(&Package{Path: path, AOTPath: "example.com/aot/fakeaot", GoVersion: v})
	t.Cleanup(func() {
		regMu.Lock()
		delete(aotPkgs, path)
		delete(aotOrig, "example.com/aot/fakeaot")
		regMu.Unlock()
	})
	return &loadHooks{goVersion: func() string { return host }}
}

// TestAOTReleaseMismatchIsAnError checks that AOT copies of another Go
// release make Load and RunMain fail in AOT mode, with an error and not a
// panic (aot/all used to panic in its init, before the main function of
// the host, also when the host did not use AOT mode), and that the other
// modes do not check the copies.
func TestAOTReleaseMismatchIsAnError(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/subplugin"
	h := fakeAOT(t, "go1.26.8", "go1.27.2")
	for _, opts := range []Options{
		{Dir: "..", AOT: true, hooks: h},
		{Dir: "..", AOT: true, AllowStaleAOT: true, hooks: h},
		{Dir: "..", Modes: map[string]Mode{"encoding/json": ModeAOT}, hooks: h},
	} {
		_, err := Load(opts, pkg)
		if err == nil || !strings.Contains(err.Error(), "AOT mode needs copies made for go1.27") {
			t.Errorf("Load(%+v): %v, want a release error", opts, err)
		}
	}
	if _, err := RunMain(RunOptions{Dir: "..", AOT: true, hooks: h}, "./testdata/subplugin"); err == nil || !strings.Contains(err.Error(), "AOT mode needs copies made for go1.27") {
		t.Errorf("RunMain: %v, want a release error", err)
	}
	if _, err := Load(Options{Dir: "..", hooks: h}, pkg); err != nil {
		t.Errorf("default mode: %v", err)
	}
}

// TestAOTStaleCopies checks the patch-level policy at Load: copies made with
// an older patch release than the host are refused, unless the host sets
// AllowStaleAOT; copies made with a newer patch release are accepted.
//
// The test binary may hold the real copies, made with the Go release of the
// toolchain (the guard of aot/all stops the build otherwise): the fake host
// and copies are of that release too. With go1.26 versions written here,
// the test failed with copies made by Go 1.27 (CI job "next Go").
func TestAOTStaleCopies(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/subplugin"
	rel := gorelease.Release(runtime.Version())
	t.Run("older copies", func(t *testing.T) {
		h := fakeAOT(t, rel+".98", rel+".99")
		if _, err := Load(Options{Dir: "..", AOT: true, hooks: h}, pkg); err == nil || !strings.Contains(err.Error(), "AllowStaleAOT") {
			t.Errorf("Load: %v, want a stale copies error", err)
		}
		p, err := Load(Options{Dir: "..", AOT: true, AllowStaleAOT: true, hooks: h}, pkg)
		if err != nil {
			t.Fatalf("Load with AllowStaleAOT: %v", err)
		}
		if _, err := createConfig(p); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("newer copies", func(t *testing.T) {
		h := fakeAOT(t, rel+".98", rel+".0")
		if _, err := Load(Options{Dir: "..", AOT: true, hooks: h}, pkg); err != nil {
			t.Errorf("Load: %v", err)
		}
	})
}

// TestCheckAOT: the check that a host runs at start-up or in its tests
// (CheckAOT, which is checkAOTVersion with runtime.Version) gives the same
// answer as Load in AOT mode, and AOTGoVersions names the versions of the
// copies.
func TestCheckAOT(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/subplugin"
	rel := gorelease.Release(runtime.Version())
	for _, c := range []struct {
		name, copies, host string
		allowStale         bool
		want               string // part of the error; "" for no error
	}{
		{"same", rel + ".0", rel + ".0", false, ""},
		{"older copies", rel + ".98", rel + ".99", false, "AllowStaleAOT"},
		{"older copies allowed", rel + ".98", rel + ".99", true, ""},
		{"newer copies", rel + ".98", rel + ".0", false, ""},
		{"other release", "go1.26.8", "go1.27.2", true, "AOT mode needs copies made for go1.27"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := fakeAOT(t, c.copies, c.host)
			check := checkAOTVersion(c.host, c.allowStale)
			_, load := Load(Options{Dir: "..", AOT: true, AllowStaleAOT: c.allowStale, hooks: h}, pkg)
			for what, err := range map[string]error{"CheckAOT": check, "Load": load} {
				if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
					t.Errorf("%s: %v, want %q", what, err, c.want)
				}
			}
			if !slices.Contains(AOTGoVersions(), c.copies) {
				t.Errorf("AOTGoVersions() = %v, want %s in it", AOTGoVersions(), c.copies)
			}
		})
	}
}

// TestLoadFailsClosedWithoutStdTypes checks that Load does not fall back to
// go list on a platform without embedded standard library types: it returns
// an error, and go list (which runs the Go toolchain on the plugin sources,
// before the load-time checks) only runs when the host sets GoList or Src.
func TestLoadFailsClosedWithoutStdTypes(t *testing.T) {
	calls := 0
	errGoList := errors.New("go list called")
	h := &loadHooks{
		stdtypesAvailable: func() bool { return false },
		packagesLoad: func(*packages.Config, ...string) ([]*packages.Package, error) {
			calls++
			return nil, errGoList
		},
	}
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/subplugin"
	for _, opts := range []Options{{Dir: "..", hooks: h}, {Dir: "..", AOT: true, hooks: h}, {Dir: "..", Unrestricted: true, hooks: h}} {
		_, err := Load(opts, pkg)
		if err == nil || !strings.Contains(err.Error(), "set Options.GoList") {
			t.Errorf("Load(%+v): %v, want an error that names Options.GoList", opts, err)
		}
	}
	if calls != 0 {
		t.Fatalf("go list ran %d times without GoList or Src", calls)
	}
	for _, opts := range []Options{{Dir: "..", GoList: true, hooks: h}, {Dir: "..", Src: []string{"fmt"}, hooks: h}} {
		if _, err := Load(opts, pkg); !errors.Is(err, errGoList) {
			t.Errorf("Load(%+v): %v, want go list to run", opts, err)
		}
	}
	if calls != 2 {
		t.Errorf("go list ran %d times with GoList or Src, want 2", calls)
	}
	// Without the hooks, the plugin loads without go list.
	if !stdtypes.Available() {
		t.Skip("no embedded stdlib types for this platform")
	}
	if _, err := Load(Options{Dir: ".."}, pkg); err != nil {
		t.Fatal(err)
	}
}

// TestStaleAOTFallback checks Options.StaleAOTFallback: with AOT copies made
// with an older Go patch release than the host (a Traefik release built
// with a newer Go patch than its yaegi v2), AOT mode is refused without the
// option, and with it the load goes on in default mode: no AOT copy runs
// (the standard library is native, with the fixes of the host), and
// Plugin.Notes says why. Fresh copies, and AllowStaleAOT, keep AOT mode
// and add no note.
func TestStaleAOTFallback(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/subplugin"
	rel := gorelease.Release(runtime.Version())
	hasNote := func(p *Plugin) bool {
		for _, n := range p.Notes {
			if strings.Contains(n, "StaleAOTFallback") && strings.Contains(n, "miss the fixes") {
				return true
			}
		}
		return false
	}
	t.Run("stale copies", func(t *testing.T) {
		h := fakeAOT(t, rel+".98", rel+".99")
		modes := map[string]Mode{"example.com/fakeaot": ModeAOT, "net/http": ModeNative}
		if _, err := Load(Options{Dir: "..", AOT: true, Modes: modes, hooks: h}, pkg); err == nil || !strings.Contains(err.Error(), "miss the fixes") {
			t.Fatalf("Load without StaleAOTFallback: %v, want a stale copies error", err)
		}
		for _, opts := range []Options{
			{Dir: "..", AOT: true, StaleAOTFallback: true, hooks: h},
			{Dir: "..", Modes: modes, StaleAOTFallback: true, hooks: h},
		} {
			p, err := Load(opts, pkg)
			if err != nil {
				t.Fatalf("Load(%+v): %v", opts, err)
			}
			if _, err := createConfig(p); err != nil {
				t.Fatal(err)
			}
			if p.M.AOT || hasMode(p.M.Modes, ModeAOT) || p.M.OpaqueAny {
				t.Errorf("AOT mode still on: AOT %v, Modes %v, OpaqueAny %v", p.M.AOT, p.M.Modes, p.M.OpaqueAny)
			}
			for path, want := range map[string]Mode{"encoding/json": ModeNative, "html/template": ModeNative, "example.com/fakeaot": ModeInterp, "net/http": ModeNative} {
				if got := p.M.modeOf(path); got != want {
					t.Errorf("mode of %s: %v, want %v", path, got, want)
				}
			}
			if !hasNote(p) {
				t.Errorf("Notes = %q, want a StaleAOTFallback note", p.Notes)
			}
		}
		if len(modes) != 2 || modes["example.com/fakeaot"] != ModeAOT {
			t.Errorf("Load changed the Modes of the caller: %v", modes)
		}
		p, err := Load(Options{Dir: "..", AOT: true, AllowStaleAOT: true, StaleAOTFallback: true, hooks: h}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if !p.M.AOT || hasNote(p) {
			t.Errorf("AllowStaleAOT: AOT %v, Notes %q, want AOT mode and no note", p.M.AOT, p.Notes)
		}
	})
	t.Run("fresh copies", func(t *testing.T) {
		h := fakeAOT(t, rel+".98", rel+".0")
		p, err := Load(Options{Dir: "..", AOT: true, StaleAOTFallback: true, hooks: h}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if !p.M.AOT || p.M.modeOf("example.com/fakeaot") != ModeAOT || len(p.Notes) != 0 {
			t.Errorf("AOT %v, Notes %q, want AOT mode and no note", p.M.AOT, p.Notes)
		}
	})
	t.Run("RunMain", func(t *testing.T) {
		h := fakeAOT(t, rel+".98", rel+".99")
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{
			"go.mod": "module test\n\ngo 1.26\n",
			"main.go": `package main

import (
	"encoding/json"
	"html/template"
	"os"
)

type T struct{ A int }

func (T) Hi() string { return "hi" }

func main() {
	b, err := json.Marshal(T{A: 1})
	if err != nil || string(b) != ` + "`" + `{"A":1}` + "`" + ` {
		panic(string(b))
	}
	template.Must(template.New("t").Parse("{{.A}}")).Execute(os.Stdout, T{A: 2})
}
`,
		})
		if _, err := RunMain(RunOptions{Dir: dir, AOT: true, hooks: h}, "."); err == nil || !strings.Contains(err.Error(), "miss the fixes") {
			t.Fatalf("RunMain without StaleAOTFallback: %v, want a stale copies error", err)
		}
		var notes []string
		if _, err := RunMain(RunOptions{Dir: dir, AOT: true, StaleAOTFallback: true, OnStaleAOT: func(n string) { notes = append(notes, n) }, hooks: h}, "."); err != nil {
			t.Fatalf("RunMain with StaleAOTFallback: %v", err)
		}
		if len(notes) != 1 || !strings.Contains(notes[0], "StaleAOTFallback") {
			t.Errorf("OnStaleAOT got %q, want one note", notes)
		}
	})
	t.Run("NewSession", func(t *testing.T) {
		h := fakeAOT(t, rel+".98", rel+".99")
		if _, err := NewSession(Options{AOT: true, hooks: h}); err == nil || !strings.Contains(err.Error(), "miss the fixes") {
			t.Fatalf("NewSession without StaleAOTFallback: %v, want a stale copies error", err)
		}
		s, err := NewSession(Options{AOT: true, StaleAOTFallback: true, hooks: h})
		if err != nil {
			t.Fatal(err)
		}
		if s.m.AOT || s.m.modeOf("example.com/fakeaot") == ModeAOT {
			t.Errorf("NewSession with StaleAOTFallback: AOT %v, mode of the stale copy %v, want default mode", s.m.AOT, s.m.modeOf("example.com/fakeaot"))
		}
	})
}

// setGoSources adds the entries of m to the manifest of the Go sources
// until the end of the test.
func setGoSources(t *testing.T, m gosources.Manifest) {
	regMu.Lock()
	old := map[string]map[string]string{}
	if goSources == nil {
		goSources = gosources.Manifest{}
	}
	for v, pkgs := range m {
		old[v] = goSources[v]
		goSources[v] = pkgs
	}
	regMu.Unlock()
	t.Cleanup(func() {
		regMu.Lock()
		for v, pkgs := range old {
			if pkgs == nil {
				delete(goSources, v)
			} else {
				goSources[v] = pkgs
			}
		}
		regMu.Unlock()
	})
}

// TestAOTGoSourcesPatch checks the manifest of the Go sources
// (aot/all/gosources.txt) at Load, in the three cases of a host built with
// a Go patch release: the same as the copies (AOT mode); a newer one that
// changed no copied package (AOT mode with the copies, no fallback, no
// regeneration); a newer one that changed a copied package (refused, or
// default mode with StaleAOTFallback). The copies of the test binary (made
// with an older patch release) get the same entries as the fake ones.
func TestAOTGoSourcesPatch(t *testing.T) {
	const pkg = "github.com/traefik/yaegi/v2/vm/testdata/subplugin"
	rel := gorelease.Release(runtime.Version())
	copies, host := rel+".0", rel+".97"
	entry := map[string]string{"fmt": "s1:aaaa"}
	regMu.RLock()
	for _, v := range AOTGoVersions() {
		if e := goSources[v]; e != nil && gorelease.Release(v) == rel {
			entry = e // the real copies of the test binary
		}
	}
	regMu.RUnlock()
	changed := map[string]string{}
	var first string
	for p, h := range entry {
		changed[p] = h
		if first == "" || p < first {
			first = p
		}
	}
	changed[first] = "s1:changed"

	t.Run("same patch", func(t *testing.T) {
		h := fakeAOT(t, copies, copies)
		p, err := Load(Options{Dir: "..", AOT: true, hooks: h}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if !p.M.AOT || len(p.Notes) != 0 {
			t.Errorf("AOT %v, Notes %q, want AOT mode and no note", p.M.AOT, p.Notes)
		}
	})
	t.Run("newer patch, same sources", func(t *testing.T) {
		h := fakeAOT(t, copies, host)
		for _, v := range AOTGoVersions() {
			if v != copies && gorelease.Release(v) == rel {
				setGoSources(t, gosources.Manifest{v: entry})
			}
		}
		setGoSources(t, gosources.Manifest{copies: entry, host: entry})
		if err := checkAOTVersion(host, false); err != nil {
			t.Fatalf("checkAOTVersion: %v", err)
		}
		p, err := Load(Options{Dir: "..", AOT: true, hooks: h}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if !p.M.AOT || p.M.modeOf("example.com/fakeaot") != ModeAOT || len(p.Notes) != 0 {
			t.Errorf("AOT %v, Notes %q, want AOT mode with the copies and no note", p.M.AOT, p.Notes)
		}
	})
	t.Run("newer patch, changed sources", func(t *testing.T) {
		h := fakeAOT(t, copies, host)
		setGoSources(t, gosources.Manifest{copies: entry, host: changed})
		_, err := Load(Options{Dir: "..", AOT: true, hooks: h}, pkg)
		if err == nil || !strings.Contains(err.Error(), "changed the sources of "+first) {
			t.Fatalf("Load: %v, want an error that names %s", err, first)
		}
		p, err := Load(Options{Dir: "..", AOT: true, StaleAOTFallback: true, hooks: h}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		if p.M.AOT || len(p.Notes) == 0 || !strings.Contains(p.Notes[len(p.Notes)-1], "changed the sources of "+first) {
			t.Errorf("AOT %v, Notes %q, want default mode and a note", p.M.AOT, p.Notes)
		}
	})
	t.Run("newer patch, not in the manifest", func(t *testing.T) {
		h := fakeAOT(t, copies, rel+".96")
		setGoSources(t, gosources.Manifest{copies: entry})
		if _, err := Load(Options{Dir: "..", AOT: true, hooks: h}, pkg); err == nil || !strings.Contains(err.Error(), "not in the manifest") {
			t.Fatalf("Load: %v, want a stale copies error", err)
		}
	})
}

// TestAOTGoSourcesManifest checks aot/all/gosources.txt against the AOT
// copies of the test binary: an entry for the Go version of the copies,
// with every registered standard copy in it, and, when the test runs with
// that Go version, the hashes of its GOROOT.
func TestAOTGoSourcesManifest(t *testing.T) {
	regMu.RLock()
	perr := goSourcesErr
	byVersion := map[string][]string{}
	for path, p := range aotPkgs {
		if p.GoVersion != "" && isStdPath(path) {
			v := gorelease.StripExperiments(p.GoVersion)
			byVersion[v] = append(byVersion[v], path)
		}
	}
	entries := map[string]map[string]string{}
	for v := range byVersion {
		entries[v] = goSources[v]
	}
	regMu.RUnlock()
	if perr != nil {
		t.Fatal(perr)
	}
	if len(byVersion) == 0 {
		t.Skip("no AOT copy of the standard library in this build")
	}
	for v, paths := range byVersion {
		e := entries[v]
		if e == nil {
			t.Errorf("aot/all/gosources.txt has no entry for %s, the version of the copies: run sh scripts/aotgen.sh", v)
			continue
		}
		for _, p := range paths {
			if _, ok := e[p]; !ok {
				t.Errorf("aot/all/gosources.txt: %s has no hash for %s", v, p)
			}
		}
		if gorelease.StripExperiments(runtime.Version()) != v {
			continue
		}
		pkgs := make([]string, 0, len(e))
		for p := range e {
			pkgs = append(pkgs, p)
		}
		got, err := gosources.Hash(runtime.GOROOT(), pkgs)
		if err != nil {
			t.Skipf("no GOROOT sources: %v", err)
		}
		for _, p := range pkgs {
			if got[p] != e[p] {
				t.Errorf("aot/all/gosources.txt: %s %s is %s, the GOROOT has %s: run sh scripts/aotgen.sh", v, p, e[p], got[p])
			}
		}
	}
}
