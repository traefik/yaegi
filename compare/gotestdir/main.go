//go:build unix

// Command gotestdir runs the tests of GOROOT/test (the official Go
// conformance suite) against one engine: go, govm (yaegi v2, the engine key
// of the measurements) or yaegi (yaegi v0).
//
// It follows the rules of cmd/internal/testdir:
//   - the recipe is the first non-empty line that is not a build constraint;
//   - files whose build constraints do not match linux/amd64 are skipped;
//   - run-like actions (run, rundir, runindir, runoutput, buildrun,
//     buildrundir, the run part of errorcheckandrundir): exit 0 and
//     stdout+stderr identical to the .out file (empty if none). Extra .go
//     files in a run recipe ("// run cmplxdivide1.go") are part of the
//     program, as with go run;
//   - compile-like actions (compile, compiledir, build, builddir): the
//     packages compile (go) or load and build their SSA code (govm, with
//     the loadcheck command: yaegi v2 has no "load only" mode);
//   - errorcheck-like actions (errorcheck, errorcheckdir, errorcheckoutput,
//     errorcheckwithauto, the first part of errorcheckandrundir): see
//     checkErrors. Only the tests without compiler flags are counted;
//   - the tests of a directory (*dir actions) are packages compiled in the
//     order of their file names, the main package last. Interpreters get the
//     packages in a module "test", one directory per package, with the
//     import paths "./a" written "test/a": the import path that gc gives
//     them with -D test.
//
// Compiler-only flags (-gcflags, -ldflags, -race, -tags, -l=4...) are
// ignored for the interpreters; -goexperiment and -godebug add to the
// environment as testdir does, and -t sets the timeout of the test.
//
// Every command runs in its own process group. On timeout the whole group is
// killed, so no grandchild (the binary started by "go run") keeps running.
//
// Local paths (test directory, GOROOT, GOMODCACHE, repository, $HOME, /tmp)
// are removed from the details before they are cut, so the results can be
// committed.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/build/constraint"
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"
)

var dirs = []string{".", "ken", "chan", "interface", "internal/runtime/sys", "syntax", "fixedbugs", "abi", "typeparam", "typeparam/mdempsky"}

// rejectCode is the exit code of each engine when it refuses a program at
// compile time (parse or type error). Any other non-zero code is a crash or
// a runtime failure.
// For go, it is the exit code of "go tool compile".
var rejectCode = map[string]int{"go": 2, "govm": 4, "yaegi": 1}

// The actions of testdir that gotestdir runs, by kind. asmcheck (assembly
// of gc) and skip are not run.
var (
	runLike     = setOf("run", "rundir", "runindir", "runoutput", "buildrun", "buildrundir")
	compileLike = setOf("compile", "compiledir", "build", "builddir")
	errorLike   = setOf("errorcheck", "errorcheckdir", "errorcheckoutput", "errorcheckwithauto", "errorcheckandrundir")
)

func setOf(s ...string) map[string]bool {
	m := map[string]bool{}
	for _, k := range s {
		m[k] = true
	}
	return m
}

type result struct {
	file, action, status, detail string
	dur                          time.Duration
}

type runner struct {
	root, engine, bin string
	gobin             string // the go command: reference engine, and generator of runoutput tests
	load              string // loadcheck binary: compile-like tests of govm (empty: not run)
	timeout           time.Duration
	importcfg         string // go engine: stdlib export data for the compiler
	stdcfg            string // go engine: the content of importcfg
	actions           map[string]bool
}

// testEnv is the environment of testdir for the go command: no go.env file,
// no GOFLAGS (yaegi v2's own -mod=mod must not reach the tests).
var testEnv = []string{"GOENV=off", "GOFLAGS="}

// checkNoModule refuses a test directory inside a module. testdir runs the
// tests in GOROOT/test, which is not in a module: the go command then uses
// the defaults of the toolchain (language version, GODEBUG). Inside yaegi v2's
// checkout, yaegi v2's go.mod would set them instead.
func checkNoModule(root string) error {
	c := exec.Command("go", "env", "GOMOD")
	c.Dir = root
	c.Env = append(os.Environ(), testEnv...)
	out, err := c.Output()
	if err != nil {
		return fmt.Errorf("go env GOMOD: %v", err)
	}
	if m := strings.TrimSpace(string(out)); m != "" && m != os.DevNull {
		return fmt.Errorf("the test directory is inside the module of %s: its settings would change the results; use a copy of the tests outside any module", clean(m))
	}
	return nil
}

// writeImportcfg writes the import configuration of the standard library,
// as testdir does (go list -export std).
func writeImportcfg(gobin, file string) (string, error) {
	c := exec.Command(gobin, "list", "-export", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", "std")
	c.Env = append(os.Environ(), testEnv...)
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("go list -export std: %v", err)
	}
	return string(out), os.WriteFile(file, out, 0o644)
}

func main() {
	root := flag.String("root", "", "GOROOT/test directory")
	engine := flag.String("engine", "go", "go | govm | yaegi")
	bin := flag.String("bin", "", "engine binary")
	gobin := flag.String("go", "go", "go command (generator of runoutput and errorcheckoutput tests)")
	load := flag.String("load", "", "govm: loadcheck binary for the compile-like tests (compile, compiledir, build, builddir); empty: they are not run")
	jobs := flag.Int("j", 8, "parallel tests")
	timeout := flag.Duration("timeout", 30*time.Second, "per test")
	only := flag.String("only", "", "regexp on the file name")
	actions := flag.String("actions", "all", "comma-separated actions to run (all: every action gotestdir knows)")
	flag.Parse()
	if _, ok := rejectCode[*engine]; !ok {
		fmt.Fprintln(os.Stderr, "gotestdir: unknown engine", *engine)
		os.Exit(2)
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotestdir:", err)
		os.Exit(2)
	}
	r := runner{root: abs, engine: *engine, bin: *bin, gobin: *gobin, load: *load, timeout: *timeout, actions: map[string]bool{}}
	if *engine == "go" {
		r.gobin = *bin
	}
	for _, a := range strings.Split(*actions, ",") {
		if a == "all" {
			for _, s := range []map[string]bool{runLike, compileLike, errorLike} {
				for k := range s {
					r.actions[k] = true
				}
			}
			continue
		}
		if !runLike[a] && !compileLike[a] && !errorLike[a] {
			fmt.Fprintln(os.Stderr, "gotestdir: unknown action", a)
			os.Exit(2)
		}
		r.actions[a] = true
	}
	initCleaner(abs)
	if err := checkNoModule(abs); err != nil {
		fmt.Fprintln(os.Stderr, "gotestdir:", err)
		os.Exit(2)
	}
	if *engine == "go" {
		// The compiler alone, as testdir does: it needs the export data
		// of the standard library.
		tmp, err := os.MkdirTemp("", "gotestdir")
		if err == nil {
			defer os.RemoveAll(tmp)
			r.importcfg = filepath.Join(tmp, "importcfg")
			r.stdcfg, err = writeImportcfg(*bin, r.importcfg)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "gotestdir:", err)
			os.Exit(2)
		}
	}

	var onlyRx *regexp.Regexp
	if *only != "" {
		onlyRx = regexp.MustCompile(*only)
	}
	var files []string
	for _, d := range dirs {
		m, _ := filepath.Glob(filepath.Join(abs, d, "*.go"))
		for _, f := range m {
			rel, _ := filepath.Rel(abs, f)
			if onlyRx == nil || onlyRx.MatchString(rel) {
				files = append(files, rel)
			}
		}
	}
	sort.Strings(files)

	res := make([]result, len(files))
	sem := make(chan struct{}, *jobs)
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res[i] = r.runOne(f)
		}()
	}
	wg.Wait()
	for _, r := range res {
		if r.status == "" {
			continue
		}
		d := strings.ReplaceAll(strings.ReplaceAll(r.detail, "\t", " "), "\n", " | ")
		fmt.Printf("%s\t%s\t%s\t%.2f\t%s\n", r.file, r.action, r.status, r.dur.Seconds(), cut(d, 300))
	}
}

func recipe(src string) (string, bool) {
	header, _, ok := strings.Cut(src, "\npackage")
	if !ok {
		header = src
	}
	for _, line := range strings.Split(header, "\n") {
		if expr, err := constraint.Parse(line); err == nil {
			if !expr.Eval(func(tag string) bool {
				switch tag {
				case "linux", "amd64", "unix", "gc", "cgo", "goexperiment.swissmap", "goexperiment.aliastypeparams":
					return true
				}
				if strings.HasPrefix(tag, "go1.") {
					n, _ := strconv.Atoi(tag[4:])
					return n <= 26
				}
				return false
			}) {
				return "", false
			}
		}
	}
	for s := src; s != ""; {
		var line string
		line, s, _ = strings.Cut(s, "\n")
		if constraint.IsGoBuild(line) || constraint.IsPlusBuild(line) {
			continue
		}
		if a := strings.TrimSpace(strings.TrimPrefix(line, "//")); a != "" {
			return a, true
		}
	}
	return "", true
}

// needsTestdir lists run tests that call the compiler themselves with the
// environment of cmd/internal/testdir: they fail under plain "go run" too.
var needsTestdir = map[string]bool{"fixedbugs/bug369.go": true, "fixedbugs/issue21317.go": true, "linkobj.go": true}

// test is one test being run.
type test struct {
	*runner
	file, long, src string // path relative to the root, absolute path, source
	action          string
	flags, args     []string
	env             []string // -goexperiment and -godebug
	wantError       bool
	single          bool // -s: each file of a directory is a package
	gomodvers       string
	deadline        time.Time
	dur             time.Duration
	tmp             string
}

func (rn *runner) runOne(file string) result {
	r := result{file: file}
	srcB, err := os.ReadFile(filepath.Join(rn.root, file))
	if err != nil {
		return r
	}
	src := string(srcB)
	act, ok := recipe(src)
	if !ok {
		return r // build constraints exclude linux/amd64
	}
	f := splitQuoted(act)
	if len(f) == 0 {
		return r
	}
	t := &test{runner: rn, file: file, long: filepath.Join(rn.root, file), src: src, action: f[0]}
	args := f[1:]
	if !rn.actions[t.action] {
		return r
	}
	switch t.action {
	case "errorcheck", "errorcheckdir", "errorcheckoutput", "errorcheckwithauto":
		t.wantError = true
	}
	timeout := rn.timeout
	goexp, godebug := os.Getenv("GOEXPERIMENT"), os.Getenv("GODEBUG")
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-0":
			t.wantError = false
		case "-1":
			t.wantError = true
		case "-s":
			t.single = true
		case "-t":
			// The timeout of the test in seconds, as in testdir.
			args = args[1:]
			if n, err := strconv.Atoi(args[0]); err == nil {
				timeout = time.Duration(n) * time.Second
			}
		case "-goexperiment":
			// Add to the current value, as testdir does.
			args = args[1:]
			if goexp != "" {
				goexp += ","
			}
			goexp += args[0]
			t.env = append(t.env, "GOEXPERIMENT="+goexp)
		case "-godebug":
			args = args[1:]
			if godebug != "" {
				godebug += ","
			}
			godebug += args[0]
			t.env = append(t.env, "GODEBUG="+godebug)
		case "-gomodversion":
			args = args[1:]
			t.gomodvers = args[0]
		default:
			t.flags = append(t.flags, args[0])
		}
		args = args[1:]
	}
	t.args = args
	if errorLike[t.action] {
		// Keep only plain type-checking tests: compiler flags (-m, -d=, -live,
		// -lang...) check gc diagnostics, not the language. Without -1, the
		// errorcheck part of errorcheckandrundir checks only such
		// diagnostics (-0 -m).
		var rest []string
		for _, f := range t.flags {
			if f != "-e" {
				rest = append(rest, f)
			}
		}
		if !t.wantError || len(rest) > 0 {
			return r
		}
		if (t.action == "errorcheck" || t.action == "errorcheckwithauto") && len(args) > 0 {
			return r
		}
		t.flags = nil
	}
	if needsTestdir[file] {
		return r
	}
	if compileLike[t.action] && rn.engine != "go" && rn.load == "" {
		return r
	}
	r.action = t.action

	tmp, err := os.MkdirTemp("", "gotestdir")
	if err != nil {
		r.status, r.detail = "FAIL_EXIT", err.Error()
		return r
	}
	defer os.RemoveAll(tmp)
	t.tmp = tmp
	t.deadline = time.Now().Add(timeout)
	switch t.action {
	case "run":
		r.status, r.detail = t.run()
	case "errorcheck", "errorcheckwithauto":
		r.status, r.detail = t.errorcheck()
	case "compile", "build":
		r.status, r.detail = t.compile()
	case "compiledir":
		r.status, r.detail = t.compiledir()
	case "errorcheckdir", "errorcheckandrundir", "rundir":
		r.status, r.detail = t.dir()
	case "runindir":
		r.status, r.detail = t.runindir()
	case "runoutput", "errorcheckoutput":
		r.status, r.detail = t.output()
	case "buildrun":
		r.status, r.detail = t.buildrun()
	case "buildrundir", "builddir":
		r.status, r.detail = t.builddir()
	}
	r.dur = t.dur
	return r
}

// cmdResult is the result of one command of a test.
type cmdResult struct {
	out      string
	code     int
	timedOut bool
	err      error
}

// exec runs a command of the test in dir with the environment of testdir,
// the environment of the recipe and more. The time left before the deadline
// of the test is its timeout.
func (t *test) exec(dir string, cmd []string, more ...string) cmdResult {
	left := time.Until(t.deadline)
	if left <= 0 {
		return cmdResult{code: -1, timedOut: true}
	}
	env := append(append(append([]string{}, testEnv...), t.env...), more...)
	o, code, timedOut, err := execute(dir, cmd, env, left)
	t.dur += o.dur
	return cmdResult{strings.ReplaceAll(o.out, "\r\n", "\n"), code, timedOut, err}
}

// engineCmd is the command that runs target (a file, or a directory or an
// import path for a package) with the engine.
func (t *test) engineCmd(target string, args []string) []string {
	if t.engine == "yaegi" {
		return append([]string{t.bin, "run", target}, args...)
	}
	return append([]string{t.bin, target}, args...)
}

// checkOutput compares the output of a run with the .out file of the test.
func (t *test) checkOutput(c cmdResult) (status, detail string) {
	if c.timedOut {
		return "TIMEOUT", cleanStart(c.out)
	}
	if c.err != nil {
		return "FAIL_EXIT", c.err.Error() + ": " + cleanStart(c.out)
	}
	want, _ := os.ReadFile(strings.TrimSuffix(t.long, ".go") + ".out")
	if c.out != string(want) {
		return "FAIL_OUTPUT", firstDiff(c.out, string(want))
	}
	return "PASS", ""
}

// failed gives the status of a step that must succeed (a compile, a link,
// a generator); ok is false when the step failed.
func failed(step string, c cmdResult) (status, detail string, ok bool) {
	switch {
	case c.timedOut:
		return "TIMEOUT", step + ": " + cleanStart(c.out), false
	case c.err != nil:
		return "FAIL_EXIT", step + ": " + c.err.Error() + ": " + cleanStart(c.out), false
	}
	return "", "", true
}

// run is the run action. The go engine runs "go run" (with the flags of
// the recipe); the interpreters run the file, or a directory with all the
// files of the program when the recipe names more files.
func (t *test) run() (status, detail string) {
	// Leading .go arguments are more source files of the program, as with
	// "go run a.go b.go" (cmplxdivide.go has "// run cmplxdivide1.go").
	args := t.args
	var extra []string
	for len(args) > 0 && strings.HasSuffix(args[0], ".go") {
		extra = append(extra, filepath.Join(filepath.Dir(t.file), args[0]))
		args = args[1:]
	}
	if t.engine == "go" {
		cmd := append(append([]string{t.bin, "run"}, t.flags...), t.file)
		cmd = append(append(cmd, extra...), args...)
		return t.checkOutput(t.exec(t.root, cmd))
	}
	dir, target := t.root, t.file
	var more []string
	if len(extra) > 0 {
		// yaegi v2 and yaegi take one file or one directory: put the files of
		// the program in a directory of their own, with a go.mod for yaegi v2
		// (go/packages) and in a temporary GOPATH for yaegi (it finds a
		// directory only by its import path in GOPATH).
		dir = filepath.Join(t.tmp, "src", "prog")
		var names []string
		for _, name := range append([]string{t.file}, extra...) {
			names = append(names, filepath.Join(t.root, name))
		}
		if err := copyFiles(dir, names, "module prog\n\ngo 1.27\n"); err != nil {
			return "FAIL_EXIT", err.Error()
		}
		target = "."
		if t.engine == "yaegi" {
			target = "prog"
		}
		more = append(more, "GOPATH="+t.tmp)
	}
	return t.checkOutput(t.exec(dir, t.engineCmd(target, args), more...))
}

// errorcheck is the errorcheck action: see checkErrors.
func (t *test) errorcheck() (status, detail string) {
	var c cmdResult
	if t.engine == "go" {
		// The compiler alone, with the flags of testdir: "go build" would
		// first report the errors of go/parser (go list).
		c = t.exec(t.root, []string{t.bin, "tool", "compile", "-p=p", "-d=panic", "-C", "-e", "-importcfg=" + t.importcfg, "-o", filepath.Join(t.tmp, "a.o"), t.file})
	} else {
		c = t.exec(t.root, t.engineCmd(t.file, t.args))
	}
	if c.timedOut {
		return "TIMEOUT", cleanStart(c.out)
	}
	return checkErrors(t.engine, t.root, t.file, t.src, c.out, c.code, c.err)
}

// loadCmd is the command of the compile-like tests for an interpreter.
func (t *test) loadCmd(patterns ...string) []string {
	return append([]string{t.load}, patterns...)
}

// compile is the compile and build actions: the go engine compiles the file
// (compile: the compiler alone, build: go build); yaegi v2 loads it.
func (t *test) compile() (status, detail string) {
	var c cmdResult
	switch {
	case t.engine != "go":
		c = t.exec(t.root, t.loadCmd(t.long))
	case t.action == "compile":
		cmd := append([]string{t.bin, "tool", "compile", "-e", "-p=p", "-importcfg=" + t.importcfg}, t.flags...)
		c = t.exec(t.tmp, append(cmd, t.long))
	default:
		cmd := append([]string{t.bin, "build", "-gcflags=all="}, t.flags...)
		c = t.exec(t.tmp, append(cmd, "-o", "a.exe", t.long))
	}
	if s, d, ok := failed("compile", c); !ok {
		return s, d
	}
	return "PASS", ""
}

// pkg is a package of a test directory: its name and its files.
type pkg struct {
	name  string
	files []string
}

var packageRx = regexp.MustCompile(`(?m)^package ([\p{Lu}\p{Ll}\w]+)`)

// dirPackages returns the packages of a test directory as testdir does:
// the .go files in the order of their names, a new package for each new
// package name (or for each file with -s).
func dirPackages(dir string, single bool) ([]*pkg, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var pkgs []*pkg
	m := map[string]*pkg{}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".go" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		n := packageRx.FindStringSubmatch(string(b))
		if n == nil {
			return nil, fmt.Errorf("cannot find package name in %s", e.Name())
		}
		p, ok := m[n[1]]
		if single || !ok {
			p = &pkg{name: n[1]}
			pkgs = append(pkgs, p)
			m[n[1]] = p
		}
		p.files = append(p.files, e.Name())
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no package in %s", dir)
	}
	return pkgs, nil
}

// pkgPath is the import path of a package of a test directory, as testdir
// gives it to gc: test/<first file without .go>.
func (p *pkg) pkgPath() string { return "test/" + strings.TrimSuffix(p.files[0], ".go") }

// goDir is the directory of the test: x.go has x.dir.
func (t *test) goDir() string { return strings.TrimSuffix(t.long, ".go") + ".dir" }

// goCompileInDir is compileInDir of testdir: one package with gc, in the
// temporary directory.
func (t *test) goCompileInDir(dir, importcfg string, p *pkg, flags []string) cmdResult {
	cmd := []string{t.bin, "tool", "compile", "-e", "-D", "test", "-importcfg=" + importcfg}
	if p.name == "main" {
		cmd = append(cmd, "-p=main")
	} else {
		cmd = append(cmd, "-o", p.pkgPath()+".a", "-p", p.pkgPath())
	}
	cmd = append(cmd, flags...)
	for _, f := range p.files {
		cmd = append(cmd, filepath.Join(dir, f))
	}
	return t.exec(t.tmp, cmd)
}

// goImportcfg is importcfg of testdir: the standard library and the
// packages of the test directory.
func (t *test) goImportcfg(pkgs []*pkg) (string, error) {
	cfg := t.stdcfg
	for _, p := range pkgs {
		cfg += "\npackagefile " + p.pkgPath() + "=" + filepath.Join(t.tmp, p.pkgPath()+".a")
	}
	file := filepath.Join(t.tmp, "importcfg")
	if err := os.MkdirAll(filepath.Join(t.tmp, "test"), 0o755); err != nil {
		return "", err
	}
	return file, os.WriteFile(file, []byte(cfg), 0o644)
}

// goLink is linkFile of testdir.
func (t *test) goLink(out, in, importcfg string, ldflags []string) cmdResult {
	in = strings.TrimSuffix(in, ".go")
	if !strings.HasSuffix(in, ".a") && !strings.HasSuffix(in, ".o") {
		in += ".o"
	}
	cmd := append([]string{t.bin, "tool", "link", "-s", "-w", "-buildid=test", "-o", out, "-importcfg=" + importcfg}, ldflags...)
	return t.exec(t.tmp, append(cmd, in))
}

// layout copies the packages of a test directory into a module "test" for
// an interpreter: one directory per package, named like its import path
// for gc (test/a for a.go), and the import paths "./a" become "test/a"
// (gc gets the same path with -D test). The line numbers do not change.
// It returns the module root and the directory of each package.
func (t *test) layout(dir string, pkgs []*pkg) (root string, pdirs []string, err error) {
	root = filepath.Join(t.tmp, "src", "test")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0o644); err != nil {
		return "", nil, err
	}
	for _, p := range pkgs {
		pd := filepath.Join(t.tmp, "src", p.pkgPath())
		if err := os.MkdirAll(pd, 0o755); err != nil {
			return "", nil, err
		}
		for _, f := range p.files {
			b, err := os.ReadFile(filepath.Join(dir, f))
			if err != nil {
				return "", nil, err
			}
			if err := os.WriteFile(filepath.Join(pd, layoutName(f)), rewriteImports(b), 0o644); err != nil {
				return "", nil, err
			}
		}
		pdirs = append(pdirs, pd)
	}
	return root, pdirs, nil
}

// layoutName is the name of a file of a test directory in the layout of an
// interpreter: x_test.go becomes x_test_.go, because the go command (and
// go/packages) leaves test files out of a package, and gc does not.
func layoutName(f string) string {
	if b, ok := strings.CutSuffix(f, "_test.go"); ok {
		return b + "_test_.go"
	}
	return f
}

// rewriteImports writes the import paths "./x" of src as "test/x". Only the
// strings of import declarations change (not comments, not other strings),
// and they keep their line.
func rewriteImports(src []byte) []byte {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, 0)
	type edit struct{ off, end int }
	var edits []edit
	inImport, paren := false, false
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		switch {
		case tok == token.IMPORT:
			inImport, paren = true, false
		case !inImport:
		case tok == token.LPAREN:
			paren = true
		case tok == token.RPAREN:
			inImport = false
		case tok == token.STRING:
			if p, err := strconv.Unquote(lit); err == nil && strings.HasPrefix(p, "./") {
				off := file.Offset(pos)
				edits = append(edits, edit{off, off + len(lit)})
			}
			if !paren {
				inImport = false
			}
		case tok == token.SEMICOLON || tok == token.IDENT || tok == token.PERIOD:
		default:
			inImport = false
		}
	}
	if len(edits) == 0 {
		return src
	}
	var out []byte
	last := 0
	for _, e := range edits {
		p, _ := strconv.Unquote(string(src[e.off:e.end]))
		out = append(out, src[last:e.off]...)
		out = append(out, strconv.Quote("test/"+strings.TrimPrefix(p, "./"))...)
		last = e.end
	}
	return append(out, src[last:]...)
}

// dirSources returns the files of the packages for checkErrors.
func dirSources(pdirs []string, pkgs []*pkg) ([]srcFile, error) {
	var fs []srcFile
	for i, p := range pkgs {
		for _, f := range p.files {
			b, err := os.ReadFile(filepath.Join(pdirs[i], layoutName(f)))
			if err != nil {
				return nil, err
			}
			fs = append(fs, srcFile{abs: filepath.Join(pdirs[i], layoutName(f)), base: layoutName(f), src: string(b)})
		}
	}
	return fs, nil
}

// compiledir compiles all the packages of the directory in order (go), or
// loads them all (yaegi v2).
func (t *test) compiledir() (status, detail string) {
	dir := t.goDir()
	pkgs, err := dirPackages(dir, t.single)
	if err != nil {
		return "FAIL_EXIT", err.Error()
	}
	if t.engine != "go" {
		root, _, err := t.layout(dir, pkgs)
		if err != nil {
			return "FAIL_EXIT", err.Error()
		}
		if s, d, ok := failed("load", t.exec(root, t.loadCmd("./..."))); !ok {
			return s, d
		}
		return "PASS", ""
	}
	cfg, err := t.goImportcfg(pkgs)
	if err != nil {
		return "FAIL_EXIT", err.Error()
	}
	for _, p := range pkgs {
		if s, d, ok := failed("compile "+p.pkgPath(), t.goCompileInDir(dir, cfg, p, t.flags)); !ok {
			return s, d
		}
	}
	return "PASS", ""
}

// dir is rundir, errorcheckdir and errorcheckandrundir. The packages are
// compiled in order. errorcheckdir: the last package must be refused, with
// the errors of its ERROR comments (checkErrors). errorcheckandrundir: the
// package before the last must be refused, then the last package (main)
// runs. rundir: the last package (main) runs.
func (t *test) dir() (status, detail string) {
	dir := t.goDir()
	pkgs, err := dirPackages(dir, t.single)
	if err != nil {
		return "FAIL_EXIT", err.Error()
	}
	errPkg := -1
	if t.action == "errorcheckdir" {
		errPkg = len(pkgs) - 1
	} else if t.action == "errorcheckandrundir" {
		errPkg = len(pkgs) - 2
	}
	if errPkg < 0 && t.action != "rundir" {
		return "FAIL_EXIT", "not enough packages"
	}
	last := pkgs[len(pkgs)-1]

	if t.engine != "go" {
		_, pdirs, err := t.layout(dir, pkgs)
		if err != nil {
			return "FAIL_EXIT", err.Error()
		}
		target := func(i int) string {
			if t.engine == "yaegi" {
				return pkgs[i].pkgPath()
			}
			return pdirs[i]
		}
		gopath := "GOPATH=" + t.tmp
		if errPkg >= 0 {
			fs, err := dirSources(pdirs, pkgs)
			if err != nil {
				return "FAIL_EXIT", err.Error()
			}
			c := t.exec(pdirs[errPkg], t.engineCmd(target(errPkg), nil), gopath)
			if c.timedOut {
				return "TIMEOUT", cleanStart(c.out)
			}
			status, detail = checkErrorsFiles(t.engine, c.out, c.code, c.err, fs)
			if status != "PASS" || t.action == "errorcheckdir" {
				return status, detail
			}
		}
		return t.checkOutput(t.exec(pdirs[len(pkgs)-1], t.engineCmd(target(len(pkgs)-1), t.args), gopath))
	}

	flags, ldflags := t.flags, []string(nil)
	if errPkg >= 0 {
		flags = append(append([]string{}, flags...), "-d=panic")
	}
	for i, fl := range flags {
		if fl == "-ldflags" {
			ldflags = flags[i+1:]
			flags = flags[:i]
			break
		}
	}
	cfg, err := t.goImportcfg(pkgs)
	if err != nil {
		return "FAIL_EXIT", err.Error()
	}
	var errOut strings.Builder
	var errRes cmdResult
	for i, p := range pkgs {
		c := t.goCompileInDir(dir, cfg, p, flags)
		if c.timedOut {
			return "TIMEOUT", cleanStart(c.out)
		}
		errOut.WriteString(c.out)
		if i == errPkg {
			errRes = c
			continue
		}
		if c.err != nil {
			s, d, _ := failed("compile "+p.pkgPath(), c)
			if errPkg >= 0 {
				return "ERR_CRASH", d
			}
			return s, d
		}
	}
	if errPkg >= 0 {
		var fs []srcFile
		for _, p := range pkgs {
			for _, f := range p.files {
				b, err := os.ReadFile(filepath.Join(dir, f))
				if err != nil {
					return "FAIL_EXIT", err.Error()
				}
				fs = append(fs, srcFile{abs: filepath.Join(dir, f), base: f, src: string(b)})
			}
		}
		status, detail = checkErrorsFiles(t.engine, errOut.String(), errRes.code, errRes.err, fs)
		if status != "PASS" || t.action == "errorcheckdir" {
			return status, detail
		}
	}
	if s, d, ok := failed("link", t.goLink("a.exe", last.files[0], cfg, ldflags)); !ok {
		return s, d
	}
	return t.checkOutput(t.exec(t.tmp, append([]string{filepath.Join(t.tmp, "a.exe")}, t.args...)))
}

// runindir copies the test directory into a module of its own (named like
// the directory, language version 1.14 or -gomodversion) in a temporary
// GOPATH, and runs "go run ." there (go) or the directory (interpreters).
func (t *test) runindir() (status, detail string) {
	src := t.goDir()
	mod := filepath.Base(src)
	dir := filepath.Join(t.tmp, "src", mod)
	if err := copyTree(dir, src); err != nil {
		return "FAIL_EXIT", err.Error()
	}
	v := t.gomodvers
	if v == "" {
		v = "1.14"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(fmt.Sprintf("module %s\ngo %s\n", mod, v)), 0o666); err != nil {
		return "FAIL_EXIT", err.Error()
	}
	gopath := "GOPATH=" + t.tmp
	if t.engine == "go" {
		cmd := append(append([]string{t.bin, "run", "-gcflags=all="}, t.flags...), ".")
		return t.checkOutput(t.exec(dir, cmd, gopath))
	}
	target := "."
	if t.engine == "yaegi" {
		target = mod
	}
	return t.checkOutput(t.exec(dir, t.engineCmd(target, nil), gopath))
}

// output is runoutput and errorcheckoutput: "go run" of the test (always
// with the go command) writes a program, tmp__.go; the engine runs it
// (runoutput) or must refuse it (errorcheckoutput).
func (t *test) output() (status, detail string) {
	gen := append([]string{t.gobin, "run", "-gcflags=all=", t.file}, t.args...)
	c := t.exec(t.root, gen)
	if s, d, ok := failed("generator", c); !ok {
		return s, d
	}
	tfile := filepath.Join(t.tmp, "tmp__.go")
	if err := os.WriteFile(tfile, []byte(c.out), 0o666); err != nil {
		return "FAIL_EXIT", err.Error()
	}
	if t.action == "runoutput" {
		cmd := t.engineCmd(tfile, nil)
		if t.engine == "go" {
			cmd = []string{t.bin, "run", "-gcflags=all=", tfile}
		}
		return t.checkOutput(t.exec(t.root, cmd))
	}
	var r cmdResult
	if t.engine == "go" {
		cmd := append([]string{t.bin, "tool", "compile", "-importcfg=" + t.importcfg, "-p=p", "-d=panic", "-e", "-o", "a.o"}, t.flags...)
		r = t.exec(t.tmp, append(cmd, tfile))
	} else {
		r = t.exec(t.root, t.engineCmd(tfile, nil))
	}
	if r.timedOut {
		return "TIMEOUT", cleanStart(r.out)
	}
	return checkErrorsFiles(t.engine, r.out, r.code, r.err, []srcFile{{abs: tfile, base: "tmp__.go", src: c.out}})
}

// buildrun builds the file with go build and runs the binary in the
// temporary directory (go); the interpreters run the file there.
func (t *test) buildrun() (status, detail string) {
	if t.engine != "go" {
		return t.checkOutput(t.exec(t.tmp, t.engineCmd(t.long, t.args)))
	}
	cmd := append(append([]string{t.bin, "build", "-gcflags=all=", "-o", "a.exe"}, t.flags...), t.long)
	if s, d, ok := failed("build", t.exec(t.tmp, cmd)); !ok {
		return s, d
	}
	return t.checkOutput(t.exec(t.tmp, append([]string{filepath.Join(t.tmp, "a.exe")}, t.args...)))
}

// builddir is builddir and buildrundir: all the .go and .s files of the
// directory are one main package. go builds it with the compiler, the
// assembler and the linker as testdir does; the interpreters get a copy of
// the directory (.s files included) as a module "prog".
func (t *test) builddir() (status, detail string) {
	dir := t.goDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "FAIL_EXIT", err.Error()
	}
	var gos, asms []string
	for _, e := range entries {
		switch filepath.Ext(e.Name()) {
		case ".go":
			gos = append(gos, filepath.Join(dir, e.Name()))
		case ".s":
			asms = append(asms, filepath.Join(dir, e.Name()))
		}
	}
	if t.engine != "go" {
		pd := filepath.Join(t.tmp, "src", "prog")
		if err := copyFiles(pd, append(append([]string{}, gos...), asms...), "module prog\n\ngo 1.27\n"); err != nil {
			return "FAIL_EXIT", err.Error()
		}
		if t.action == "builddir" {
			if s, d, ok := failed("load", t.exec(pd, t.loadCmd("."))); !ok {
				return s, d
			}
			return "PASS", ""
		}
		target := "."
		if t.engine == "yaegi" {
			target = "prog"
		}
		return t.checkOutput(t.exec(pd, t.engineCmd(target, nil), "GOPATH="+t.tmp))
	}
	if len(asms) > 0 {
		if err := os.WriteFile(filepath.Join(t.tmp, "go_asm.h"), nil, 0o666); err != nil {
			return "FAIL_EXIT", err.Error()
		}
		cmd := append([]string{t.bin, "tool", "asm", "-p=main", "-gensymabis", "-o", "symabis"}, asms...)
		if s, d, ok := failed("asm", t.exec(t.tmp, cmd)); !ok {
			return s, d
		}
	}
	cmd := []string{t.bin, "tool", "compile", "-p=main", "-e", "-D", ".", "-importcfg=" + t.importcfg, "-o", "go.o"}
	if len(asms) > 0 {
		cmd = append(cmd, "-asmhdr", "go_asm.h", "-symabis", "symabis")
	}
	if s, d, ok := failed("compile", t.exec(t.tmp, append(cmd, gos...))); !ok {
		return s, d
	}
	objs := []string{"go.o"}
	if len(asms) > 0 {
		cmd = append([]string{t.bin, "tool", "asm", "-p=main", "-e", "-I", ".", "-o", "asm.o"}, asms...)
		if s, d, ok := failed("asm", t.exec(t.tmp, cmd)); !ok {
			return s, d
		}
		objs = append(objs, "asm.o")
	}
	if s, d, ok := failed("pack", t.exec(t.tmp, append([]string{t.bin, "tool", "pack", "c", "all.a"}, objs...))); !ok {
		return s, d
	}
	if s, d, ok := failed("link", t.goLink("a.exe", "all.a", t.importcfg, nil)); !ok {
		return s, d
	}
	if t.action == "builddir" {
		return "PASS", ""
	}
	return t.checkOutput(t.exec(t.tmp, []string{filepath.Join(t.tmp, "a.exe")}))
}

// copyFiles copies files into dir, with a go.mod.
func copyFiles(dir string, files []string, gomod string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644)
}

// copyTree copies the directory src to dst, as overlayDir of testdir.
func copyTree(dst, src string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(to, b, 0o644)
	})
}

type output struct {
	out string
	dur time.Duration
}

// execute runs cmd in its own process group. When the timeout expires, it
// kills the whole group (the engine and its children). timedOut is true only
// when the process was killed by this timeout. After the end of the leader,
// what is left in the group is killed too.
func execute(dir string, cmd, env []string, timeout time.Duration) (o output, code int, timedOut bool, err error) {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Dir = dir
	c.Env = append(append(os.Environ(), "PWD="+dir), env...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// A child that keeps the output pipe open must not block Wait forever.
	c.WaitDelay = 5 * time.Second
	var buf bytes.Buffer
	c.Stdout, c.Stderr = &buf, &buf
	start := time.Now()
	if err = c.Start(); err != nil {
		return output{out: err.Error()}, -1, false, err
	}
	pgid := c.Process.Pid
	var killed atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		killed.Store(true)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	})
	err = c.Wait()
	timer.Stop()
	o.dur = time.Since(start)
	_ = syscall.Kill(-pgid, syscall.SIGKILL) // children left behind
	o.out = buf.String()
	code = -1
	if ps := c.ProcessState; ps != nil {
		code = ps.ExitCode()
		if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL && killed.Load() {
			timedOut = true
		}
	}
	return o, code, timedOut, err
}

// Patterns of ERROR comments, as in cmd/internal/testdir.
var (
	errRx       = regexp.MustCompile(`// (?:GC_)?ERROR (.*)`)
	errQuotesRx = regexp.MustCompile(`"([^"]*)"`)
	lineRx      = regexp.MustCompile(`LINE(([+-])(\d+))?`)
	moreErrsRx  = regexp.MustCompile(`\s*\(and \d+ more errors\)$`)
	crashRx     = regexp.MustCompile(`(?m)^(panic: |fatal error: |\[signal |goroutine \d+ \[)`)
	// notMainRx is the refusal of yaegi v2 to run a package that is not main
	// (v1: "load: p is not a main package", v2: "p.go:1:1: package p;
	// expected package main"): the package loaded without error.
	notMainRx = regexp.MustCompile(`(?m)^.*(is not a main package|; expected package main)$`)
)

// wantedErrors returns, for each line of src that has an ERROR comment, the
// regexps of this comment. LINE, LINE+n and LINE-n become "base:N".
func wantedErrors(base, src string) (map[int][]*regexp.Regexp, error) {
	want := map[int][]*regexp.Regexp{}
	for i, line := range strings.Split(src, "\n") {
		n := i + 1
		if strings.Contains(line, "////") {
			continue // a double comment disables ERROR
		}
		m := errRx.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		for _, q := range errQuotesRx.FindAllStringSubmatch(m[1], -1) {
			rx := lineRx.ReplaceAllStringFunc(q[1], func(s string) string {
				l := n
				if d, ok := strings.CutPrefix(s, "LINE+"); ok {
					k, _ := strconv.Atoi(d)
					l += k
				} else if d, ok := strings.CutPrefix(s, "LINE-"); ok {
					k, _ := strconv.Atoi(d)
					l -= k
				}
				return fmt.Sprintf("%s:%d", base, l)
			})
			re, err := regexp.Compile(rx)
			if err != nil {
				return nil, fmt.Errorf("line %d: bad ERROR regexp %q: %v", n, rx, err)
			}
			want[n] = append(want[n], re)
		}
		if want[n] == nil {
			want[n] = []*regexp.Regexp{} // ERROR with no quoted regexp
		}
	}
	return want, nil
}

// srcFile is a source file of an errorcheck test: its absolute path, its
// path from the test root (one-file tests; empty for a test directory), its
// base name in the messages, and its source.
type srcFile struct{ abs, rel, base, src string }

// checkErrors gives the status of an errorcheck test of one file (see
// checkErrorsFiles).
func checkErrors(engine, root, file, src, out string, code int, err error) (status, detail string) {
	return checkErrorsFiles(engine, out, code, err, []srcFile{{abs: filepath.Join(root, file), rel: file, base: filepath.Base(file), src: src}})
}

// checkErrorsFiles gives the status of an errorcheck test. The rule is the
// rule of testdir, with two changes for engines that stop early:
//
//   - the engine must refuse the program at compile time: exit code of a
//     compile error for this engine (rejectCode), and no panic or crash in
//     the output. A crash of the interpreter is ERR_CRASH, not a rejection;
//   - every error the engine reports on the files ("file.go:LINE[:COL]: msg")
//     must be on a line with an ERROR comment, and its message must match
//     one of the regexps of this comment (as testdir, the text after
//     "file:line: " is matched). At least one such error is needed;
//   - unlike testdir, the engine does not have to report every ERROR line:
//     yaegi stops at the first error, and go/types may stop earlier than gc.
//
// The message wording counts: yaegi v2 prints go/types messages, which are
// almost always the messages of gc (types2); yaegi has its own messages.
func checkErrorsFiles(engine, out string, code int, err error, files []srcFile) (status, detail string) {
	if code == 0 && err == nil {
		return "ERR_MISSED", cleanStart(out)
	}
	if code != rejectCode[engine] || crashRx.MatchString(out) {
		return "ERR_CRASH", fmt.Sprintf("exit %d: %s", code, cleanStart(out))
	}
	if engine == "govm" && strings.TrimSpace(notMainRx.ReplaceAllString(out, "")) == "" && notMainRx.MatchString(out) {
		// The package loaded without error; yaegi v2 runs only main packages.
		return "ERR_MISSED", cleanStart(out)
	}
	// As testdir, write each file as its base name in the messages
	// ("declared at issue68292.go:9:8").
	var repl []string
	want := map[string]map[int][]*regexp.Regexp{}
	var bases []string
	for _, f := range files {
		repl = append(repl, f.abs, f.base)
		if f.rel != "" {
			repl = append(repl, "./"+f.rel, f.base, f.rel, f.base)
		}
		w, werr := wantedErrors(f.base, f.src)
		if werr != nil {
			return "ERR_WRONGMSG", f.base + ": " + werr.Error()
		}
		want[f.base] = w
		bases = append(bases, regexp.QuoteMeta(f.base))
	}
	out = strings.NewReplacer(repl...).Replace(out)
	// The file name must start the line or follow a space or a slash, so
	// that "xbug1.go:12" is not taken for "bug1.go:12".
	posRx := regexp.MustCompile(`(?:^|[\s/])(` + strings.Join(bases, "|") + `):(\d+)(?::\d+)?: ?(.*)$`)
	var msgs []string // gc messages go on in lines that start with a tab
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "\t") && len(msgs) > 0 {
			msgs[len(msgs)-1] += "\n" + line
		} else if strings.TrimSpace(line) != "" {
			msgs = append(msgs, line)
		}
	}
	// First the lines of all errors, then their messages: ERR_WRONGMSG
	// means that every error is on an ERROR line.
	type found struct {
		base      string
		n         int
		msg, text string
	}
	var errs []found
	for _, msg := range msgs {
		first, _, _ := strings.Cut(msg, "\n")
		m := posRx.FindStringSubmatchIndex(first)
		if m == nil {
			continue // not an error of these files ("# command-line-arguments"...)
		}
		base := first[m[2]:m[3]]
		n, _ := strconv.Atoi(first[m[4]:m[5]])
		text := moreErrsRx.ReplaceAllString(msg[m[6]:], "") // yaegi: "(and 3 more errors)"
		if strings.HasPrefix(text, "\t") || strings.HasPrefix(text, " ") {
			// go/types gives the second part of an error ("other
			// declaration of x") as an error that starts with a tab;
			// gc prints it on a line that starts with a tab.
			continue
		}
		if text == "too many errors" {
			// Not an error of its own: the engine stops after 10 errors
			// (gc without -e, go/packages).
			continue
		}
		if _, ok := want[base][n]; !ok {
			where := fmt.Sprintf("line %d", n)
			if len(files) > 1 {
				where = fmt.Sprintf("%s:%d", base, n)
			}
			return "ERR_WRONGLINE", fmt.Sprintf("%s has no ERROR comment: %s", where, cleanStart(msg))
		}
		errs = append(errs, found{base, n, msg, text})
	}
	if len(errs) == 0 {
		return "ERR_WRONGLINE", "no error on a line of the file: " + cleanStart(out)
	}
	for _, e := range errs {
		matched := false
		for _, re := range want[e.base][e.n] {
			if re.MatchString(e.text) {
				matched = true
				break
			}
		}
		if !matched {
			var rs []string
			for _, re := range want[e.base][e.n] {
				rs = append(rs, strconv.Quote(re.String()))
			}
			where := fmt.Sprintf("line %d", e.n)
			if len(files) > 1 {
				where = fmt.Sprintf("%s:%d", e.base, e.n)
			}
			return "ERR_WRONGMSG", fmt.Sprintf("%s: %s does not match %s", where, strconv.Quote(cut(clean(e.text), 150)), strings.Join(rs, " "))
		}
	}
	return "PASS", ""
}

// firstDiff shows where the output starts to differ from the .out file: the
// line number, then the rest of the output and of the .out file from the
// start of this line.
func firstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	ls := strings.LastIndexByte(got[:i], '\n') + 1
	line := strings.Count(got[:ls], "\n") + 1
	return fmt.Sprintf("line %d: got %s want %s", line, strconv.Quote(cut(clean(got[ls:]), 120)), strconv.Quote(cut(clean(want[ls:]), 120)))
}

// cleanStart returns the start of an output, without local paths.
func cleanStart(s string) string {
	return cut(clean(strings.TrimSpace(s)), 400)
}

// cut keeps at most n bytes of s, without cutting a UTF-8 rune.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

var (
	cleaner  *strings.Replacer
	homeRx   = regexp.MustCompile(`/home/[^/\s"']+`)
	tmpDirRx = regexp.MustCompile(`/tmp/(?:[^\s:"'/]*/)*`)
)

// initCleaner prepares clean: each local directory becomes a variable name.
func initCleaner(root string) {
	type repl struct{ dir, name string }
	rs := []repl{{root, "$GOROOT/test"}}
	if out, err := exec.Command("go", "env", "GOROOT", "GOMODCACHE", "GOPATH", "GOMOD").Output(); err == nil {
		v := strings.Split(string(out), "\n")
		for i, name := range []string{"$GOROOT", "$GOMODCACHE", "$GOPATH"} {
			if i < len(v) && v[i] != "" {
				rs = append(rs, repl{v[i], name})
			}
		}
		if len(v) > 3 && v[3] != "" && v[3] != os.DevNull {
			rs = append(rs, repl{filepath.Dir(v[3]), "$YAEGI"})
		}
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" && h != "/" {
		rs = append(rs, repl{h, "$HOME"})
	}
	if t := os.TempDir(); t != "/" && t != "/tmp" { // /tmp: see tmpDirRx
		rs = append(rs, repl{t, "$TMPDIR"})
	}
	// Longest first: the test directory can be inside the repository.
	sort.Slice(rs, func(i, j int) bool { return len(rs[i].dir) > len(rs[j].dir) })
	var args []string
	for _, r := range rs {
		args = append(args, r.dir, r.name)
	}
	cleaner = strings.NewReplacer(args...)
}

// clean removes local paths, invalid UTF-8 and control characters (except
// newline and tab) from s.
func clean(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return '?'
		}
		return r
	}, s)
	if cleaner != nil {
		s = cleaner.Replace(s)
	}
	s = homeRx.ReplaceAllString(s, "$$HOME")
	return tmpDirRx.ReplaceAllString(s, "$$TMP/")
}

// splitQuoted splits a recipe like testdir (single and double quotes).
func splitQuoted(s string) []string {
	var out []string
	var cur strings.Builder
	var q rune
	in := false
	for _, ch := range s {
		switch {
		case q != 0 && ch == q:
			q = 0
		case q != 0:
			cur.WriteRune(ch)
		case ch == '"' || ch == '\'':
			q, in = ch, true
		case ch == ' ' || ch == '\t':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(ch)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}
