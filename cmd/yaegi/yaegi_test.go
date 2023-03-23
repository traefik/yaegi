package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	// CITimeoutMultiplier is the multiplier for all timeouts in the CI.
	CITimeoutMultiplier = 3
)

// Sleep pauses the current goroutine for at least the duration d.
func Sleep(d time.Duration) {
	d = applyCIMultiplier(d)
	time.Sleep(d)
}

func applyCIMultiplier(timeout time.Duration) time.Duration {
	ci := os.Getenv("CI")
	if ci == "" {
		return timeout
	}
	b, err := strconv.ParseBool(ci)
	if err != nil || !b {
		return timeout
	}
	return time.Duration(float64(timeout) * CITimeoutMultiplier)
}

func TestYaegiCmdCancel(t *testing.T) {
	tmp := t.TempDir()
	yaegi := filepath.Join(tmp, "yaegi")

	args := []string{"build"}
	if raceDetectorSupported(runtime.GOOS, runtime.GOARCH) {
		args = append(args, "-race")
	}
	args = append(args, "-o", yaegi, ".")

	build := exec.Command("go", args...)

	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build yaegi command: %v: %s", err, out)
	}

	// Test src must be terminated by a single newline.
	tests := []string{
		"for {}\n",
		"select {}\n",
	}
	for _, src := range tests {
		cmd := exec.Command(yaegi)
		in, err := cmd.StdinPipe()
		if err != nil {
			t.Errorf("failed to get stdin pipe to yaegi command: %v", err)
		}
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		// https://golang.org/doc/articles/race_detector.html#Options
		cmd.Env = []string{`GORACE="halt_on_error=1"`}

		err = cmd.Start()
		if err != nil {
			t.Fatalf("failed to start yaegi command: %v", err)
		}

		_, err = in.Write([]byte(src))
		if err != nil {
			t.Errorf("failed pipe test source to yaegi command: %v", err)
		}
		Sleep(500 * time.Millisecond)
		err = cmd.Process.Signal(os.Interrupt)
		if err != nil {
			t.Errorf("failed to send os.Interrupt to yaegi command: %v", err)
		}

		_, err = in.Write([]byte("1+1\n"))
		if err != nil {
			t.Errorf("failed to probe race: %v", err)
		}
		err = in.Close()
		if err != nil {
			t.Errorf("failed to close stdin pipe: %v", err)
		}

		err = cmd.Wait()
		if err != nil {
			if cmd.ProcessState.ExitCode() == 66 { // See race_detector.html article.
				t.Errorf("race detected running yaegi command canceling %q: %v", src, err)
				if testing.Verbose() {
					t.Log(&errBuf)
				}
			} else {
				t.Errorf("error running yaegi command for %q: %v", src, err)
			}
			continue
		}

		if strings.TrimSuffix(errBuf.String(), "\n") != context.Canceled.Error() {
			t.Errorf("unexpected error: %q", &errBuf)
		}
	}
}

func raceDetectorSupported(goos, goarch string) bool {
	if strings.Contains(os.Getenv("GOFLAGS"), "-buildmode=pie") {
		// The Go race detector is not compatible with position independent code (pie).
		// We read the conventional GOFLAGS env variable used for example on AlpineLinux
		// to build packages, as there is no way to get this information from the runtime.
		return false
	}
	switch goos {
	case "linux":
		return goarch == "amd64" || goarch == "ppc64le" || goarch == "arm64"
	case "darwin":
		return goarch == "amd64" || goarch == "arm64"
	case "freebsd", "netbsd", "openbsd", "windows":
		return goarch == "amd64"
	default:
		return false
	}
}
