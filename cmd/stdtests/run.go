package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Statuses of a test in one mode.
const (
	StatusPass        = "pass"
	StatusFail        = "fail"
	StatusSkip        = "skip"
	StatusPanic       = "panic"       // the process ended in the test: panic, fatal error
	StatusTimeout     = "timeout"     // the test ran past -test.timeout
	StatusUnsupported = "unsupported" // yaegi v2 cannot run a construct of the test
	StatusError       = "error"       // the program did not build or load
	StatusNotRun      = "notrun"      // the program ended before the test, with no other cause
	StatusExcluded    = "excluded"    // the test does not build outside of its package
)

// An Outcome is the result of one test in one mode.
type Outcome struct {
	Status string
	Detail string // first lines of the failure, or the skip message
}

// Modes of a run.
const (
	ModeNative = "native"
	ModeVM     = "vm"
	ModeAOT    = "aot"
)

// A runner runs the programs.
type runner struct {
	vmhost  string        // vmhost binary
	timeout time.Duration // -test.timeout of one process
	short   bool
	verbose bool
}

// command returns the command that runs prog in mode with the test flags.
func (r *runner) command(ctx context.Context, prog *Program, mode string, flags []string) *exec.Cmd {
	var cmd *exec.Cmd
	switch mode {
	case ModeNative:
		cmd = exec.CommandContext(ctx, filepath.Join(prog.Dir, "native.bin"), flags...)
	case ModeVM:
		cmd = exec.CommandContext(ctx, r.vmhost, append([]string{prog.Dir}, flags...)...)
	case ModeAOT:
		cmd = exec.CommandContext(ctx, r.vmhost, append([]string{"-aot", prog.Dir}, flags...)...)
	}
	cmd.Dir = prog.PkgDir // testdata/ paths are relative to the package
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	ownGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// buildNative builds the program with the Go toolchain.
func buildNative(prog *Program) error {
	cmd := exec.Command("go", "build", "-o", "native.bin", ".")
	cmd.Dir = prog.Dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	return nil
}

// run runs the tests names of prog in mode until each has an outcome. A
// process that ends in a test (a panic, a timeout) runs again for the
// tests that did not run.
func (r *runner) run(prog *Program, mode string, names []string) map[string]Outcome {
	res := map[string]Outcome{}
	remaining := append([]string(nil), names...)
	for len(remaining) > 0 {
		b := r.batch(prog, mode, remaining)
		for n, o := range b.results {
			res[n] = o
		}
		var left []string
		for _, n := range remaining {
			if _, ok := res[n]; !ok {
				left = append(left, n)
			}
		}
		remaining = left
		if len(remaining) == 0 {
			break
		}
		switch {
		case !b.abnormal:
			for _, n := range remaining {
				res[n] = Outcome{StatusNotRun, "no result in the output"}
			}
			remaining = nil
		case len(b.results) == 0 && len(b.started) == 0:
			// Nothing ran: the program does not build or load, or it
			// ended before the first test.
			for _, n := range remaining {
				res[n] = Outcome{b.status, b.detail}
			}
			remaining = nil
		case len(b.started) == 0:
			// The process ended between tests: run the rest again.
		case len(b.started) == 1 || len(remaining) == 1:
			for _, n := range b.started {
				res[n] = Outcome{b.status, b.detail}
			}
		default:
			// Several tests were running (t.Parallel): run each alone to
			// know which one ended the process.
			for _, n := range b.started {
				for k, o := range r.run(prog, mode, []string{n}) {
					res[k] = o
				}
			}
		}
		if len(remaining) > 0 {
			left = left[:0]
			for _, n := range remaining {
				if _, ok := res[n]; !ok {
					left = append(left, n)
				}
			}
			remaining = left
		}
	}
	return res
}

// A batchResult is the result of one process.
type batchResult struct {
	results  map[string]Outcome
	started  []string // tests that started and have no result
	abnormal bool     // the process did not end with PASS or FAIL
	status   string   // the cause when abnormal
	detail   string
}

var (
	resultRE = regexp.MustCompile(`^--- (PASS|FAIL|SKIP): (\S+) \(`)
	runRE    = regexp.MustCompile(`^=== (RUN|NAME|CONT|PAUSE)\s+(\S+)$`)
)

func (r *runner) batch(prog *Program, mode string, names []string) batchResult {
	flags := []string{"-test.v", "-test.count=1", "-test.timeout=" + r.timeout.String(),
		"-test.run=^(" + strings.Join(names, "|") + ")$"}
	if r.short {
		flags = append(flags, "-test.short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout+time.Minute)
	defer cancel()
	cmd := r.command(ctx, prog, mode, flags)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	start := time.Now()
	err := cmd.Run()
	if r.verbose {
		fmt.Fprintf(os.Stderr, "stdtests: %s %s: %d tests, %v, %v\n", prog.Pkg, mode, len(names), time.Since(start).Round(time.Millisecond), err)
	}
	return parseOutput(out.Bytes(), err, ctx.Err() != nil, names)
}

// parseOutput reads the output of go test -v.
func parseOutput(out []byte, runErr error, killed bool, names []string) batchResult {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	b := batchResult{results: map[string]Outcome{}}
	logs := map[string][]string{} // output lines of each top-level test
	started := map[string]bool{}
	var order []string
	cur := ""
	ended := false // PASS or FAIL line at the end
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	panicAt := -1
	for i, line := range lines {
		if m := runRE.FindStringSubmatch(line); m != nil {
			top, _, _ := strings.Cut(m[2], "/")
			cur = top
			if m[1] == "RUN" && want[top] && !started[top] {
				started[top] = true
				order = append(order, top)
			}
			continue
		}
		if m := resultRE.FindStringSubmatch(line); m != nil && want[m[2]] {
			st := map[string]string{"PASS": StatusPass, "FAIL": StatusFail, "SKIP": StatusSkip}[m[1]]
			b.results[m[2]] = Outcome{Status: st}
			cur = m[2]
			continue
		}
		if line == "PASS" || line == "FAIL" {
			ended = true
			continue
		}
		if strings.HasPrefix(line, "panic: ") || strings.HasPrefix(line, "fatal error: ") {
			if panicAt < 0 {
				panicAt = i
			}
		}
		if cur != "" && panicAt < 0 {
			logs[cur] = append(logs[cur], line)
		}
	}
	for n, o := range b.results {
		o.Detail = summarize(logs[n])
		b.results[n] = o
	}

	exit := 0
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		exit = ee.ExitCode()
	} else if runErr != nil {
		exit = -1
	}
	b.abnormal = !(ended && (exit == 0 || exit == 1))
	if !b.abnormal {
		return b
	}
	tail := lines
	if panicAt >= 0 {
		tail = lines[panicAt:]
	}
	b.status, b.detail = StatusPanic, summarize(tail)
	switch {
	case killed:
		b.status, b.detail = StatusTimeout, "killed after the timeout of the process"
	case panicAt >= 0 && strings.HasPrefix(lines[panicAt], "panic: test timed out"):
		b.status = StatusTimeout
	case exit == 3:
		b.status = StatusUnsupported
	case exit == 4 || (len(b.results) == 0 && len(started) == 0 && exit != 2):
		b.status = StatusError
	}
	if b.status == StatusPanic && len(lines) > 0 && panicAt >= 0 {
		// A test that panics is reported "--- FAIL" just before the panic
		// (tRunner), after the "--- FAIL" lines of its subtests: that test
		// ended the process.
		for i := panicAt - 1; i >= 0; i-- {
			l := strings.TrimSpace(lines[i])
			if l == "" || strings.HasPrefix(lines[i], " ") && strings.HasPrefix(l, "--- FAIL: ") {
				continue
			}
			if m := resultRE.FindStringSubmatch(lines[i]); m != nil && m[1] == "FAIL" && want[m[2]] {
				b.results[m[2]] = Outcome{StatusPanic, b.detail}
				return b
			}
			break
		}
	}
	for _, n := range order {
		if _, ok := b.results[n]; !ok {
			b.started = append(b.started, n)
		}
	}
	return b
}

// summarize returns the first meaningful lines of a log, on one line.
func summarize(lines []string) string {
	var parts []string
	size := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "=== ") || strings.HasPrefix(l, "--- ") {
			continue
		}
		parts = append(parts, l)
		size += len(l)
		if size > 300 || len(parts) >= 4 {
			break
		}
	}
	s := strings.Join(parts, " | ")
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
	if len(s) > 400 {
		s = s[:400] + "..."
	}
	return s
}
