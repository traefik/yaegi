package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Statuses of a conformance program.
const (
	StatusPass        = "PASS"         // Output: program, exit 0, same output
	StatusPassErr     = "PASS_ERR"     // Error: program rejected, message contains yaegi's expectation
	StatusPassErrGC   = "PASS_ERR_GC"  // Error: program rejected, message contains gc's (TestInterpErrorConsistency)
	StatusWrong       = "WRONG"        // exit 0, different output
	StatusCrash       = "CRASH"        // unrecovered panic or non-zero exit
	StatusLoadFail    = "LOADFAIL"     // Output: program rejected by go/packages / go/types
	StatusUnsupported = "UNSUPPORTED"  // VM compile error: construct not supported
	StatusTimeout     = "TIMEOUT"      // killed after the timeout
	StatusErrOtherMsg = "ERR_OTHERMSG" // Error: program rejected with another message
	StatusErrMissed   = "ERR_MISSED"   // Error: program accepted and run successfully
	StatusSkip        = "SKIP"         // no expectation, or not runnable by design
)

// Group maps a status to pass / fail / unsupported / skipped.
func Group(status string) string {
	switch status {
	case StatusPass, StatusPassErr, StatusPassErrGC:
		return "pass"
	case StatusUnsupported:
		return "unsupported"
	case StatusSkip:
		return "skipped"
	}
	return "fail"
}

// Config configures a conformance run.
type Config struct {
	TestDir   string        // tests/yaegi/_test
	VM        string        // yaegi binary
	VMArgs    []string      // extra yaegi flags
	Jobs      int           // parallel programs
	Timeout   time.Duration // per program
	GoVersion string        // go directive of the workspace module
}

// Result is the outcome of one program.
type Result struct {
	File   string // base name
	Origin string // master | fork:<branch> | upstream-pr:<N>
	Kind   string // Output | Error | "" (no expectation)
	Status string
	Family string
	Detail string
	Note   string
	Dur    time.Duration
}

// Expectation is the trailing comment of a program, as read by yaegi's TestFile.
type Expectation struct {
	Kind   string // Output | Error | ""
	Want   string
	GoPath string // relative to the test dir (yaegi's "GOPATH:" prefix)
}

// ReadExpectation parses the last comment of a _test program
// (same logic as wantedFromComment in yaegi's interp_file_test.go).
func ReadExpectation(path string) Expectation {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if f == nil || len(f.Comments) == 0 {
		return Expectation{}
	}
	text := f.Comments[len(f.Comments)-1].Text()
	var e Expectation
	if strings.HasPrefix(text, "GOPATH:") {
		parts := strings.SplitN(text, "\n", 2)
		e.GoPath = strings.TrimPrefix(parts[0], "GOPATH:")
		if len(parts) < 2 {
			return Expectation{}
		}
		text = parts[1]
	}
	switch {
	case strings.HasPrefix(text, "Output:\n"):
		e.Kind, e.Want = "Output", strings.TrimSpace(strings.TrimPrefix(text, "Output:\n"))
	case strings.HasPrefix(text, "Error:\n"):
		e.Kind, e.Want = "Error", strings.TrimSpace(strings.TrimPrefix(text, "Error:\n"))
	default:
		return Expectation{}
	}
	return e
}

// Discover returns the top-level .go programs of dir, sorted.
func Discover(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// PrepareWorkspace copies testDir into a temporary module named
// github.com/traefik/yaegi, as _test/. Programs importing local packages
// ("github.com/traefik/yaegi/_test/foo", resolved by yaegi through GOPATH)
// then resolve them inside that main module. It returns the copied _test dir.
func PrepareWorkspace(testDir, goVersion string) (wsTest string, cleanup func(), err error) {
	ws, err := os.MkdirTemp("", "yaegi-conformance-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(ws) }
	gomod := fmt.Sprintf("module github.com/traefik/yaegi\n\ngo %s\n", goVersion)
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte(gomod), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	wsTest = filepath.Join(ws, "_test")
	if err := copyTree(testDir, wsTest); err != nil {
		cleanup()
		return "", nil, err
	}
	// Resolve symlinks (macOS: /var -> /private/var) so that the paths printed
	// by go/packages can be rewritten in messages.
	if r, err := filepath.EvalSymlinks(wsTest); err == nil {
		wsTest = r
	}
	return wsTest, cleanup, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// BuildVM builds cmd/yaegi of the module rooted at root into out.
func BuildVM(root, out string) error {
	cmd := exec.Command("go", "build", "-o", out, "./cmd/yaegi")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build yaegi: %w\n%s", err, b)
	}
	return nil
}

// FindRoot walks up from dir to the go.mod of this repository.
func FindRoot(dir string) (string, error) {
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && bytes.HasPrefix(b, []byte("module github.com/traefik/yaegi/v2\n")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root (go.mod of github.com/traefik/yaegi/v2) not found")
		}
		dir = parent
	}
}

// ReadOrigins reads tests/yaegi/MANIFEST.tsv: base name of a top-level _test
// program -> origin without commit (master, fork:<branch>, upstream-pr:<N>).
func ReadOrigins(manifest string) map[string]string {
	origins := map[string]string{}
	f, err := os.Open(manifest)
	if err != nil {
		return origins
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) < 2 || !strings.HasPrefix(cols[0], "_test/") || strings.Count(cols[0], "/") != 1 {
			continue
		}
		origin, _, _ := strings.Cut(cols[1], "@")
		origins[strings.TrimPrefix(cols[0], "_test/")] = origin
	}
	return origins
}

var familyRe = regexp.MustCompile(`^[A-Za-z_]+`)

// Family is the alphabetic prefix of a program name (closure, issue, gen...).
func Family(name string) string {
	if f := familyRe.FindString(name); f != "" {
		return f
	}
	return "other"
}

// Run runs files (paths under wsTest) through the VM.
func Run(cfg Config, wsTest string, files []string, origins map[string]string) []Result {
	jobs := make(chan string)
	results := make([]Result, 0, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < max(1, cfg.Jobs); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				r := runOne(cfg, wsTest, p)
				r.Origin = origins[r.File]
				if r.Origin == "" {
					r.Origin = "local"
				}
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].File < results[j].File })
	return results
}

func runOne(cfg Config, wsTest, p string) Result {
	name := filepath.Base(p)
	r := Result{File: name, Family: Family(name), Note: consistencyReason[name]}
	exp := ReadExpectation(p)
	r.Kind = exp.Kind
	switch {
	case exp.Kind == "":
		r.Status, r.Detail = StatusSkip, "no Output:/Error: comment"
		return r
	case apiOnly[name]:
		r.Status, r.Detail = StatusSkip, "imports yaegi's interp package"
		return r
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	// The Output: comments of the yaegi programs are what yaegi writes to
	// its Stdout, and yaegi writes print and println there (gc writes them
	// to stderr): -print-stdout gives the same streams as yaegi.
	// yaegi's TestFile gives every program its restricted stdlib
	// (stdlib.Symbols: os.Exit and log.Fatal panic, os.FindProcess refuses
	// the current process, no os/exec and syscall): -restrictedstd gives
	// the same table (-restricted would also add the import checks of
	// plugins, which TestFile does not have). -yaegi-tags reads the
	// "// yaegi:tags" lines of the programs (tag0.go), as yaegi does.
	args := append([]string{"-print-stdout", "-restrictedstd", "-yaegi-tags"}, cfg.VMArgs...)
	cmd := exec.CommandContext(ctx, cfg.VM, append(args, name)...)
	cmd.Dir = filepath.Dir(p)
	cmd.WaitDelay = 2 * time.Second
	env := append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOWORK=off", "GOTOOLCHAIN=local")
	if exp.GoPath != "" {
		env = append(env, "GO111MODULE=off", "GOFLAGS=", "GOPATH="+filepath.Join(wsTest, exp.GoPath))
	} else {
		env = append(env, "GO111MODULE=on", "GOFLAGS=-mod=mod")
	}
	cmd.Env = env
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	t0 := time.Now()
	err := cmd.Run()
	r.Dur = time.Since(t0)
	timedOut := ctx.Err() != nil
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	stderr := strings.ReplaceAll(se.String(), wsTest+string(filepath.Separator), "../_test/")
	first := firstLine(stderr)

	if exp.Kind == "Error" {
		switch {
		case timedOut:
			r.Status = StatusTimeout
		case code == 0:
			r.Status = StatusErrMissed
		case strings.Contains(stderr, exp.Want):
			r.Status, r.Detail = StatusPassErr, first
		case gcError[name] != "" && strings.Contains(stderr, gcError[name]):
			r.Status, r.Detail = StatusPassErrGC, first
		case code == 3:
			r.Status, r.Detail = StatusUnsupported, strings.TrimPrefix(first, "yaegi: ")
		default:
			r.Status, r.Detail = StatusErrOtherMsg, first
		}
		return r
	}

	got := strings.TrimSpace(so.String())
	got = strings.ReplaceAll(got, "../_test/"+name+":", "")
	got = strings.ReplaceAll(got, name+":", "")
	switch {
	case timedOut:
		r.Status = StatusTimeout
	case code == 0 && got == exp.Want:
		r.Status = StatusPass
	case code == 0:
		r.Status, r.Detail = StatusWrong, fmt.Sprintf("%q", truncate(got, 100))
	case code == 3:
		r.Status, r.Detail = StatusUnsupported, strings.TrimPrefix(first, "yaegi: ")
	case code == 4:
		r.Status, r.Detail = StatusLoadFail, first
	default:
		r.Status, r.Detail = StatusCrash, fmt.Sprintf("exit %d: %s", code, first)
		if got == exp.Want {
			r.Detail += " (stdout matched)"
		}
	}
	return r
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return truncate(s, 200)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// WriteTSV writes one line per program.
func WriteTSV(w io.Writer, results []Result) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "file\torigin\tkind\tstatus\tgroup\tfamily\tms\tdetail\tnote")
	for _, r := range results {
		fmt.Fprintf(bw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", r.File, r.Origin, r.Kind, r.Status, Group(r.Status), r.Family, r.Dur.Milliseconds(),
			strings.ReplaceAll(r.Detail, "\t", " "), r.Note)
	}
	return bw.Flush()
}

var normRe = regexp.MustCompile(`(native (func|type|global) )\S+`)

// WriteSummary writes the counts by status, group, origin, kind and family.
func WriteSummary(w io.Writer, results []Result) {
	count := func(keyOf func(Result) string) (map[string]map[string]int, []string) {
		m := map[string]map[string]int{}
		for _, r := range results {
			k := keyOf(r)
			if m[k] == nil {
				m[k] = map[string]int{}
			}
			m[k][Group(r.Status)]++
			m[k]["total"]++
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return m, keys
	}
	table := func(title string, keyOf func(Result) string) {
		m, keys := count(keyOf)
		fmt.Fprintf(w, "\n%s\n", title)
		fmt.Fprintf(w, "  %-34s %6s %6s %6s %12s %8s\n", "", "total", "pass", "fail", "unsupported", "skipped")
		for _, k := range keys {
			c := m[k]
			fmt.Fprintf(w, "  %-34s %6d %6d %6d %12d %8d\n", k, c["total"], c["pass"], c["fail"], c["unsupported"], c["skipped"])
		}
	}

	statuses := map[string]int{}
	for _, r := range results {
		statuses[r.Status]++
	}
	keys := make([]string, 0, len(statuses))
	for k := range statuses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "programs: %d\n\nby status\n", len(results))
	for _, k := range keys {
		fmt.Fprintf(w, "  %-14s %5d\n", k, statuses[k])
	}
	table("by group", func(Result) string { return "all" })
	table("by origin", func(r Result) string { return r.Origin })
	table("by kind", func(r Result) string {
		if r.Kind == "" {
			return "(none)"
		}
		return r.Kind
	})

	// Spike-comparable figure: master Output: programs.
	var pass, total, lenient int
	for _, r := range results {
		if r.Origin == "master" && r.Kind == "Output" && !apiOnly[r.File] {
			total++
			if r.Status == StatusPass {
				pass++
			}
			if r.Status == StatusCrash && strings.HasSuffix(r.Detail, "(stdout matched)") {
				lenient++
			}
		}
	}
	fmt.Fprintf(w, "\nmaster Output: programs passing: %d / %d (+%d with matching stdout but non-zero exit)\n", pass, total, lenient)

	m, fams := count(func(r Result) string { return r.Family })
	sort.SliceStable(fams, func(i, j int) bool { return m[fams[i]]["total"] > m[fams[j]]["total"] })
	fmt.Fprintf(w, "\nby family (pass/total, largest first)\n ")
	for i, f := range fams {
		fmt.Fprintf(w, " %s %d/%d", f, m[f]["pass"], m[f]["total"])
		if i%8 == 7 {
			fmt.Fprintf(w, "\n ")
		}
	}
	fmt.Fprintln(w)

	causes := map[string]int{}
	for _, r := range results {
		if g := Group(r.Status); g == "fail" || g == "unsupported" {
			causes[r.Status+": "+normRe.ReplaceAllString(truncate(r.Detail, 120), "$1X")]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var cs []kv
	for k, v := range causes {
		cs = append(cs, kv{k, v})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].v > cs[j].v || cs[i].v == cs[j].v && cs[i].k < cs[j].k })
	fmt.Fprintln(w, "\ntop failure causes")
	for i, c := range cs {
		if i == 30 {
			break
		}
		fmt.Fprintf(w, "  %4d %s\n", c.v, c.k)
	}
}
