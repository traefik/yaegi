package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/traefik/yaegi/v2/internal/progen"
)

// TestGeneratedPrograms is a differential test: it writes random programs
// (internal/progen, one per seed), and compares yaegi v2 with go on each, in
// every mode of goCompareModes. The seeds are fixed, so a failure is
// reproduced with go run ./cmd/progen -seed N.
//
// YAEGI_PROGEN_N sets the number of programs (default 40), YAEGI_PROGEN_FIRST
// the first seed (default 0): a long run, for example by hand after a change
// of the compiler, is YAEGI_PROGEN_N=2000.
func TestGeneratedPrograms(t *testing.T) {
	n, first := envInt(t, "YAEGI_PROGEN_N", 40), envInt(t, "YAEGI_PROGEN_FIRST", 0)
	dir := t.TempDir()
	for seed := first; seed < first+n; seed++ {
		name := "seed" + strconv.Itoa(seed)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := filepath.Join(dir, name+".go")
			if err := os.WriteFile(src, []byte(progen.Generate(uint64(seed))), 0o644); err != nil {
				t.Fatal(err)
			}
			exe := filepath.Join(dir, name+exeSuffix)
			if out, err := hangCommand(t, "go", "build", "-o", exe, src).CombinedOutput(); err != nil {
				t.Fatalf("go build (a bug of internal/progen): %v\n%s", err, out)
			}
			want, err := hangCommand(t, exe).CombinedOutput()
			if err != nil {
				t.Fatalf("the Go program does not exit with status 0 (a bug of internal/progen): %v\n%s", err, want)
			}
			for _, mode := range goCompareModes {
				got, err := hangCommand(t, yaegiBin, append(append([]string{}, mode...), src)...).CombinedOutput()
				if err != nil {
					t.Errorf("yaegi %v: %v\n%s", mode, err, got)
					continue
				}
				if !bytes.Equal(got, want) {
					t.Errorf("yaegi %v differs from go (go run ./cmd/progen -seed %d):\n%s", mode, seed, diffLines(string(want), string(got)))
				}
			}
		})
	}
}

func envInt(t *testing.T, name string, def int) int {
	t.Helper()
	s := os.Getenv(name)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		t.Fatalf("%s=%q: want a number", name, s)
	}
	return v
}
