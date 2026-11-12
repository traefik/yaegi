// Command runtests runs the yaegi conformance programs (tests/yaegi/_test)
// through the VM and compares them with their expectation, using the same
// convention as yaegi's TestFile: the last comment of a program is
// "Output:" (stdout, trimmed) or "Error:" (substring of the error message).
//
// Programs are run by the yaegi binary, one process per program, inside a
// temporary copy of _test/ placed in a module named github.com/traefik/yaegi,
// so that imports of local packages ("github.com/traefik/yaegi/_test/foo")
// resolve as they do in a yaegi checkout. Programs with a "GOPATH:" prefix run
// in GOPATH mode with that GOPATH, as yaegi's TestFile does.
//
// Usage:
//
//	go run ./cmd/runtests [-out results/conformance.tsv] [-summary results/conformance.txt] [-run regexp]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "runtests:", err)
		os.Exit(1)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := FindRoot(wd)
	if err != nil {
		return err
	}
	dir := flag.String("dir", filepath.Join(root, "tests", "yaegi", "_test"), "directory of the conformance programs")
	vmBin := flag.String("vm", "", "yaegi binary (default: build ./cmd/yaegi into a temporary directory)")
	vmArgs := flag.String("args", "", "extra yaegi flags, space separated")
	out := flag.String("out", filepath.Join(root, "results", "conformance.tsv"), "per-program TSV output")
	summary := flag.String("summary", filepath.Join(root, "results", "conformance.txt"), "summary output (also printed)")
	filter := flag.String("run", "", "only run programs whose file name matches this regexp")
	jobs := flag.Int("j", runtime.NumCPU(), "parallel programs")
	timeout := flag.Duration("timeout", 30*time.Second, "timeout per program")
	goVersion := flag.String("go", "1.22", "go directive of the temporary module (1.22+: per-iteration loop variables, as yaegi implements)")
	flag.Parse()

	tmp, err := os.MkdirTemp("", "yaegi-runtests-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if *vmBin == "" {
		*vmBin = filepath.Join(tmp, "yaegi")
		if err := BuildVM(root, *vmBin); err != nil {
			return err
		}
	}
	wsTest, cleanup, err := PrepareWorkspace(*dir, *goVersion)
	if err != nil {
		return err
	}
	defer cleanup()

	files, err := Discover(wsTest)
	if err != nil {
		return err
	}
	if *filter != "" {
		re, err := regexp.Compile(*filter)
		if err != nil {
			return err
		}
		var kept []string
		for _, f := range files {
			if re.MatchString(filepath.Base(f)) {
				kept = append(kept, f)
			}
		}
		files = kept
	}
	cfg := Config{TestDir: *dir, VM: *vmBin, VMArgs: strings.Fields(*vmArgs), Jobs: *jobs, Timeout: *timeout, GoVersion: *goVersion}
	t0 := time.Now()
	results := Run(cfg, wsTest, files, ReadOrigins(filepath.Join(filepath.Dir(*dir), "MANIFEST.tsv")))
	elapsed := time.Since(t0)

	for _, p := range []string{*out, *summary} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	if err := WriteTSV(f, results); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "yaegi v2 conformance run: %s, %s, go directive %s, %d jobs, %v\n\n", time.Now().Format("2006-01-02"), runtime.Version(), *goVersion, *jobs, elapsed.Round(time.Second))
	WriteSummary(&sb, results)
	fmt.Print(sb.String())
	return os.WriteFile(*summary, []byte(sb.String()), 0o644)
}
