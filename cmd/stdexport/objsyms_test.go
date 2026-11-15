package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestMain makes the test binary the toolexec child of toolexecSymbols, as
// main does for the command.
func TestMain(m *testing.M) {
	if dir := os.Getenv(toolexecEnv); dir != "" {
		os.Exit(toolexec(dir, os.Getenv(nmEnv), os.Args[1:]))
	}
	os.Exit(m.Run())
}

// TestToolexecSymbols checks the path that Go 1.28 takes (go list -export
// gives export data only, objsyms.go) against the path of the releases of
// the window (the export file is the object file): both see the same init
// task symbols, for a few packages with and without init work.
func TestToolexecSymbols(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles packages again (go build -a)")
	}
	nmPath, err := exec.Command("go", "tool", "-n", "nm").Output()
	if err != nil {
		t.Fatalf("go tool -n nm: %v", err)
	}
	nm := strings.TrimSpace(string(nmPath))
	patterns := []string{"errors", "unicode/utf8", "fmt", "os"}
	env := append(os.Environ(), "CGO_ENABLED=0")
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedExportFile, Env: env}, patterns...)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != len(patterns) {
		t.Fatalf("loaded %d packages, want %d", len(pkgs), len(patterns))
	}
	for _, p := range pkgs {
		ok, err := isArchive(p.ExportFile)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Skipf("%s: the export file is not an object file (Go 1.28): nothing to compare with", p.PkgPath)
		}
	}
	fromList, err := archiveSymbols(nm, pkgs)
	if err != nil {
		t.Fatal(err)
	}
	fromBuild, err := toolexecSymbols(nm, env, pkgs, patterns)
	if err != nil {
		t.Fatal(err)
	}
	inittasks := func(out string) []string {
		var l []string
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && strings.HasSuffix(f[len(f)-1], "..inittask") {
				l = append(l, f[len(f)-2]+" "+f[len(f)-1])
			}
		}
		slices.Sort(l)
		return l
	}
	for _, p := range pkgs {
		a, b := inittasks(fromList[p.PkgPath]), inittasks(fromBuild[p.PkgPath])
		if !slices.Equal(a, b) {
			t.Errorf("%s: init task symbols from go build -toolexec %q, from go list -export %q", p.PkgPath, b, a)
		}
		// fmt and os have init work, and fmt imports os.
		if p.PkgPath == "fmt" && !slices.Contains(b, "U os..inittask") {
			t.Errorf("fmt: no reference to the init task of os in %q", b)
		}
	}
}

// TestCompileOutputs checks which file the toolexec child gives to nm, for
// the compile command lines of the releases of the window and of Go 1.28
// (tip), where -o is the export data only and -linkobj the object file.
func TestCompileOutputs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		pkg, obj string
	}{
		{
			name: "go1.27",
			args: strings.Fields("-o $WORK/b006/_pkg_.a -trimpath $WORK/b006=> -p internal/goarch -lang=go1.27 -std -complete -buildid x/x -goversion go1.27.3 -c=4 -nolocalimports -importcfg $WORK/b006/importcfg -pack ./goarch.go ./zgoarch_amd64.go"),
			pkg:  "internal/goarch",
			obj:  "$WORK/b006/_pkg_.a",
		},
		{
			// cmd/go/internal/work/gc.go on tip: "-o", export, "-linkobj", ofile
			name: "tip",
			args: strings.Fields("-o $WORK/export/b006.a -linkobj $WORK/b006/_pkg_.a -trimpath $WORK/b006=> -p internal/goarch -lang=go1.28 -std -complete -buildid x/x -goversion devel -c=4 -nolocalimports -importcfg $WORK/b006/importcfg -pack ./goarch.go"),
			pkg:  "internal/goarch",
			obj:  "$WORK/b006/_pkg_.a",
		},
		{
			name: "tip, -linkobj first",
			args: strings.Fields("-linkobj $WORK/b006/_pkg_.a -p fmt -o $WORK/export/b006.a ./print.go"),
			pkg:  "fmt",
			obj:  "$WORK/b006/_pkg_.a",
		},
		{
			name: "flag=value",
			args: strings.Fields("-o=$WORK/export/b006.a -linkobj=$WORK/b006/_pkg_.a -p=fmt ./print.go"),
			pkg:  "fmt",
			obj:  "$WORK/b006/_pkg_.a",
		},
		{
			name: "compile -V=full",
			args: []string{"-V=full"},
		},
		{
			name: "-o without a value",
			args: []string{"-p", "fmt", "-o"},
			pkg:  "fmt",
		},
	}
	for _, c := range cases {
		pkg, obj := compileOutputs(c.args)
		if pkg != c.pkg || obj != c.obj {
			t.Errorf("%s: compileOutputs = %q, %q, want %q, %q", c.name, pkg, obj, c.pkg, c.obj)
		}
	}
}

// TestRunNMNoSymbols checks that a file without symbols (the export data
// of Go 1.28 when -linkobj is missing) gives an empty nm output, not an
// error.
func TestRunNMNoSymbols(t *testing.T) {
	nmPath, err := exec.Command("go", "tool", "-n", "nm").Output()
	if err != nil {
		t.Fatalf("go tool -n nm: %v", err)
	}
	nm := strings.TrimSpace(string(nmPath))
	// An empty archive: go tool nm reads it and finds no symbols.
	f := filepath.Join(t.TempDir(), "export.a")
	if err := os.WriteFile(f, []byte("!<arch>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(nm, f).CombinedOutput(); err == nil || !strings.Contains(string(out), "no symbols") {
		t.Skipf("go tool nm on an empty archive: %v, %q (not the case of this test)", err, out)
	}
	out, err := runNM(nm, f)
	if err != nil || out != "" {
		t.Errorf("runNM on a file without symbols = %q, %v, want \"\", nil", out, err)
	}
}

// TestInitTasksNoSymbolsAnywhere checks that an nm that finds no symbols in
// any file gives an error, not an empty init graph: fmt has an init task.
func TestInitTasksNoSymbolsAnywhere(t *testing.T) {
	nm := filepath.Join(t.TempDir(), "nm")
	script := "#!/bin/sh\necho \"reading $1: no symbols\" >&2\nexit 1\n"
	if err := os.WriteFile(nm, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tasks, err := initTasks(nm, os.Environ(), "fmt")
	if err == nil {
		t.Fatalf("initTasks = %d tasks, nil error; want an error", len(tasks))
	}
}
