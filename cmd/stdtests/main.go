// Command stdtests runs the external tests of standard library packages
// (the _test.go files of package xxx_test in GOROOT) under yaegi v2, with the
// native package as the code under test. The tests of the Go team become an
// oracle for the values of the program that go through fmt, sort, io,
// encoding/json...: a test counts as a yaegi v2 failure only when it passes
// with the native toolchain on the same machine.
//
// For each package, it writes a program (see prepare.go) and runs it three
// ways: compiled by the Go toolchain (native), by yaegi v2 in VM mode (vm),
// and by yaegi v2 in AOT mode (aot). It records the result of each TestXxx,
// FuzzXxx (its f.Add seeds) and ExampleXxx (compared with its Output:
// comment by package testing). See docs/STDTESTS.md.
//
// Usage:
//
//	go run ./cmd/stdtests [-pkgs fmt,sort] [-modes native,vm,aot] [-out f.tsv] [-summary f.txt] [-check docs/conformance/stdlib.tsv]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// defaultPkgs are the packages of the first study (October 2026).
var defaultPkgs = []string{
	"fmt", "sort", "slices", "maps", "io", "bufio", "bytes", "strings", "strconv",
	"errors", "encoding/json", "encoding/xml", "flag", "context", "math/big", "text/template",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "stdtests:", err)
		os.Exit(1)
	}
}

// A Row is the result of one test in every mode.
type Row struct {
	Pkg, Test string
	Status    map[string]string // mode -> status
	Reason    string
}

func run() error {
	pkgsFlag := flag.String("pkgs", strings.Join(defaultPkgs, ","), "comma-separated packages")
	modesFlag := flag.String("modes", "native,vm,aot", "comma-separated modes: native, vm, aot")
	out := flag.String("out", "", "per-test TSV output")
	summary := flag.String("summary", "", "summary output (also printed)")
	check := flag.String("check", "", "checked-in TSV: fail when a test that passes there in vm or aot mode (and natively) does not pass now while it passes natively")
	work := flag.String("work", "", "work directory for the programs (default: cmd/stdtests/_work of the module)")
	jobs := flag.Int("j", max(1, runtime.NumCPU()/4), "packages run in parallel")
	timeout := flag.Duration("timeout", 3*time.Minute, "-test.timeout of one process (a test that runs longer is a timeout)")
	short := flag.Bool("short", true, "run the tests with -test.short, as the builders of Go do")
	filter := flag.String("run", "", "only run the tests whose name matches this regexp")
	verbose := flag.Bool("v", false, "print each process run")
	causesFile := flag.String("causes", "", "known causes of the yaegi v2 failures (default: docs/conformance/stdlib-causes.tsv of the module)")
	flag.Parse()

	root, err := findRoot()
	if err != nil {
		return err
	}
	goroot, err := goEnv("GOROOT")
	if err != nil {
		return err
	}
	goVersion, err := goEnv("GOVERSION")
	if err != nil {
		return err
	}
	if *work == "" {
		*work = filepath.Join(root, "cmd", "stdtests", "_work")
	}
	// The programs import packages of this module: the work directory must
	// be inside it (a directory starting with "_" is not part of ./...).
	if rel, err := filepath.Rel(root, *work); err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("-work must be inside the module %s", root)
	}
	if err := os.MkdirAll(*work, 0o755); err != nil {
		return err
	}
	if *causesFile == "" {
		*causesFile = filepath.Join(root, "docs", "conformance", "stdlib-causes.tsv")
	}
	causes, err := readCauses(*causesFile)
	if err != nil {
		return err
	}
	modes := strings.Split(*modesFlag, ",")
	if *check != "" && !contains(modes, ModeNative) {
		// A regression is a test that passes natively and fails under
		// yaegi v2: without the native run, -check could never find one.
		return fmt.Errorf("-check needs the native mode in -modes")
	}
	var re *regexp.Regexp
	if *filter != "" {
		if re, err = regexp.Compile(*filter); err != nil {
			return err
		}
	}

	r := &runner{timeout: *timeout, short: *short, verbose: *verbose}
	if contains(modes, ModeVM) || contains(modes, ModeAOT) {
		r.vmhost = filepath.Join(*work, "vmhost")
		if runtime.GOOS == "windows" {
			r.vmhost += ".exe" // exec.Command finds no file without it
		}
		cmd := exec.Command("go", "build", "-o", r.vmhost, "./cmd/stdtests/vmhost")
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build vmhost: %v\n%s", err, b)
		}
	}

	pkgs := strings.Split(*pkgsFlag, ",")
	rows := make([][]Row, len(pkgs))
	progsList := make([]*Program, len(pkgs))
	errs := make([]error, len(pkgs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *jobs)
	for i, pkg := range pkgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			start := time.Now()
			rows[i], progsList[i], errs[i] = runPackage(r, root, goroot, pkg, filepath.Join(*work, strings.ReplaceAll(pkg, "/", "_")), modes, re)
			fmt.Fprintf(os.Stderr, "stdtests: %s done in %v\n", pkg, time.Since(start).Round(time.Second))
		}()
	}
	wg.Wait()
	// A package whose program cannot be made or built is reported, and the
	// others are still written: the command fails at the end.
	var all []Row
	pkgErrs := map[string]error{}
	progs := map[string]*Program{}
	for i, err := range errs {
		if err != nil {
			pkgErrs[pkgs[i]] = err
			fmt.Fprintf(os.Stderr, "stdtests: %v\n", err)
		}
		all = append(all, rows[i]...)
		progs[pkgs[i]] = progsList[i]
	}
	for i := range all {
		classify(&all[i], modes, causes)
	}

	if *out != "" {
		if err := writeTSV(*out, modes, all); err != nil {
			return err
		}
	}
	var sum strings.Builder
	writeSummary(&sum, goVersion, modes, pkgs, all, pkgErrs)
	fmt.Print(sum.String())
	if *summary != "" {
		if err := os.WriteFile(*summary, []byte(sum.String()), 0o644); err != nil {
			return err
		}
	}
	if *check != "" {
		if err := checkRegressions(*check, modes, all, re, func(pkg, test, mode string) bool {
			prog := progs[pkg]
			return prog != nil && r.run(prog, mode, []string{test})[test].Status == StatusPass
		}); err != nil {
			return err
		}
	}
	if len(pkgErrs) > 0 {
		return fmt.Errorf("%d packages could not run", len(pkgErrs))
	}
	return nil
}

// runPackage prepares and runs the program of pkg in each mode.
func runPackage(r *runner, root, goroot, pkg, dir string, modes []string, re *regexp.Regexp) ([]Row, *Program, error) {
	prog, err := prepare(root, goroot, pkg, dir)
	if err != nil {
		return nil, nil, err
	}
	var names []string
	for _, n := range prog.Names() {
		if re == nil || re.MatchString(n) {
			names = append(names, n)
		}
	}
	res := map[string]map[string]Outcome{}
	for _, mode := range modes {
		if mode == ModeNative {
			if err := buildNative(prog); err != nil {
				return nil, nil, fmt.Errorf("%s: %v", pkg, err)
			}
		}
		res[mode] = r.run(prog, mode, names)
	}
	var rows []Row
	for _, n := range names {
		row := Row{Pkg: pkg, Test: n, Status: map[string]string{}}
		for _, mode := range modes {
			row.Status[mode] = res[mode][n].Status
		}
		row.Reason = reason(modes, res, n)
		rows = append(rows, row)
	}
	for n, why := range prog.Excluded {
		if re != nil && !re.MatchString(n) {
			continue
		}
		row := Row{Pkg: pkg, Test: n, Status: map[string]string{}, Reason: why}
		for _, mode := range modes {
			row.Status[mode] = StatusExcluded
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Test < rows[j].Test })
	return rows, prog, nil
}

// reason explains the row: the first yaegi v2 mode that does not pass while
// native passes, else what the native run says (a skip message, a failure).
func reason(modes []string, res map[string]map[string]Outcome, n string) string {
	nat, hasNat := res[ModeNative][n]
	for _, mode := range modes {
		if mode == ModeNative {
			continue
		}
		o := res[mode][n]
		if o.Status != StatusPass && (!hasNat || nat.Status == StatusPass) {
			return mode + ": " + o.Detail
		}
	}
	if hasNat && nat.Status != StatusPass {
		return "native: " + nat.Detail
	}
	return ""
}

func writeTSV(path string, modes []string, rows []Row) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "package\ttest\t%s\treason\n", strings.Join(modes, "\t"))
	for _, r := range rows {
		cols := []string{r.Pkg, r.Test}
		for _, m := range modes {
			cols = append(cols, r.Status[m])
		}
		cols = append(cols, r.Reason)
		fmt.Fprintln(w, strings.Join(cols, "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return f.Close()
}

// writeSummary writes one line per package and a total: the tests run
// (not excluded), the native passes, and for vm and aot the passes among
// the native passes.
func writeSummary(w io.Writer, goVersion string, modes []string, pkgs []string, rows []Row, pkgErrs map[string]error) {
	fmt.Fprintf(w, "# cmd/stdtests, %s %s/%s, -test.short\n", goVersion, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "# package: run (excluded), native pass; vm and aot: pass / native pass\n")
	byPkg := map[string][]Row{}
	for _, r := range rows {
		byPkg[r.Pkg] = append(byPkg[r.Pkg], r)
	}
	line := func(name string, rows []Row) {
		var run, excl, nat int
		pass := map[string]int{}
		for _, r := range rows {
			if r.Status[modes[0]] == StatusExcluded {
				excl++
				continue
			}
			run++
			if contains(modes, ModeNative) && r.Status[ModeNative] != StatusPass {
				continue
			}
			nat++
			for _, m := range modes {
				if m != ModeNative && r.Status[m] == StatusPass {
					pass[m]++
				}
			}
		}
		fmt.Fprintf(w, "%-16s run %4d (excluded %3d), native pass %4d", name+":", run, excl, nat)
		for _, m := range modes {
			if m != ModeNative {
				fmt.Fprintf(w, ", %s %4d / %d", m, pass[m], nat)
			}
		}
		fmt.Fprintln(w)
	}
	for _, p := range pkgs {
		if err := pkgErrs[p]; err != nil {
			first, _, _ := strings.Cut(err.Error(), "\n")
			fmt.Fprintf(w, "%-16s error: %s\n", p+":", first)
			continue
		}
		line(p, byPkg[p])
	}
	line("total", rows)

	// The yaegi v2 failures by class (docs/conformance/stdlib-causes.tsv).
	classes := map[string]int{}
	for _, r := range rows {
		if strings.HasPrefix(r.Reason, "[") {
			c, _, _ := strings.Cut(strings.TrimPrefix(r.Reason, "["), "]")
			c, _, _ = strings.Cut(c, " ")
			classes[c]++
		}
	}
	var names []string
	for c := range classes {
		names = append(names, c)
	}
	sort.Strings(names)
	var parts []string
	for _, c := range names {
		parts = append(parts, fmt.Sprintf("%s %d", c, classes[c]))
	}
	fmt.Fprintf(w, "yaegi v2 failures by class (tests, vm or aot): %s\n", strings.Join(parts, ", "))
}

// checkRegressions compares rows with a checked-in TSV: a test that passes
// there natively and in a yaegi v2 mode must still pass in that mode when it
// passes natively now. A test that does not pass runs once more, alone
// (again): a slow runner must not turn a pass into a timeout.
func checkRegressions(path string, modes []string, rows []Row, re *regexp.Regexp, again func(pkg, test, mode string) bool) error {
	old, err := readTSV(path)
	if err != nil {
		return err
	}
	now := map[string]Row{}
	for _, r := range rows {
		now[r.Pkg+"\t"+r.Test] = r
	}
	var bad []string
	for _, o := range old {
		if re != nil && !re.MatchString(o.Test) {
			continue
		}
		n, ok := now[o.Pkg+"\t"+o.Test]
		if !ok || o.Status[ModeNative] != StatusPass || n.Status[ModeNative] != StatusPass {
			continue
		}
		for _, m := range modes {
			if m == ModeNative || o.Status[m] != StatusPass {
				continue
			}
			if n.Status[m] != StatusPass {
				if again(o.Pkg, o.Test, m) {
					fmt.Fprintf(os.Stderr, "stdtests: %s %s (%s): %s, then pass when run alone\n", o.Pkg, o.Test, m, n.Status[m])
					continue
				}
				bad = append(bad, fmt.Sprintf("%s %s (%s): %s: %s", o.Pkg, o.Test, m, n.Status[m], n.Reason))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%d regressions against %s:\n  %s", len(bad), path, strings.Join(bad, "\n  "))
	}
	fmt.Fprintf(os.Stderr, "stdtests: no regression against %s\n", path)
	return nil
}

func readTSV(path string) ([]Row, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s: empty", path)
	}
	head := strings.Split(lines[0], "\t")
	var rows []Row
	for _, l := range lines[1:] {
		cols := strings.Split(l, "\t")
		if len(cols) != len(head) {
			return nil, fmt.Errorf("%s: bad line %q", path, l)
		}
		r := Row{Pkg: cols[0], Test: cols[1], Status: map[string]string{}, Reason: cols[len(cols)-1]}
		for i := 2; i < len(head)-1; i++ {
			r.Status[head[i]] = cols[i]
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.HasPrefix(string(b), "module "+modulePath+"\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in the module %s", modulePath)
		}
		dir = parent
	}
}

func goEnv(name string) (string, error) {
	b, err := exec.Command("go", "env", name).Output()
	if err != nil {
		return "", fmt.Errorf("go env %s: %v", name, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// readCauses reads the known causes of the yaegi v2 failures: package, test,
// class, issues (see docs/conformance/stdlib-causes.tsv).
func readCauses(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		c := strings.Split(l, "\t")
		if len(c) != 4 {
			return nil, fmt.Errorf("%s: bad line %q", path, l)
		}
		cause := c[2]
		if c[3] != "-" {
			cause += " " + c[3]
		}
		out[c[0]+"\t"+c[1]] = cause
	}
	return out, nil
}

// classify writes the cause of a yaegi v2 failure at the start of the reason:
// the class and issues of the causes file, or "new".
func classify(r *Row, modes []string, causes map[string]string) {
	if contains(modes, ModeNative) && r.Status[ModeNative] != StatusPass {
		return
	}
	for _, m := range modes {
		if m != ModeNative && r.Status[m] != StatusPass {
			cause, ok := causes[r.Pkg+"\t"+r.Test]
			if !ok {
				cause = "new"
			}
			r.Reason = "[" + cause + "] " + r.Reason
			return
		}
	}
}
