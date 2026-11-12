package vm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/stdtypes"
)

// Fuzz targets for the toolchain-free loader (srcload.go). They feed attacker
// source to vm.Load and check that it returns (with an error or a plugin)
// instead of hanging or killing the process with a fatal out-of-memory or
// stack overflow. Load recovers panics into errors, so a surviving fatal
// crash shows up as a fuzz crasher.
//
// Run a target for a while with, for example:
//
//	go test ./vm -run x -fuzz FuzzLoadSingleFile -fuzztime 10m
//
// The short `go test` run (no -fuzz) replays the seed corpus only.

// fuzzLoadLimits are tight on purpose: a fuzzer must not spend its budget on
// one input, and the abandoned type-check goroutine of a timed-out load keeps
// running in the background (see loaddos.go).
var fuzzLoadLimits = LoadLimits{MaxTotalSourceBytes: 8 << 20, MaxFiles: 200, MaxPackages: 200}

func fuzzLoadSeeds(f *testing.F) {
	seeds := []string{
		"package p\n",
		"package p\n\nfunc CreateConfig() any { return nil }\n",
		// deep parenthesis nesting (parser)
		"package p\n\nvar X = " + rep("(", 2000) + "1" + rep(")", 2000) + "\n",
		// deep block nesting (parser)
		"package p\n\nfunc F() {" + rep("{", 2000) + rep("}", 2000) + "}\n",
		// big array type (zero value), gc refuses it
		"package p\n\nvar X = [...]byte{1 << 40: 1}\n",
		// nested generic aliases (go/types instantiation)
		genAliasBomb(12),
		// doubled string constant (go/constant)
		doubledConst(20),
		// a huge constant expression
		"package p\n\nconst C = 1 << 62\nvar X = C * C\n",
		// recursive type
		"package p\n\ntype M map[string]M\n\nvar X M\n",
		// many small declarations
		"package p\n" + manyVars(500),
		// unicode / invalid bytes in identifiers
		"package p\n\nvar é = 1\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}
}

// FuzzLoadSingleFile loads one file as a single-file plugin.
func FuzzLoadSingleFile(f *testing.F) {
	if !stdtypes.Available() {
		f.Skip("no embedded stdlib types for this platform")
	}
	fuzzLoadSeeds(f)
	f.Fuzz(func(t *testing.T, src string) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		fuzzLoad(t, dir, "example.com/p")
	})
}

// FuzzLoadModuleTree loads a two-file, two-package module tree: the second
// file is a subpackage the first imports, so the loader walks the module.
func FuzzLoadModuleTree(f *testing.F) {
	if !stdtypes.Available() {
		f.Skip("no embedded stdlib types for this platform")
	}
	f.Add("package p\n\nimport _ \"example.com/p/a\"\n", "package a\n")
	f.Add("package p\n\nimport _ \"example.com/p/a\"\n", "package a\n\ntype M map[string]M\n")
	f.Add("package p\n", "package a\n\nvar X = [...]byte{1 << 40: 1}\n")
	f.Fuzz(func(t *testing.T, root, sub string) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(root), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a", "a.go"), []byte(sub), 0o644); err != nil {
			t.Fatal(err)
		}
		fuzzLoad(t, dir, "example.com/p")
	})
}

// fuzzLoad runs loadImpl synchronously (not through the LoadContext watchdog)
// so the memory of each pathological input is freed before the next input,
// instead of leaking into a background goroutine that would make the fuzzer
// itself run out of memory. loadImpl recovers panics into errors, so only a
// fatal crash (out of memory, stack overflow) escapes, as a fuzz crasher,
// which is what the target looks for.
func fuzzLoad(t *testing.T, dir, pkgPath string) {
	t.Helper()
	limits := fuzzLoadLimits
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = loadImpl(ctx, &Options{Dir: dir, LoadLimits: &limits}, pkgPath)
}

func rep(s string, n int) string {
	b := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}

func doubledConst(n int) string {
	var b []byte
	b = append(b, "package p\n\nconst s0 = \"xxxxxxxx\"\n"...)
	for i := 1; i <= n; i++ {
		b = append(b, []byte("const s"+itoaSmall(i)+" = s"+itoaSmall(i-1)+" + s"+itoaSmall(i-1)+"\n")...)
	}
	b = append(b, []byte("var N = len(s"+itoaSmall(n)+")\n")...)
	return string(b)
}

func manyVars(n int) string {
	var b []byte
	for i := 0; i < n; i++ {
		b = append(b, []byte("var v"+itoaSmall(i)+" = "+itoaSmall(i)+"\n")...)
	}
	return string(b)
}

func itoaSmall(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for i > 0 {
		d = append([]byte{byte('0' + i%10)}, d...)
		i /= 10
	}
	return string(d)
}
