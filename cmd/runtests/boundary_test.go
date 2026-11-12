package main

import (
	"bytes"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/traefik/yaegi/v2/internal/boundary"
	"github.com/traefik/yaegi/v2/internal/gorelease"
)

// The programs of testdata/boundary test the native boundary of the
// standard library: each row of the inventory (docs/BOUNDARY.md, made by
// cmd/boundaryscan) has a probe, a call of probe("id", func() {...}) that
// gives a value of a program type to the function of the row. Each line
// of output starts with the id of its probe, and a panic of a probe is
// recovered and printed. TestBoundary compares the output of each probe
// with go run, in VM mode, in AOT mode and in restricted AOT mode (the mode
// of Traefik's plugins); the probes that do not give the
// output of Go yet are in testdata/boundary/known.tsv, with their issue.

// boundaryModes are the modes of TestBoundary: the name used in known.tsv
// and the yaegi flags. restricted runs with -aot, as the plugin host of
// Traefik loads plugins (internal/pluginhost). -restricted without -aot is
// not covered here.
var boundaryModes = []struct {
	name  string
	flags []string
}{
	{"vm", nil},
	{"aot", []string{"-aot"}},
	{"restricted", []string{"-restricted", "-aot"}},
}

// boundaryExtraFlags are flags of a mode for one program. net.go imports
// syscall (SyscallConn of its program conns), which a restricted plugin
// can import only with Options.UseUnsafe.
var boundaryExtraFlags = map[string]map[string][]string{
	"restricted": {"net.go": {"-unsafe"}},
}

var probeCall = regexp.MustCompile(`probe\("([^"]+)"`)

// probeIDs returns the ids of the probes of a program, in source order.
func probeIDs(t *testing.T, path string) []string {
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	seen := map[string]bool{}
	for _, m := range probeCall.FindAllSubmatch(src, -1) {
		id := string(m[1])
		if seen[id] {
			t.Fatalf("%s: probe %q twice", path, id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// splitProbes returns the output of each probe: the lines that start with
// "id: ", and the lines after them that start with no id (a multi-line
// value). Output before the first probe goes to "".
func splitProbes(out []byte, ids []string) map[string]string {
	byLen := append([]string(nil), ids...)
	sort.Slice(byLen, func(i, j int) bool { return len(byLen[i]) > len(byLen[j]) })
	res := map[string]string{}
	cur := ""
	for _, line := range strings.SplitAfter(string(out), "\n") {
		if line == "" {
			continue
		}
		for _, id := range byLen {
			if strings.HasPrefix(line, id+": ") {
				cur = id
				break
			}
		}
		res[cur] += line
	}
	return res
}

// TestBoundary runs the programs of testdata/boundary with go run and with
// yaegi v2, and compares the output of each probe.
func TestBoundary(t *testing.T) {
	dir := filepath.Join(root, "testdata", "boundary")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	known, err := boundary.ReadKnown(filepath.Join(dir, "known.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	// file:id mode. A mode may name one Go release (vm@go1.27): the probe
	// differs only with the standard library of that release.
	knownMode := map[string]bool{}
	release := gorelease.Release(runtime.Version())
	for probe, ls := range known {
		for _, k := range ls {
			mode, rel, ok := strings.Cut(k.Mode, "@")
			if mode != "vm" && mode != "aot" && mode != "restricted" {
				t.Errorf("known.tsv: %s: bad mode %q", probe, k.Mode)
			}
			if !ok || rel == release {
				knownMode[probe+" "+mode] = true
			}
		}
	}
	allIDs := map[string]bool{}
	for _, f := range files {
		base := filepath.Base(f)
		ids := probeIDs(t, f)
		allIDs[base+":"] = true // the output before the first probe
		for _, id := range ids {
			allIDs[base+":"+id] = true
		}
		t.Run(base, func(t *testing.T) {
			if ok, err := build.Default.MatchFile(dir, base); err == nil && !ok {
				t.Skip("excluded by its build constraint with this toolchain (encoding/json/v2 needs GOEXPERIMENT=jsonv2 with Go 1.26)")
			}
			t.Parallel()
			goCmd := hangCommand(t, "go", "run", f)
			goCmd.Env = caseEnv(nil)
			want, err := goCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go run: %v\n%s", err, want)
			}
			wantBy := splitProbes(want, ids)
			for _, m := range boundaryModes {
				args := append([]string{}, m.flags...)
				args = append(args, boundaryExtraFlags[m.name][base]...)
				vmCmd := hangCommand(t, yaegiBin, append(args, f)...)
				vmCmd.Env = caseEnv(nil)
				got, err := vmCmd.CombinedOutput()
				gotBy := splitProbes(got, ids)
				if err != nil {
					// the program did not end: the probes after the
					// crash have no output, and fail below
					t.Logf("yaegi %s: %v", m.name, err)
				}
				for _, id := range append([]string{""}, ids...) {
					key := base + ":" + id
					same := gotBy[id] == wantBy[id]
					switch {
					case !same && !knownMode[key+" "+m.name]:
						t.Errorf("%s: probe %q differs from go run (add it to known.tsv with an issue only if it cannot be fixed now)\ngot:\n%s\nwant:\n%s", m.name, id, gotBy[id], wantBy[id])
					case same && knownMode[key+" "+m.name]:
						t.Errorf("%s: probe %q gives the output of go run: remove it from known.tsv", m.name, id)
					}
				}
			}
		})
	}
	for probe := range known {
		if !allIDs[probe] {
			t.Errorf("known.tsv names %q, which is not a probe of testdata/boundary", probe)
		}
	}
}

// TestBoundaryInventory checks the inventory of the native boundary: every
// row of docs/boundary/*.tsv has a probe, every row comment of the probes
// matches a row, and docs/BOUNDARY.md is up to date.
func TestBoundaryInventory(t *testing.T) {
	tsvs, err := filepath.Glob(filepath.Join(root, "docs", "boundary", "go1_*.tsv"))
	if err != nil || len(tsvs) == 0 {
		t.Fatalf("no docs/boundary/go1_*.tsv: %v", err)
	}
	probes, err := boundary.ReadProbes(filepath.Join(root, "testdata", "boundary"))
	if err != nil {
		t.Fatal(err)
	}
	used := make([]bool, len(probes))
	missing := 0
	for _, f := range tsvs {
		rows, err := boundary.ReadTSV(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			ok := false
			for i, p := range probes {
				if p.Covers(r) {
					ok, used[i] = true, true
				}
			}
			if !ok {
				missing++
				if missing <= 50 {
					t.Errorf("%s: no probe for %s %s %s %s", filepath.Base(f), r.Pkg, r.Entry, r.Kind, r.Check)
				}
			}
		}
	}
	if missing > 50 {
		t.Errorf("... %d rows without a probe in all", missing)
	}
	for i, p := range probes {
		if !used[i] {
			t.Errorf("%s: the row comment %s %s %s %s matches no row of the inventory", p.Probe, p.Pkg, strings.Join(p.Entries, ","), p.Kind, p.Check)
		}
	}
	md, err := boundary.MD(filepath.Join(root, "docs"))
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile(filepath.Join(root, "docs", "BOUNDARY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(md, have) {
		t.Errorf("docs/BOUNDARY.md is not up to date: go run ./cmd/boundaryscan -md")
	}
}

// TestBoundaryScan scans GOROOT again and compares the rows with the .tsv
// file of the release of the toolchain (not the sites: their lines change
// with the patch release).
func TestBoundaryScan(t *testing.T) {
	if testing.Short() {
		t.Skip("scans the standard library")
	}
	minor, ok := gorelease.Minor(runtime.Version())
	if !ok {
		t.Skipf("not a Go release: %s", runtime.Version())
	}
	path := filepath.Join(root, "docs", "boundary", fmt.Sprintf("go1_%d.tsv", minor))
	want, err := boundary.ReadTSV(path)
	if os.IsNotExist(err) {
		t.Skipf("no inventory for %s", runtime.Version())
	}
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "env", "GOVERSION").Output(); err != nil || gorelease.Release(strings.TrimSpace(string(out))) != gorelease.Release(runtime.Version()) {
		t.Skipf("the go command is not the release of the test (%s): %v", out, err)
	}
	got, err := boundary.Scan()
	if err != nil {
		t.Fatal(err)
	}
	key := func(r boundary.Row) string { return r.Key() + "\t" + r.Detail }
	wantK, gotK := map[string]bool{}, map[string]bool{}
	for _, r := range want {
		wantK[key(r)] = true
	}
	for _, r := range got {
		gotK[key(r)] = true
		if !wantK[key(r)] {
			t.Errorf("new row: %s", key(r))
		}
	}
	for _, r := range want {
		if !gotK[key(r)] {
			t.Errorf("row gone: %s", key(r))
		}
	}
	if t.Failed() {
		t.Log("regenerate: go run ./cmd/boundaryscan (with the toolchain of each release of the window)")
	}
}
