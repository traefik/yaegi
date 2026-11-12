package main

import (
	"os/exec"
	"strings"
	"testing"
)

// exitErr returns an *exec.ExitError with the exit code code.
func exitErr(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+string(rune('0'+code))).Run()
	if err == nil {
		t.Fatal("no error")
	}
	return err
}

func TestParseOutput(t *testing.T) {
	out := strings.Join([]string{
		"=== RUN   TestA",
		"=== RUN   TestA/sub",
		"--- PASS: TestA (0.00s)",
		"    --- PASS: TestA/sub (0.00s)",
		"=== RUN   TestB",
		"    x_test.go:3: boom",
		"--- FAIL: TestB (0.00s)",
		"=== RUN   TestC",
		"    x_test.go:9: skipped here",
		"--- SKIP: TestC (0.00s)",
		"=== RUN   ExampleD",
		"--- PASS: ExampleD (0.00s)",
		"FAIL",
	}, "\n")
	b := parseOutput([]byte(out), exitErr(t, 1), false, []string{"TestA", "TestB", "TestC", "ExampleD"})
	if b.abnormal {
		t.Fatal("abnormal")
	}
	want := map[string]string{"TestA": StatusPass, "TestB": StatusFail, "TestC": StatusSkip, "ExampleD": StatusPass}
	for n, st := range want {
		if b.results[n].Status != st {
			t.Errorf("%s: %q, want %q", n, b.results[n].Status, st)
		}
	}
	if b.results["TestB"].Detail != "x_test.go:3: boom" || b.results["TestC"].Detail != "x_test.go:9: skipped here" {
		t.Errorf("details: %q %q", b.results["TestB"].Detail, b.results["TestC"].Detail)
	}
}

// A test that panics is reported "--- FAIL" before the panic, after the
// lines of its subtests: it is the test that ended the process.
func TestParseOutputPanic(t *testing.T) {
	out := strings.Join([]string{
		"=== RUN   TestA",
		"--- PASS: TestA (0.00s)",
		"=== RUN   TestB",
		"=== RUN   TestB/v1",
		"--- FAIL: TestB (0.00s)",
		"    --- FAIL: TestB/v1 (0.00s)",
		"panic: unsupported: something [recovered, repanicked]",
		"",
		"goroutine 7 [running]:",
	}, "\n")
	b := parseOutput([]byte(out), exitErr(t, 2), false, []string{"TestA", "TestB", "TestC"})
	if !b.abnormal || b.results["TestB"].Status != StatusPanic || len(b.started) != 0 {
		t.Fatalf("got %+v", b)
	}
	if !strings.HasPrefix(b.results["TestB"].Detail, "panic: unsupported: something") {
		t.Errorf("detail %q", b.results["TestB"].Detail)
	}
	if _, ok := b.results["TestC"]; ok {
		t.Error("TestC has a result")
	}
}

// A timeout names the tests that were running.
func TestParseOutputTimeout(t *testing.T) {
	out := strings.Join([]string{
		"=== RUN   TestA",
		"--- PASS: TestA (0.00s)",
		"=== RUN   TestSlow",
		"panic: test timed out after 3m0s",
		"\trunning tests:",
		"\t\tTestSlow (3m0s)",
	}, "\n")
	b := parseOutput([]byte(out), exitErr(t, 2), false, []string{"TestA", "TestSlow", "TestC"})
	if !b.abnormal || b.status != StatusTimeout || len(b.started) != 1 || b.started[0] != "TestSlow" {
		t.Fatalf("got %+v", b)
	}
}

// Nothing ran: the program did not load (exit code 4 of vmhost).
func TestParseOutputLoadError(t *testing.T) {
	b := parseOutput([]byte("x.go:1:1: undefined: y\n"), exitErr(t, 4), false, []string{"TestA"})
	if !b.abnormal || b.status != StatusError || len(b.results) != 0 || len(b.started) != 0 {
		t.Fatalf("got %+v", b)
	}
}

func TestIsTestName(t *testing.T) {
	for _, c := range []struct {
		name, prefix string
		want         bool
	}{
		{"TestA", "Test", true}, {"Test", "Test", true}, {"Testing", "Test", false},
		{"Test_x", "Test", true}, {"Example", "Example", true}, {"Examples", "Example", false},
	} {
		if got := isTestName(c.name, c.prefix); got != c.want {
			t.Errorf("isTestName(%q, %q) = %v", c.name, c.prefix, got)
		}
	}
}
