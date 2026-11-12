package main

// The hostile corpus: plugins that misbehave on purpose (testdata/hostile),
// and the guard rail of yaegi v2 that must stop each one (docs/SANDBOX.md).
// Each case
// loads its plugin with the options of the catalog host
// (pluginhost.Options), sends its requests, and checks from the host that
// the guard held: the request ended with the expected panic, the host state
// did not change, the process is still alive.
//
//	vmhost hostile -dir <yaegi v2 root> [-aot] [name...]   # all the cases by default
//
// Each case runs in a child process: a guard that fails can end the
// process (a fatal error of the runtime), and the other cases must still
// run. The command prints one line per case, "held" or "BROKEN", and exits
// with status 1 when one is broken. go test runs the corpus in both modes
// (TestHostileCorpus).
//
// The memory quota of the cases is 256 MiB instead of the 1 GiB of
// pluginhost.Options: the same mechanism, with less memory for the test.
//
// This is not a proof that yaegi v2 is a sandbox (it is not, see the README):
// the corpus checks that the guard rails that exist keep holding.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

const hostilePrefix = "github.com/traefik/yaegi/v2/compare/catalog/vmhost/testdata/hostile/"

// hostileMaxMemory is the MaxMemory of the corpus (see above).
const hostileMaxMemory = 256 << 20

// hostileEnvSecret is a variable of the host that no plugin may read.
const hostileEnvSecret = "HOSTILE_SECRET"

// hostileCase is one plugin of the corpus and the check of its guard.
type hostileCase struct {
	name  string // directory in testdata/hostile
	guard string // what must stop it
	check func(h *hostileHost) error
}

// hostileHost runs one case: the plugin, its handler, and what reached
// GoPanic.
type hostileHost struct {
	dir     string
	aot     bool
	p       *vm.Plugin
	h       http.Handler
	mu      sync.Mutex
	gopanic []any
}

// load loads the plugin and calls CreateConfig and New.
func (hh *hostileHost) load(name string) error {
	opts := pluginhost.Options()
	opts.Dir, opts.AOT, opts.MaxMemory = hh.dir, hh.aot, hostileMaxMemory
	opts.GoPanic = func(r any) {
		hh.mu.Lock()
		hh.gopanic = append(hh.gopanic, r)
		hh.mu.Unlock()
	}
	p, err := vm.Load(opts, hostilePrefix+name)
	if err != nil {
		return err
	}
	hh.p = p
	cfg, err := pluginhost.CreateConfig(p)
	if err != nil {
		return err
	}
	hh.h, err = pluginhost.NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "hostile")
	return err
}

// mustLoad is load, as an error of the case.
func (hh *hostileHost) mustLoad(name string) error {
	if err := hh.load(name); err != nil {
		return fmt.Errorf("load: %v", err)
	}
	return nil
}

// served is the outcome of one request.
type served struct {
	status  int
	body    string
	panic   any
	timeout bool
	took    time.Duration
}

// serve sends GET path to the handler, with a timeout.
func (hh *hostileHost) serve(path string, timeout time.Duration) served {
	done := make(chan served, 1)
	start := time.Now()
	go func() {
		w := httptest.NewRecorder()
		defer func() {
			if p := recover(); p != nil {
				done <- served{panic: p, took: time.Since(start)}
			}
		}()
		hh.h.ServeHTTP(w, httptest.NewRequest("GET", "http://example.com"+path, nil))
		done <- served{status: w.Code, body: w.Body.String(), took: time.Since(start)}
	}()
	select {
	case s := <-done:
		return s
	case <-time.After(timeout):
		return served{timeout: true, took: timeout}
	}
}

// panicIs checks that the request ended with a panic that is err (or wraps
// it).
func panicIs(s served, err error) error {
	if s.timeout {
		return fmt.Errorf("the request still runs after %v", s.took)
	}
	e, _ := s.panic.(error)
	if e == nil || !errors.Is(e, err) {
		return fmt.Errorf("the request ended with panic %v (status %d), want %v", s.panic, s.status, err)
	}
	return nil
}

// panicAs checks that the request ended with a panic of type *T.
func panicAs[T error](s served) error {
	if s.timeout {
		return fmt.Errorf("the request still runs after %v", s.took)
	}
	e, _ := s.panic.(error)
	var t T
	if e == nil || !errors.As(e, &t) {
		return fmt.Errorf("the request ended with panic %v (status %d), want a %T", s.panic, s.status, t)
	}
	return nil
}

// goPanics returns what GoPanic got, after waiting up to d for n of them.
func (hh *hostileHost) goPanics(n int, d time.Duration) []any {
	for end := time.Now().Add(d); ; time.Sleep(10 * time.Millisecond) {
		hh.mu.Lock()
		got := append([]any(nil), hh.gopanic...)
		hh.mu.Unlock()
		if len(got) >= n || time.Now().After(end) {
			return got
		}
	}
}

var hostileCases = []hostileCase{
	{"spin", "Budget", func(hh *hostileHost) error {
		if err := hh.mustLoad("spin"); err != nil {
			return err
		}
		return panicIs(hh.serve("/", 30*time.Second), vm.ErrBudget)
	}},
	{"initspin", "Budget of init, LoadTimeout", func(hh *hostileHost) error {
		start := time.Now()
		err := hh.load("initspin")
		if err == nil {
			return errors.New("the plugin loaded")
		}
		if d := time.Since(start); d > 30*time.Second {
			return fmt.Errorf("the load took %v", d)
		}
		return nil
	}},
	{"loadbomb", "load-time checks (loadbomb.go)", func(hh *hostileHost) error {
		start := time.Now()
		err := hh.load("loadbomb")
		if err == nil {
			return errors.New("the plugin loaded")
		}
		if d := time.Since(start); d > 10*time.Second {
			return fmt.Errorf("the load took %v: %v", d, err)
		}
		return nil
	}},
	{"bgspin", "BackgroundRate", func(hh *hostileHost) error {
		if err := hh.mustLoad("bgspin"); err != nil {
			return err
		}
		rounds := func() (int, error) {
			s := hh.serve("/", 10*time.Second)
			if s.panic != nil || s.timeout {
				return 0, fmt.Errorf("stat request: panic %v, timeout %v", s.panic, s.timeout)
			}
			return strconv.Atoi(s.body)
		}
		time.Sleep(1500 * time.Millisecond) // past the burst of one second
		n1, err := rounds()
		if err != nil {
			return err
		}
		t1 := time.Now()
		time.Sleep(2 * time.Second)
		n2, err := rounds()
		if err != nil {
			return err
		}
		// A round is about 100000 steps: rate/100000 rounds per second,
		// plus one chunk per goroutine.
		d := time.Since(t1).Seconds()
		want := int(d*pluginhost.BackgroundRate/100000) + 12
		if n2-n1 > 2*want {
			return fmt.Errorf("%d rounds in %.1f s, want at most about %d", n2-n1, d, want)
		}
		if n2 <= n1 {
			return errors.New("no round: the goroutines must be slowed, not stopped")
		}
		return nil
	}},
	{"gobomb", "MaxGoroutines", func(hh *hostileHost) error {
		if err := hh.mustLoad("gobomb"); err != nil {
			return err
		}
		if err := panicIs(hh.serve("/", 30*time.Second), vm.ErrTooManyGoroutines); err != nil {
			return err
		}
		if n, max := hh.p.M.Goroutines(), pluginhost.Options().MaxGoroutines; n > max {
			return fmt.Errorf("%d live goroutines, more than MaxGoroutines %d", n, max)
		}
		return nil
	}},
	{"timerbomb", "MaxGoroutines for the callbacks of native goroutines", func(hh *hostileHost) error {
		if err := hh.mustLoad("timerbomb"); err != nil {
			return err
		}
		if s := hh.serve("/start", 30*time.Second); s.panic != nil || s.timeout {
			return fmt.Errorf("/start: panic %v, timeout %v", s.panic, s.timeout)
		}
		max := pluginhost.Options().MaxGoroutines
		var most, ran int
		for end := time.Now().Add(30 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
			f := strings.Fields(hh.serve("/stat", 10*time.Second).body)
			if len(f) == 2 {
				most, _ = strconv.Atoi(f[0])
				ran, _ = strconv.Atoi(f[1])
			}
			if ran+len(hh.goPanics(0, 0)) >= 3000 {
				break
			}
		}
		if most > max {
			return fmt.Errorf("%d callbacks ran at the same time, more than MaxGoroutines %d", most, max)
		}
		for _, r := range hh.goPanics(0, 0) {
			if r != vm.ErrTooManyGoroutines {
				return fmt.Errorf("GoPanic got %v", r)
			}
		}
		return nil
	}},
	{"unlock", "PanicOnFatal", func(hh *hostileHost) error {
		if err := hh.mustLoad("unlock"); err != nil {
			return err
		}
		return panicAs[*vm.FatalError](hh.serve("/", 30*time.Second))
	}},
	{"maprace", "PanicOnFatal", func(hh *hostileHost) error {
		if err := hh.mustLoad("maprace"); err != nil {
			return err
		}
		s := hh.serve("/", 60*time.Second)
		if s.timeout || s.panic != nil {
			return fmt.Errorf("the request: panic %v, timeout %v", s.panic, s.timeout)
		}
		got := hh.goPanics(1, time.Second)
		if len(got) == 0 && runtime.GOMAXPROCS(0) > 1 {
			return errors.New("no goroutine of the plugin saw the race")
		}
		for _, r := range got {
			if _, ok := r.(*vm.FatalError); !ok {
				return fmt.Errorf("GoPanic got %v, want a *vm.FatalError", r)
			}
		}
		return nil
	}},
	{"membomb", "MaxMemory", func(hh *hostileHost) error {
		if err := hh.mustLoad("membomb"); err != nil {
			return err
		}
		if err := panicIs(hh.serve("/", 60*time.Second), vm.ErrMemory); err != nil {
			return err
		}
		if n := hh.p.M.Memory(); n > hostileMaxMemory {
			return fmt.Errorf("the quota counts %d bytes, more than %d", n, hostileMaxMemory)
		}
		return nil
	}},
	{"exit", "the restricted os.Exit and log.Fatal", func(hh *hostileHost) error {
		if err := hh.mustLoad("exit"); err != nil {
			return err
		}
		for _, path := range []string{"/", "/fatal"} {
			if s := hh.serve(path, 30*time.Second); s.panic == nil || s.timeout {
				return fmt.Errorf("%s: panic %v, timeout %v; want a panic", path, s.panic, s.timeout)
			}
		}
		return nil
	}},
	{"gopanic", "GoPanic", func(hh *hostileHost) error {
		if err := hh.mustLoad("gopanic"); err != nil {
			return err
		}
		if s := hh.serve("/", 30*time.Second); s.panic != nil || s.timeout {
			return fmt.Errorf("the request: panic %v, timeout %v", s.panic, s.timeout)
		}
		if got := hh.goPanics(2, 10*time.Second); len(got) != 2 {
			return fmt.Errorf("GoPanic got %v, want the two panics", got)
		}
		return nil
	}},
	{"env", "the environment copy (Env)", func(hh *hostileHost) error {
		os.Setenv(hostileEnvSecret, "s3cr3t-of-the-host")
		defer os.Unsetenv(hostileEnvSecret)
		if err := hh.mustLoad("env"); err != nil {
			return err
		}
		s := hh.serve("/", 30*time.Second)
		if s.panic != nil || s.timeout {
			return fmt.Errorf("the request: panic %v, timeout %v", s.panic, s.timeout)
		}
		if strings.Contains(s.body, "s3cr3t") || strings.Contains(s.body, hostileEnvSecret) {
			return fmt.Errorf("the plugin read the environment of the host: %q", s.body)
		}
		return nil
	}},
	{"stdio", "the plugin's own os.Stdout and os.Stderr", func(hh *hostileHost) error {
		if err := hh.mustLoad("stdio"); err != nil {
			return err
		}
		if s := hh.serve("/", 30*time.Second); s.panic != nil || s.timeout {
			return fmt.Errorf("the request: panic %v, timeout %v", s.panic, s.timeout)
		}
		for _, f := range []*os.File{os.Stdout, os.Stderr} {
			if _, err := f.Stat(); err != nil {
				return fmt.Errorf("the host lost %s: %v", f.Name(), err)
			}
		}
		return nil
	}},
	{"hostwrite", "the per-plugin registries and the host memory checks", func(hh *hostileHost) error {
		if err := hh.mustLoad("hostwrite"); err != nil {
			return err
		}
		ip := net.IPv4bcast.String()
		if err := panicAs[*vm.RefusedError](hh.serve("/", 30*time.Second)); err != nil {
			return err
		}
		if http.DefaultClient.Timeout != 0 {
			return fmt.Errorf("the host's http.DefaultClient has the timeout %v", http.DefaultClient.Timeout)
		}
		if t := http.DefaultTransport.(*http.Transport); t.MaxIdleConns != 100 {
			return fmt.Errorf("the host's http.DefaultTransport has MaxIdleConns %d", t.MaxIdleConns)
		}
		if got := net.IPv4bcast.String(); got != ip {
			return fmt.Errorf("the host's net.IPv4bcast is %s, was %s", got, ip)
		}
		return nil
	}},
	{"mux", "the per-plugin DefaultServeMux and flag.CommandLine", func(hh *hostileHost) error {
		if err := hh.load("mux"); err != nil {
			// flag.Parse of the plugin parses the arguments of its own
			// copy of os.Args: an unknown flag of the host may make it
			// end with the restricted os.Exit, which is fine.
			if !strings.Contains(err.Error(), "os.Exit") {
				return fmt.Errorf("load: %v", err)
			}
		}
		if _, pattern := http.DefaultServeMux.Handler(httptest.NewRequest("GET", "/hostile", nil)); pattern != "" {
			return fmt.Errorf("the host's http.DefaultServeMux has the route %q", pattern)
		}
		if flag.Lookup("hostile-flag") != nil {
			return errors.New("the host's flag.CommandLine has the flag of the plugin")
		}
		return nil
	}},
	{"recursion", "MaxDepth and MaxNest", func(hh *hostileHost) error {
		if err := hh.mustLoad("recursion"); err != nil {
			return err
		}
		for _, path := range []string{"/", "/map"} {
			if err := panicIs(hh.serve(path, 60*time.Second), vm.ErrStackOverflow); err != nil {
				return fmt.Errorf("%s: %v", path, err)
			}
		}
		return nil
	}},
	{"newfile", "the refusal of os.NewFile", func(hh *hostileHost) error {
		if err := hh.mustLoad("newfile"); err != nil {
			return err
		}
		return panicAs[*vm.RefusedError](hh.serve("/", 30*time.Second))
	}},
}

// hostileEnv names the case that a child process runs.
const hostileEnv = "VMHOST_HOSTILE"

// runHostileCase runs the case name in this process and prints its line.
func runHostileCase(w io.Writer, dir string, aot bool, name string) bool {
	for _, c := range hostileCases {
		if c.name != name {
			continue
		}
		hh := &hostileHost{dir: dir, aot: aot}
		if err := c.check(hh); err != nil {
			fmt.Fprintf(w, "hostile %s: BROKEN (%s): %v\n", c.name, c.guard, err)
			return false
		}
		fmt.Fprintf(w, "hostile %s: held (%s)\n", c.name, c.guard)
		return true
	}
	fmt.Fprintf(w, "hostile %s: BROKEN: no such case\n", name)
	return false
}

// runHostileChildren runs each case in a child process made by cmd, and
// prints one line per case. A child that dies is a broken guard.
func runHostileChildren(w io.Writer, names []string, cmd func(name string) *exec.Cmd) bool {
	ok := true
	for _, name := range names {
		c := cmd(name)
		c.Env = append(os.Environ(), hostileEnv+"="+name)
		out, err := c.CombinedOutput()
		line := ""
		for _, l := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(l, "hostile "+name+": ") {
				line = l
			}
		}
		switch {
		case line == "":
			fmt.Fprintf(w, "hostile %s: BROKEN: the host process ended (%v)\n%s\n", name, err, tail(string(out), 20))
			ok = false
		case strings.Contains(line, "BROKEN") || err != nil:
			fmt.Fprintln(w, line)
			ok = false
		default:
			fmt.Fprintln(w, line)
		}
	}
	return ok
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	l := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

// hostileNames returns the names of all the cases.
func hostileNames() []string {
	var n []string
	for _, c := range hostileCases {
		n = append(n, c.name)
	}
	return n
}

// hostileMain is the command "vmhost hostile": it returns the exit status.
func hostileMain(args []string) int {
	fs := flag.NewFlagSet("hostile", flag.ExitOnError)
	dir := fs.String("dir", ".", "root of the yaegi v2 module (the corpus is in compare/catalog/vmhost/testdata/hostile)")
	aotMode := fs.Bool("aot", false, "run the packages of aot/ in AOT mode")
	fs.Parse(args)
	if name := os.Getenv(hostileEnv); name != "" {
		if !runHostileCase(os.Stdout, *dir, *aotMode, name) {
			return 1
		}
		return 0
	}
	names := fs.Args()
	if len(names) == 0 {
		names = hostileNames()
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ok := runHostileChildren(os.Stdout, names, func(string) *exec.Cmd {
		return exec.Command(exe, append([]string{"hostile"}, args...)...)
	})
	if !ok {
		return 1
	}
	return 0
}
