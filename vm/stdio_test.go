package vm

import (
	"bytes"
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a bytes.Buffer that the pipe goroutines of SetStdio and the
// test can use at the same time.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// lines waits until b holds n lines (the writes to os.Stdout and os.Stderr
// go through a pipe and come a short time later), and returns them sorted.
func (b *syncBuffer) lines(n int) []string {
	deadline := time.Now().Add(10 * time.Second)
	for {
		s := b.String()
		if strings.Count(s, "\n") >= n || time.Now().After(deadline) {
			ls := strings.SplitAfter(s, "\n")
			if ls[len(ls)-1] == "" {
				ls = ls[:len(ls)-1]
			}
			slices.Sort(ls)
			return ls
		}
		time.Sleep(time.Millisecond)
	}
}

// TestPluginStdio checks Options.Stdout and Options.Stderr: fmt.Print*,
// os.Stdout, os.Stderr, the default logger of the log package and the
// print and println builtins of the plugin go to them, as with the Stdout
// and Stderr of yaegi in Traefik. Two plugins with their own writers do not
// mix, and the host keeps its stdout, its stderr and its default logger
// (log.SetFlags in the plugin changes the plugin's logger only). The real
// file descriptors are checked in a subprocess.
func TestPluginStdio(t *testing.T) {
	if os.Getenv("YAEGI_STDIO_CHILD") == "1" {
		pluginStdioChild(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPluginStdio$", "-test.count=1")
	cmd.Env = append(os.Environ(), "YAEGI_STDIO_CHILD=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil || childStdout(stdout.String()) != "PASS\n" || stderr.Len() != 0 {
		t.Fatalf("subprocess: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
}

// childStdout returns the stdout of a test binary run as a child process
// without the line that -cover adds ("coverage: 12.3% of statements"): the
// tests compare the rest with "PASS\n", to see that nothing else wrote to
// the stdout of the host.
func childStdout(out string) string {
	var b strings.Builder
	for line := range strings.Lines(out) {
		if !strings.HasPrefix(line, "coverage: ") {
			b.WriteString(line)
		}
	}
	return b.String()
}

func pluginStdioChild(t *testing.T) {
	serve := func(name string, aot bool) (out, errOut *syncBuffer) {
		out, errOut = new(syncBuffer), new(syncBuffer)
		p, err := Load(Options{Dir: "..", AOT: aot, Stdout: out, Stderr: errOut}, "github.com/traefik/yaegi/v2/vm/testdata/stdioplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, name)
		if err != nil {
			t.Fatal(err)
		}
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		return out, errOut
	}
	outA, errA := serve("A", false)
	outB, errB := serve("B", false)
	outC, errC := serve("C", true) // AOT
	for _, c := range []struct {
		name     string
		out, err *syncBuffer
	}{{"A", outA, errA}, {"B", outB, errB}, {"C", outC, errC}} {
		wantOut := []string{"fmt.Print X\n", "fmt.Printf X\n", "fmt.Println X\n", "os.Stdout.Write X\n"}
		wantErr := []string{"fmt.Fprintln(os.Stderr) X\n", "log.Default X\n", "log.Fatal X\n", "log.Println X\n", "log.Writer X\n", "println X\n"}
		for _, want := range [][]string{wantOut, wantErr} {
			for i := range want {
				want[i] = strings.Replace(want[i], "X", c.name, 1)
			}
			slices.Sort(want)
		}
		if got := c.out.lines(len(wantOut)); !slices.Equal(got, wantOut) {
			t.Errorf("plugin %s: Stdout got %q, want %q", c.name, got, wantOut)
		}
		if got := c.err.lines(len(wantErr)); !slices.Equal(got, wantErr) {
			t.Errorf("plugin %s: Stderr got %q, want %q", c.name, got, wantErr)
		}
	}
	if log.Flags() != log.LstdFlags || log.Writer() != os.Stderr {
		t.Errorf("the plugin changed the default logger of the host: flags %d, writer %v", log.Flags(), log.Writer())
	}
}

// TestPluginAbortHandler checks that panic(http.ErrAbortHandler) in a
// plugin handler reaches net/http as itself: the server aborts the
// response without logging "panic serving" and a stack, as for a native
// handler. Other panics still reach the server, which logs them.
func TestPluginAbortHandler(t *testing.T) {
	for _, aot := range []bool{false, true} {
		p, err := Load(Options{Dir: "..", AOT: aot, Stdout: io.Discard, Stderr: io.Discard}, "github.com/traefik/yaegi/v2/vm/testdata/stdioplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "abort")
		if err != nil {
			t.Fatal(err)
		}
		errorLog := new(syncBuffer)
		srv := httptest.NewUnstartedServer(h)
		srv.Config.ErrorLog = log.New(errorLog, "", 0)
		srv.Start()

		resp, err := http.Get(srv.URL + "/abort")
		if err == nil {
			_, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if err == nil {
			t.Errorf("AOT %v: the client got a complete response, want an aborted one", aot)
		}
		if got := errorLog.String(); got != "" {
			t.Errorf("AOT %v: ErrorLog after http.ErrAbortHandler: %q, want nothing", aot, got)
		}

		if resp, err := http.Get(srv.URL + "/boom"); err == nil {
			resp.Body.Close()
		}
		srv.Close() // waits for the handlers
		if got := errorLog.String(); !strings.Contains(got, "panic serving") || !strings.Contains(got, "boom") {
			t.Errorf("AOT %v: ErrorLog after panic(\"boom\"): %q, want the panic", aot, got)
		}
	}
}

// TestPluginHostGlobals checks that a plugin loaded with Options{} (no
// Stdout and no Stderr) cannot change the globals of the host: it assigns
// os.Stdin, os.Stdout and os.Stderr and calls log.SetPrefix, log.SetFlags
// and log.SetOutput, and sees its own changes, but the host variables and
// its default logger stay the same. Its fmt.Println goes to the host
// os.Stdout and its log.Println to the writer of the host default logger.
// Its fmt.Scan reads its own os.Stdin.
func TestPluginHostGlobals(t *testing.T) {
	stdin0, stdout0, stderr0 := os.Stdin, os.Stdout, os.Stderr
	prefix0, flags0, out0 := log.Prefix(), log.Flags(), log.Writer()
	defer func() { // a failure must not break the other tests
		os.Stdin, os.Stdout, os.Stderr = stdin0, stdout0, stderr0
		log.SetPrefix(prefix0)
		log.SetFlags(flags0)
		log.SetOutput(out0)
	}()
	for _, aot := range []bool{false, true} {
		// Not t.TempDir: on Windows a child of the go command (go list)
		// can still hold the file when the test ends, and the cleanup of
		// t.TempDir fails the test.
		dir, err := os.MkdirTemp("", "hostglobals")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		stdout, err := os.Create(filepath.Join(dir, "stdout"))
		if err != nil {
			t.Fatal(err)
		}
		logOut := new(syncBuffer)
		os.Stdout = stdout
		log.SetOutput(logOut)
		log.SetPrefix("HOST ")
		log.SetFlags(0)
		stdin, stderr := os.Stdin, os.Stderr

		p, err := Load(Options{Dir: "..", AOT: aot}, "github.com/traefik/yaegi/v2/vm/testdata/hostglobplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "hostglob")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if got, want := w.Body.String(), "42 PLUGPFX true true true"; got != want {
			t.Errorf("AOT %v: the plugin sees %q, want %q", aot, got, want)
		}
		if os.Stdin != stdin || os.Stdout != stdout || os.Stderr != stderr {
			t.Errorf("AOT %v: the plugin changed os.Stdin, os.Stdout or os.Stderr of the host", aot)
		}
		if log.Prefix() != "HOST " || log.Flags() != 0 || log.Writer() != logOut {
			t.Errorf("AOT %v: the plugin changed the default logger of the host: prefix %q, flags %d, writer %v", aot, log.Prefix(), log.Flags(), log.Writer())
		}
		stdout.Close()
		if b, _ := os.ReadFile(stdout.Name()); string(b) != "fmt.Println hostglob\n" {
			t.Errorf("AOT %v: host os.Stdout got %q, want the fmt.Println of the plugin", aot, b)
		}
		if got := logOut.String(); !strings.HasSuffix(got, " log.Println hostglob\n") || strings.Contains(got, "not printed") {
			t.Errorf("AOT %v: host log writer got %q, want only the log.Println of the plugin", aot, got)
		}
		os.Stdin, os.Stdout, os.Stderr = stdin0, stdout0, stderr0
	}
}

// TestPluginSlogDefault checks that slog.SetDefault and
// slog.SetLogLoggerLevel in a plugin change the plugin only. In Go,
// SetDefault also calls log.SetOutput and log.SetFlags(0), so that the
// default logger of log writes to the new handler: the plugin sees this
// for its own loggers, but the host keeps its log.Writer(), log.Flags(),
// slog.Default() and the level of its log bridge, in every mode, with and
// without writers.
func TestPluginSlogDefault(t *testing.T) {
	out0, flags0, slog0 := log.Writer(), log.Flags(), slog.Default()
	defer func() { // a failure must not break the other tests
		log.SetOutput(out0)
		log.SetFlags(flags0)
		slog.SetDefault(slog0)
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}()
	const want = `level=INFO msg=log.Print
level=INFO msg=slog.Info k=1
level=WARN msg=Default.Warn
level=WARN msg="log.Print at warn"
level=ERROR msg=slog.LogAttrs n=2
true INFO 0
`
	for _, c := range []struct {
		name string
		opts Options
	}{
		{"default", Options{}},
		{"aot", Options{AOT: true}},
		{"unrestricted", Options{Unrestricted: true}},
		{"realprocess", Options{Unrestricted: true, RealProcess: true}},
		{"sandbox", Options{Sandbox: &Sandbox{}}},
		{"sandbox-aot", Options{Sandbox: &Sandbox{}, AOT: true}},
	} {
		for _, writers := range []bool{false, true} {
			opts := c.opts
			opts.Dir = ".."
			stderr := new(syncBuffer)
			if writers {
				opts.Stdout, opts.Stderr = io.Discard, stderr
			}
			p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/slogplug")
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "slog")
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if got := w.Body.String(); got != want {
				t.Errorf("%s, writers %v: the plugin got %q, want %q", c.name, writers, got, want)
			}
			if got := stderr.String(); got != "" {
				t.Errorf("%s, writers %v: Stderr got %q, want nothing (the log of the plugin goes to its slog handler)", c.name, writers, got)
			}
			if log.Writer() != out0 || log.Flags() != flags0 || slog.Default() != slog0 {
				t.Errorf("%s, writers %v: the plugin changed the host defaults: log writer %T, log flags %d, slog default changed %v",
					c.name, writers, log.Writer(), log.Flags(), slog.Default() != slog0)
			}
			if lvl := slog.SetLogLoggerLevel(slog.LevelInfo); lvl != slog.LevelInfo {
				t.Errorf("%s, writers %v: the plugin changed the level of the host log bridge to %v", c.name, writers, lvl)
			}
			log.SetOutput(out0)
			log.SetFlags(flags0)
			slog.SetDefault(slog0)
		}
	}
}

// TestPluginSlogDefaultHandler checks the default logger of log/slog before
// slog.SetDefault, in every mode: its handler writes through the default
// logger of the host log package (the plugin's Stderr does not get it, as
// documented). In AOT mode, the copy of log/slog has its own copy of
// log/internal, whose DefaultOutput nothing set: slog.Info panicked, in the
// New of the plugin.
func TestPluginSlogDefaultHandler(t *testing.T) {
	out0, flags0, prefix0 := log.Writer(), log.Flags(), log.Prefix()
	defer func() {
		log.SetOutput(out0)
		log.SetFlags(flags0)
		log.SetPrefix(prefix0)
	}()
	const want = "host: INFO New k=1\nhost: WARN ServeHTTP path=/p\n"
	for _, aot := range []bool{false, true} {
		for _, writers := range []bool{false, true} {
			hostLog := new(syncBuffer)
			log.SetOutput(hostLog)
			log.SetFlags(0)
			log.SetPrefix("host: ")
			opts := Options{Dir: "..", AOT: aot}
			stderr := new(syncBuffer)
			if writers {
				opts.Stdout, opts.Stderr = io.Discard, stderr
			}
			p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/slogdefplug")
			if err != nil {
				t.Fatalf("AOT %v: %v", aot, err)
			}
			cfg, err := createConfig(p)
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "slog")
			if err != nil {
				t.Fatalf("AOT %v, writers %v: New: %v", aot, writers, err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
			if got := w.Body.String(); got != "ok\n" {
				t.Errorf("AOT %v, writers %v: the plugin got %q, want \"ok\\n\"", aot, writers, got)
			}
			if got := hostLog.String(); got != want {
				t.Errorf("AOT %v, writers %v: the host log got %q, want %q", aot, writers, got, want)
			}
			if got := stderr.String(); got != "" {
				t.Errorf("AOT %v, writers %v: Stderr got %q, want nothing", aot, writers, got)
			}
		}
	}
}

// TestChildStdout checks childStdout: `go test -cover ./vm` failed because
// the child processes of TestHostMem and TestPluginStdio print a coverage
// line after PASS.
func TestChildStdout(t *testing.T) {
	for in, want := range map[string]string{
		"PASS\n":                                    "PASS\n",
		"PASS\ncoverage: 19.3% of statements\n":     "PASS\n",
		"hello\nPASS\ncoverage: 1% of statements\n": "hello\nPASS\n",
	} {
		if got := childStdout(in); got != want {
			t.Errorf("childStdout(%q) = %q, want %q", in, got, want)
		}
	}
}
