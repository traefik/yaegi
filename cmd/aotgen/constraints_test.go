package main

import (
	"fmt"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testPorts are ports of each byte order and word size, and other GOOS.
var testPorts = []string{"linux/amd64", "linux/arm64", "linux/386", "linux/s390x", "linux/ppc64",
	"linux/ppc64le", "linux/mips", "linux/mipsle", "darwin/arm64", "windows/amd64", "js/wasm", "aix/ppc64"}

// TestCopyBuildConstraints generates the copies of encoding/binary and
// encoding/json, then checks, for every port and with and without
// GOEXPERIMENT=jsonv2, that each copy builds the same files as its source
// package, or is refused by its guard file (aotgen_guard.go). Before, aotgen
// removed the build constraints of the files it copied: the copy of
// encoding/binary had native_endian_little.go on big-endian targets too
// (NativeEndian was little-endian on s390x), and the copy of encoding/json
// stayed json v1 with GOEXPERIMENT=jsonv2, where encoding/json is built on
// encoding/json/v2.
func TestCopyBuildConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("loads and generates packages")
	}
	root, mod, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	g := &gen{root: root, module: mod, outDir: out, outPath: mod + "/aot", keep: map[string]bool{}}
	targets := []string{"encoding/binary", "encoding/json"}
	if err := g.load(targets); err != nil {
		t.Fatal(err)
	}
	g.plan()
	if err := g.generate(); err != nil {
		t.Fatal(err)
	}
	for _, path := range targets {
		pi := g.pkgs[path]
		src := filepath.Dir(pi.p.GoFiles[0])
		var srcFiles []string
		for _, f := range append(slices.Clone(pi.p.GoFiles), pi.p.IgnoredFiles...) {
			if name := filepath.Base(f); strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
				srcFiles = append(srcFiles, name)
			}
		}
		slices.Sort(srcFiles)
		ents, err := os.ReadDir(pi.dir)
		if err != nil {
			t.Fatal(err)
		}
		var copyFiles []string
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".go") && e.Name() != "zz_aotglue.go" {
				copyFiles = append(copyFiles, e.Name())
			}
		}
		refused := 0
		for _, port := range testPorts {
			for _, exp := range [][]string{nil, {"goexperiment.jsonv2"}} {
				ctx := build.Default
				ctx.GOOS, ctx.GOARCH, _ = strings.Cut(port, "/")
				ctx.CgoEnabled = false
				ctx.ToolTags = exp
				sel := func(dir string, files []string) []string {
					var out []string
					for _, f := range files {
						ok, err := ctx.MatchFile(dir, f)
						if err != nil {
							t.Fatal(err)
						}
						if ok {
							out = append(out, f)
						}
					}
					return out
				}
				want := sel(src, srcFiles)
				got := sel(pi.dir, copyFiles)
				guard := slices.Contains(got, "aotgen_guard.go")
				got = slices.DeleteFunc(got, func(f string) bool { return f == "aotgen_guard.go" })
				same := slices.Equal(want, got)
				switch {
				case guard && same:
					t.Errorf("%s %s %v: the guard refuses a build where the copy has the files of the source", path, port, exp)
				case guard:
					refused++
				case !same:
					t.Errorf("%s %s %v: copy builds %v, source builds %v", path, port, exp, got, want)
				}
			}
		}
		// The generation (json v1) cannot copy the json v2 files: the
		// guard refuses GOEXPERIMENT=jsonv2 on every port.
		if wantRefused := map[string]int{"encoding/binary": 0, "encoding/json": len(testPorts)}[path]; refused != wantRefused {
			t.Errorf("%s: %d builds refused by the guard, want %d", path, refused, wantRefused)
		}
	}
}

// TestGlueSymbolsCheck checks that aotgen stops when the glue names a
// symbol that a supported build does not declare.
func TestGlueSymbolsCheck(t *testing.T) {
	expr := func(line string) constraint.Expr {
		x, err := constraint.Parse(line)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	files := map[string]constraint.Expr{
		"a.go":      nil,
		"little.go": expr("//go:build amd64 || arm64"),
		"big.go":    expr("//go:build s390x"),
	}
	decls := map[string][]string{
		"a.go":      {"F"},
		"little.go": {"Native", "native.Get"},
		"big.go":    {"Native", "native.Get"},
	}
	glue := []byte("ValueOf(F) ValueOf(&Native)")
	if err := checkGlueSymbols(glue, files, decls, nil, nil); err == nil || !strings.Contains(err.Error(), "Native") {
		t.Errorf("Native declared for 3 architectures only: err = %v", err)
	}
	// With a guard that refuses the other builds, it is fine.
	if err := checkGlueSymbols(glue, files, decls, expr("//go:build !amd64 && !arm64 && !s390x"), nil); err != nil {
		t.Errorf("with a guard: %v", err)
	}
	// A symbol that the glue does not name is not checked.
	if err := checkGlueSymbols([]byte("ValueOf(F)"), files, decls, nil, nil); err != nil {
		t.Errorf("symbol not in the glue: %v", err)
	}
}

// TestManyTags checks the constraints of a package with one file per
// GOEXPERIMENT value, as internal/goexperiment of Go 1.27 (20 experiments,
// exp_X_on.go and exp_X_off.go each). Before, aotgen tried every
// combination of the tags and stopped above 12: "too many build tags to
// check".
func TestManyTags(t *testing.T) {
	files := map[string]constraint.Expr{}
	decls := map[string][]string{}
	var glue strings.Builder
	for i := range 20 {
		tag := &constraint.TagExpr{Tag: fmt.Sprintf("goexperiment.exp%d", i)}
		files[fmt.Sprintf("exp%d_on.go", i)] = tag
		files[fmt.Sprintf("exp%d_off.go", i)] = &constraint.NotExpr{X: tag}
		name := fmt.Sprintf("Exp%d", i)
		decls[fmt.Sprintf("exp%d_on.go", i)] = []string{name}
		decls[fmt.Sprintf("exp%d_off.go", i)] = []string{name}
		fmt.Fprintf(&glue, "ValueOf(%s) ", name)
	}
	exists, err := packageExists(files, nil)
	if err != nil || exists != nil {
		t.Fatalf("packageExists = %v, %v; want every build", exists, err)
	}
	if err := checkGlueSymbols([]byte(glue.String()), files, decls, nil, nil); err != nil {
		t.Fatal(err)
	}
	// Without the off file of the last experiment, Exp19 is missing on
	// every port when the experiment is off.
	delete(files, "exp19_off.go")
	err = checkGlueSymbols([]byte(glue.String()), files, decls, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Exp19 (missing on aix/ppc64 map[goexperiment.exp19:false])") {
		t.Errorf("err = %v, want Exp19 missing", err)
	}
	// With a guard that refuses these builds, it is fine again.
	guard := &constraint.NotExpr{X: &constraint.TagExpr{Tag: "goexperiment.exp19"}}
	if err := checkGlueSymbols([]byte(glue.String()), files, decls, guard, nil); err != nil {
		t.Errorf("with a guard: %v", err)
	}
	// Only the on files: the package exists only when one is on.
	for i := range 20 {
		delete(files, fmt.Sprintf("exp%d_off.go", i))
	}
	exists, err = packageExists(files, nil)
	if err != nil || exists == nil {
		t.Fatalf("packageExists = %v, %v; want some builds", exists, err)
	}
	if err := checkGlueSymbols([]byte("ValueOf(Exp0)"), files, decls, nil, exists); err == nil {
		t.Errorf("Exp0 with the package on exp1 only: no error")
	}
}

// TestCounterexample checks the builds that counterexample finds.
func TestCounterexample(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"//go:build linux || !linux", ""},
		{"//go:build unix || windows || plan9 || js || wasip1", ""},
		{"//go:build cgo || !cgo", ""},
		{"//go:build amd64 || (!amd64 && purego) || !purego", ""},
		{"//go:build linux", "aix/ppc64 map[]"},
		{"//go:build !s390x || !purego", "linux/s390x map[purego:true]"},
		{"//go:build (cgo && amd64.v3) || !amd64", "android/amd64 map[cgo:false]"},
	} {
		x, err := constraint.Parse(c.line)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if b, ok := counterexample(x); ok {
			got = fmt.Sprintf("%s/%s %v", b.goos, b.goarch, b.tags)
		}
		if got != c.want {
			t.Errorf("%s: counterexample %q, want %q", c.line, got, c.want)
		}
	}
}

// TestKeptConstraint checks which part of a build constraint a copy keeps.
func TestKeptConstraint(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"//go:build !goexperiment.jsonv2", "!goexperiment.jsonv2"},
		{"//go:build go1.20", ""},
		{"//go:build s390x && !purego", "s390x"},
		{"//go:build (linux || windows) && go1.18", "linux || windows"},
		{"// +build go1.9", ""},
		{"//go:build ignore", "false"},
		{"//go:build !go1.20", "false"},
		{"//go:build cmp_debug", "false"},
	} {
		f, err := parser.ParseFile(token.NewFileSet(), "x.go", c.line+"\n\npackage p\n", parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		kept, ok, err := keptConstraint(f, genTag)
		got := ""
		switch {
		case err != nil:
			got = err.Error()
		case !ok:
			got = "false"
		case kept != nil:
			got = kept.String()
		}
		if got != c.want {
			t.Errorf("%s: kept %q, want %q", c.line, got, c.want)
		}
	}
}
