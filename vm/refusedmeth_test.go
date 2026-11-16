package vm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestRefusedMethods checks that a restricted plugin cannot call
// (*os.File).Chdir without naming it (refusedmeth.go): through its reflect,
// an interface, a promoted method and the templates, in the default mode
// and in AOT mode. The working directory of the host must not change.
func TestRefusedMethods(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	dir := t.TempDir()
	paths := []string{
		"/methodbyname", "/method", "/methodinterface", "/typemethod", "/typemethodindex",
		"/interface", "/embedreflect", "/template", "/templatemap", "/htmltemplate",
	}
	changePaths := []string{"/templatetree", "/htmltemplatetree", "/templateadd", "/templatenested"}
	// What the native templates give for changePaths: the copy runs without
	// the change, the added template is not in the copy, a template may not
	// call Execute. In default mode the plugin gets the AOT copies of the
	// templates when the host has them (stdAOTModes): nativeTmpl keeps the
	// native ones.
	nativeTmpl := map[string]Mode{"text/template": ModeNative, "html/template": ModeNative}
	wantDefault := map[string]string{
		"/templateadd":    `template "x" not defined`,
		"/templatenested": "the method Execute, which is not allowed in a plugin",
	}
	// serve runs path and returns the panic or the error of the plugin, and
	// whether the working directory changed.
	serve := func(h http.Handler, path string) (msg string, changed bool) {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
			now, _ := os.Getwd()
			changed = now != wd
			os.Chdir(wd)
		}()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path+"?dir="+dir, nil))
		return rec.Body.String(), false
	}
	load := func(opts Options) http.Handler {
		t.Helper()
		opts.Dir = ".."
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/chdirplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "chdir")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	for _, opts := range []Options{{}, {Modes: nativeTmpl}, {AOT: true}} {
		aot := opts.AOT
		h := load(opts)
		for _, path := range paths {
			msg, changed := serve(h, path)
			if changed {
				t.Errorf("aot %v, %s: the working directory of the host changed", aot, path)
			}
			if !strings.Contains(msg, "Chdir") || !strings.Contains(msg, "is not allowed in a plugin") {
				t.Errorf("aot %v, %s: got %q, want a refusal of Chdir", aot, path, msg)
			}
		}
		// A template that calls no refused method still runs.
		if msg, changed := serve(h, "/templateok"); changed || msg != dir {
			t.Errorf("aot %v, /templateok: got %q (changed %v), want %q", aot, msg, changed, dir)
		}
	}
	// The plugin changes the templates while they run (a FuncMap function
	// changes a tree or adds a template), or a template executes another
	// template. The working directory must not change. The native
	// templates run a private copy of the templates, so the change has no
	// effect or the added template is not found; the AOT copies (-aot, and
	// default mode with the copies) refuse the call to Chdir.
	for _, opts := range []Options{{}, {Modes: nativeTmpl}, {AOT: true}} {
		h := load(opts)
		for _, path := range changePaths {
			msg, changed := serve(h, path)
			if changed {
				t.Errorf("aot %v, modes %v, %s: the working directory of the host changed (%q)", opts.AOT, opts.Modes, path, msg)
			}
			if want := wantDefault[path]; opts.Modes != nil && !strings.Contains(msg, want) {
				t.Errorf("native templates, %s: got %q, want %q", path, msg, want)
			}
		}
	}
	// Without the restrictions, the method runs as in Go.
	h := load(Options{Unrestricted: true})
	for _, path := range append(paths, changePaths...) {
		if msg, changed := serve(h, path); !changed {
			t.Errorf("unrestricted, %s: the working directory did not change (%q)", path, msg)
		}
	}
}

// TestRefusedProcessMethods checks that a restricted plugin cannot signal
// a process that it builds itself (&os.Process{Pid: n}) when it does not
// name the methods (procsigplug), that ExecReal still runs them, and that
// with ExecMock the native (*exec.Cmd).Wait does not wait for a Process
// that the plugin set (procwaitplug).
func TestRefusedProcessMethods(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("needs a unix sleep")
	}
	// sleeper starts a child of the test; exited is closed when it ends (a
	// killed child that nobody waited for still answers signal 0).
	type proc struct {
		pid    int
		exited chan struct{}
	}
	sleeper := func() proc {
		t.Helper()
		cmd := exec.Command("sleep", "60")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		p := proc{cmd.Process.Pid, make(chan struct{})}
		go func() { cmd.Wait(); close(p.exited) }()
		t.Cleanup(func() { cmd.Process.Kill(); <-p.exited })
		return p
	}
	alive := func(p proc, wait time.Duration) bool {
		select {
		case <-p.exited:
			return false
		case <-time.After(wait):
			return true
		}
	}
	load := func(opts Options) http.Handler {
		t.Helper()
		opts.Dir = ".."
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/procsigplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "procsig")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	serve := func(h http.Handler, path string, pid int) (msg string) {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
		}()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", fmt.Sprintf("%s?pid=%d", path, pid), nil))
		return rec.Body.String()
	}
	paths := []string{"/interface", "/reflect"}
	child := sleeper()
	for _, opts := range []Options{{}, {AOT: true}, {Exec: ExecMock}} {
		h := load(opts)
		for _, path := range paths {
			msg := serve(h, path, child.pid)
			if !strings.Contains(msg, "(*os.Process).") || !strings.Contains(msg, "is not allowed in a plugin") {
				t.Errorf("aot %v, exec %v, %s: got %q, want a refusal", opts.AOT, opts.Exec, path, msg)
			}
			if !alive(child, 200*time.Millisecond) {
				t.Fatalf("aot %v, exec %v, %s: the plugin killed the process", opts.AOT, opts.Exec, path)
			}
		}
	}
	// ExecReal runs programs: the methods run as in Go.
	h := load(Options{Exec: ExecReal})
	for _, path := range paths {
		c := sleeper()
		if msg := serve(h, path, c.pid); msg != "" {
			t.Errorf("ExecReal, %s: %q", path, msg)
		}
		if alive(c, 5*time.Second) {
			t.Errorf("ExecReal, %s: the process still runs", path)
		}
	}

	// ExecMock: Wait of a Cmd whose Process the plugin set returns at once.
	p, err := Load(Options{Dir: "..", Exec: ExecMock}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/procwaitplug")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() {
		res, err := p.Call(context.Background(), "Wait", child.pid)
		if err != nil {
			done <- err.Error()
			return
		}
		v, _ := res[0].Interface()
		done <- fmt.Sprint(v)
	}()
	select {
	case got := <-done:
		if got != "exec: not started" {
			t.Errorf("ExecMock Wait: got %q, want %q", got, "exec: not started")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ExecMock Wait: the plugin waits for the process of the host")
	}
	if !alive(child, 200*time.Millisecond) {
		t.Error("ExecMock Wait: the process does not run any more")
	}
}

// TestRefusedTestingMRun checks that a restricted plugin cannot call
// (*testing.M).Run when it does not name it (mrunplug): the native Run
// parses the host's flags and ends the host with os.Exit.
func TestRefusedTestingMRun(t *testing.T) {
	for _, opts := range []Options{{}, {AOT: true}} {
		opts.Dir = ".."
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/mrunplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "mrun")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/interface", "/reflect"} {
			msg := func() (msg string) {
				defer func() {
					if r := recover(); r != nil {
						msg = fmt.Sprint(r)
					}
				}()
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
				return ""
			}()
			if !strings.Contains(msg, "(*testing.M).Run is not allowed in a plugin") {
				t.Errorf("aot %v, %s: got %q, want a refusal of (*testing.M).Run", opts.AOT, path, msg)
			}
		}
	}
}

// TestRefusedTestingMethods checks that a restricted plugin cannot change
// the working directory or the environment of the host with Chdir and
// Setenv of package testing, which call the real os.Chdir and os.Setenv,
// when it does not name them (testenvplug).
func TestRefusedTestingMethods(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	const key = "YAEGI_TESTENVPLUG"
	defer os.Unsetenv(key)
	dir := t.TempDir()
	paths := []string{"/tbchdir", "/tbsetenv", "/bchdir", "/bsetenv", "/reflectchdir", "/reflectsetenv", "/interface", "/embed", "/methodexpr", "/generic"}
	for _, opts := range []Options{{}, {AOT: true}} {
		opts.Dir = ".."
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/testenvplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "testenv")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			msg := func() (msg string) {
				defer func() {
					if r := recover(); r != nil {
						msg = fmt.Sprint(r)
					}
				}()
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path+"?dir="+dir+"&key="+key, nil))
				return ""
			}()
			if now, _ := os.Getwd(); now != wd {
				t.Errorf("aot %v, %s: the working directory of the host changed", opts.AOT, path)
				os.Chdir(wd)
			}
			if _, ok := os.LookupEnv(key); ok {
				t.Errorf("aot %v, %s: the environment of the host changed", opts.AOT, path)
				os.Unsetenv(key)
			}
			if !strings.Contains(msg, "testing.") || !strings.Contains(msg, "is not allowed in a plugin") {
				t.Errorf("aot %v, %s: got %q, want a refusal of a method of testing", opts.AOT, path, msg)
			}
		}
	}
}

// TestRefusedMethodsComplete checks that each method of refusedFuncs is
// refused at run time too.
func TestRefusedMethodsComplete(t *testing.T) {
	var native []string
	for _, what := range refusedNative {
		native = append(native, what)
	}
	for name := range refusedFuncs {
		if strings.HasPrefix(name, "(") && !slices.Contains(native, name) {
			t.Errorf("%s is in refusedFuncs but not in refusedNative", name)
		}
	}
	for k := range refusedMethods() {
		if _, ok := k.t.MethodByName(k.name); !ok {
			t.Errorf("%v has no method %s", k.t, k.name)
		}
	}
}

// TestHostImport checks that a host that imports package vm keeps its
// http.DefaultServeMux and its flag.CommandLine as they are: the tables of
// expvar, net/http/pprof and testing/quick are in stdsyms/sideeffect.
func TestHostImport(t *testing.T) {
	out, err := exec.Command("go", "run", "./testdata/hostimport").CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	if len(out) > 0 {
		t.Errorf("importing vm changed the global state of the host:\n%s", out)
	}
}

// TestRefusedFlightRecorder checks that a restricted plugin cannot start a
// zero trace.FlightRecorder when it does not name Start (flightplug).
func TestRefusedFlightRecorder(t *testing.T) {
	for _, opts := range []Options{{}, {AOT: true}} {
		opts.Dir = ".."
		p, err := Load(opts, "github.com/traefik/yaegi/v2/vm/testdata/restricted/flightplug")
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := createConfig(p)
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "flight")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/interface", "/reflect"} {
			msg := func() (msg string) {
				defer func() {
					if r := recover(); r != nil {
						msg = fmt.Sprint(r)
					}
				}()
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
				return rec.Body.String()
			}()
			if !strings.Contains(msg, "(*runtime/trace.FlightRecorder).Start is not allowed in a plugin") {
				t.Errorf("aot %v, %s: got %q, want a refusal of (*runtime/trace.FlightRecorder).Start", opts.AOT, path, msg)
			}
		}
	}
}
