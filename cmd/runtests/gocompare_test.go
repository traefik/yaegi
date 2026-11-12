package main

import (
	"bytes"
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The programs of testdata/gocompare/<area>/*.go are compared with Go in
// every mode, with no list to keep: a new file is a new test. Each program
// is built once with go build, and its output (stdout and stderr together)
// and exit status are the reference for every mode of yaegi v2.
//
// The rules of a program (README of testdata/gocompare):
//   - it exits with status 0: a panic that is not recovered prints the
//     stacks of the goroutines, which differ between Go and yaegi v2;
//   - its output is the same at each run on every platform of CI: no map
//     printed in iteration order (fmt sorts maps, a range does not), no
//     address, no time, no random number without a fixed seed, and the same
//     result with a 32-bit int (linux/386, linux/arm);
//   - it only imports packages that a restricted plugin may import (no
//     os/exec, unsafe, syscall, net/http/pprof...): it also runs with
//     -restricted.

// exeSuffix is the suffix of the binaries that go build -o writes: it does
// not add ".exe" on windows.
var exeSuffix = map[bool]string{true: ".exe"}[runtime.GOOS == "windows"]

// goCompareModes are the modes of yaegi v2 that each program runs in.
var goCompareModes = [][]string{nil, {"-aot"}, {"-opaque"}, {"-restricted"}}

// discoverGoCompare returns the programs of testdata/gocompare, relative to
// that directory ("ints/overflow.go").
func discoverGoCompare(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(root, "testdata", "gocompare")
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestGoCompare runs every program of testdata/gocompare with go and with
// yaegi v2 in each mode of goCompareModes, and compares the combined output and
// the exit status.
func TestGoCompare(t *testing.T) {
	files := discoverGoCompare(t)
	if len(files) == 0 {
		t.Fatal("no program in testdata/gocompare")
	}
	bin := t.TempDir()
	for i, f := range files {
		path := filepath.Join(root, "testdata", "gocompare", filepath.FromSlash(f))
		if ok, err := build.Default.MatchFile(filepath.Dir(path), filepath.Base(path)); err == nil && !ok {
			// Excluded by its build constraint with this toolchain: a skip,
			// so that a wrong constraint shows in the output.
			t.Run(f, func(t *testing.T) { t.Skip("excluded by its build constraint") })
			continue
		}
		// The reference: the program built by go, run once for all modes.
		var once sync.Once
		var want []byte
		var wantErr string
		ref := func(t *testing.T) ([]byte, string) {
			once.Do(func() {
				exe := filepath.Join(bin, strings.TrimSuffix(filepath.Base(f), ".go")+"-"+itoa(i)+exeSuffix)
				if out, err := hangCommand(t, "go", "build", "-o", exe, path).CombinedOutput(); err != nil {
					wantErr = "go build: " + err.Error() + "\n" + string(out)
					return
				}
				cmd := hangCommand(t, exe)
				cmd.Dir = filepath.Dir(path)
				out, err := cmd.CombinedOutput()
				want = out
				if err != nil {
					wantErr = "the Go program does not exit with status 0 (see testdata/gocompare/README.md): " + err.Error() + "\n" + string(out)
				}
			})
			return want, wantErr
		}
		for _, mode := range goCompareModes {
			name := f
			if len(mode) > 0 {
				name += " " + strings.Join(mode, "_")
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				want, wantErr := ref(t)
				if wantErr != "" {
					t.Fatal(wantErr)
				}
				cmd := hangCommand(t, yaegiBin, append(append([]string{}, mode...), path)...)
				cmd.Dir = filepath.Dir(path)
				got, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("yaegi %v: %v\n%s", mode, err, got)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("yaegi %v differs from go:\n%s", mode, diffLines(string(want), string(got)))
				}
			})
		}
	}
}

// diffLines returns the first lines where got differs from want, with a
// little context: a long output does not drown the difference.
func diffLines(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	shown := 0
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl == gl {
			continue
		}
		b.WriteString("line " + itoa(i+1) + ":\n  go:   " + wl + "\n  yaegi: " + gl + "\n")
		if shown++; shown == 10 {
			b.WriteString("...\n")
			break
		}
	}
	if shown == 0 && len(w) != len(g) {
		b.WriteString("different number of lines\n")
	}
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// TestGoCompareRules checks the form of the programs of testdata/gocompare:
// package main, a first comment that says what the program checks, and a
// directory per area. The other rules are checked by the runs: an import
// that a restricted plugin may not have fails the -restricted run, and a
// program that only works with a 64-bit int fails the nightly linux/386 job.
func TestGoCompareRules(t *testing.T) {
	for _, f := range discoverGoCompare(t) {
		path := filepath.Join(root, "testdata", "gocompare", filepath.FromSlash(f))
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if !strings.Contains(src, "\npackage main\n") && !strings.HasPrefix(src, "package main\n") {
			t.Errorf("%s: not package main", f)
		}
		if !strings.HasPrefix(src, "//") {
			t.Errorf("%s: starts without a comment that says what it checks", f)
		}
		if filepath.Dir(filepath.FromSlash(f)) == "." {
			t.Errorf("%s: put it in the directory of its area (testdata/gocompare/<area>/)", f)
		}
	}
	// A 32-bit int: the nightly CI job "linux/386" runs this test there.
}
