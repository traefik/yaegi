package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestNoNetProxy checks that a host of a scenario gets a stable refusal for
// an internet host and still reaches the local servers, and that the hosts of
// the middleware scenario keep the environment of the runner. The HTTP
// clients run in a child process (TestNoNetChild): http.ProxyFromEnvironment
// reads the environment once per process.
func TestNoNetProxy(t *testing.T) {
	if err := startNoNetProxy(); err != nil {
		t.Fatal(err)
	}
	defer func() { noNetProxy = "" }()
	if env := noNetEnv("native"); env != nil {
		t.Errorf("noNetEnv(native) = %v, want nil (middleware scenario)", env)
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "local ok")
	}))
	defer local.Close()
	// machine stands for the proxy of the measuring machine, whose answers
	// change: the hosts of the scenarios must not use it.
	machine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "machine proxy", http.StatusBadGateway)
	}))
	defer machine.Close()
	var base []string
	for _, kv := range os.Environ() {
		if !strings.Contains(strings.ToLower(strings.SplitN(kv, "=", 2)[0]), "proxy") {
			base = append(base, kv)
		}
	}
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		base = append(base, k+"="+machine.URL)
	}
	for _, mode := range []string{"reload-native", "auth-govm", "provider-aot"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestNoNetChild$", "-test.v")
		cmd.Env = append(append([]string{}, base...), noNetEnv(mode)...)
		cmd.Env = append(cmd.Env, "CATALOG_NONET_CHILD="+local.URL)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, out)
		}
		for _, want := range []string{
			`https: Get "https://cdn.jsdelivr.net/robots.json": Forbidden`,
			"http: status 403",
			"local: local ok",
		} {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s: output has no %q:\n%s", mode, want, out)
			}
		}
	}
}

// TestNoNetChild is the child process of TestNoNetProxy.
func TestNoNetChild(t *testing.T) {
	local := os.Getenv("CATALOG_NONET_CHILD")
	if local == "" {
		t.Skip("child process of TestNoNetProxy")
	}
	c := &http.Client{Timeout: 10 * time.Second}
	if _, err := c.Get("https://cdn.jsdelivr.net/robots.json"); err != nil {
		fmt.Println("https:", err)
	}
	if res, err := c.Get("http://cdn.jsdelivr.net/robots.json"); err == nil {
		fmt.Println("http: status", res.StatusCode)
		res.Body.Close()
	}
	if res, err := c.Get(local); err == nil {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		fmt.Println("local:", string(b))
	}
}

// TestStopLeak checks the comparison of the goroutines left after Stop in
// the provider scenario: only what an engine keeps beyond the maximum of the
// two native runs counts, and one goroutine more (a Provide goroutine not
// stopped) is a leak.
func TestStopLeak(t *testing.T) {
	r := &scenResult{times: map[string]map[string]string{
		"native":  {"goroutines_before": "1", "goroutines_after_stop": "9"},
		"native2": {"goroutines_before": "1", "goroutines_after_stop": "10"},
		"yaegi":   {"goroutines_before": "1", "goroutines_after_stop": "1"},
		"govm":    {"goroutines_before": "1", "goroutines_after_stop": "10"},
		"aot":     {"goroutines_before": "1", "goroutines_after_stop": "11"},
	}}
	for mode, want := range map[string]string{"yaegi": "", "govm": "", "aot": "goroutines"} {
		if got := stopLeak(r, mode); got != want {
			t.Errorf("stopLeak(%s) = %q, want %q", mode, got, want)
		}
	}
	// Without native2 times, native alone: govm keeps 1 more, a leak.
	r.times["native2"] = map[string]string{}
	if got := stopLeak(r, "govm"); got != "goroutines" {
		t.Errorf("stopLeak(govm) without native2 = %q, want \"goroutines\"", got)
	}
	r.times["govm"] = map[string]string{} // no times file: not compared
	if got := stopLeak(r, "govm"); got != "" {
		t.Errorf("stopLeak without times = %q, want \"\"", got)
	}
}

// TestNewPanicNorm checks that sNorm turns a panic of New into "new: panic"
// in every engine wording, and keeps the other yaegi v2 errors of New, which
// also start with "vm: ".
func TestNewPanicNorm(t *testing.T) {
	cases := []struct {
		err   string
		panic bool
	}{
		// Real outputs of the driver (native host, vmhost and vmhost -aot,
		// the same in both modes) for a plugin whose New panics.
		{"panic: boom", true},     // native and yaegi (protect of the driver)
		{"vm: panic: boom", true}, // yaegi v2, panic("boom")
		{"vm: panic: boom err", true},
		{"panic: runtime error: index out of range [3] with length 1", true},
		{"vm: runtime error: index out of range [3] with length 1", true},
		{"panic: runtime error: invalid memory address or nil pointer dereference", true},
		{"vm: runtime error: invalid memory address or nil pointer dereference", true},
		{"panic: assignment to entry in nil map", true},
		{"vm: assignment to entry in nil map", true},
		{"vm: runtime error: integer divide by zero", true},
		{"vm: runtime error: slice bounds out of range [:5] with capacity 1", true},
		{"vm: runtime error: makeslice: len out of range", true},
		{"panic: interface conversion: interface {} is string, not int", true},
		{"vm: interface conversion: interface {} is string, not int", true},
		{"panic: close of closed channel", true},
		{"vm: close of closed channel", true},
		// Panics raised by standard library functions (native code or AOT
		// copy): real outputs of vmhost, default and -aot, for a New that
		// calls them. Native reports "panic: <the same message>".
		{"vm: regexp: Compile(`(`): error parsing regexp: missing closing ): `(`", true},
		{"panic: regexp: Compile(`(`): error parsing regexp: missing closing ): `(`", true},
		{"vm: non-positive interval for NewTicker", true},
		{"vm: strings: negative Repeat count", true},
		{"vm: sync: negative WaitGroup counter", true},
		{"vm: bytes.Buffer: truncation out of range", true},
		{"vm: strings.NewReplacer: odd argument count", true},
		{"vm: value method example.com/plug.T.M called using nil *T pointer", true},
		// yaegi v2 bugs that are still panics (the wording differs from
		// native: tasks in TODO.md): close of a nil channel and
		// reflect.ValueOf(1).Elem().
		{"vm: interface conversion: interface {} is nil, not chan vm.Value", true},
		{"vm: panic: nil", true},
		// yaegi v2 failures, not panics of the plugin.
		{"vm: vm: execution budget exhausted", false},
		{"vm: vm: stack overflow (MaxDepth, MaxStack or MaxNest)", false},
		{"vm: runtime.Goexit called", false},
		{"vm: the data for the recursive type main.T is nested too deeply for yaegi v2 default mode: its native type would be too large", false},
		{"vm: vm: value of type [100000000]int is too large for the VM (more than 67108864 entries): gc refuses it or needs as much memory", false},
		{"vm: unsupported: some construct", false},
		// Real output of vmhost (both modes) for a restricted plugin that
		// calls os.Stdin.Chdir through an interface of its own.
		{"vm: (*os.File).Chdir is not allowed in a plugin", false},
		// Errors returned by New.
		{"siteKey is required", false},
		{"invalid handler type <nil>", false},
	}
	for _, c := range cases {
		// The transcript of New (scenarios auth and provider), and the line
		// of the reload scenario, where the driver removes every ": ".
		tr := fmt.Sprintf("load: ok\nconfig: ok\nnew: error %q\n", c.err)
		// As runReload (vmhost/driver_scenarios.go) writes it.
		out := strings.ReplaceAll(fmt.Sprintf("config: ok\nnew: error %q", c.err), "\n", ", ")
		rl := "config 0: " + strings.ReplaceAll(out, ": ", " ") + "\n"
		for _, in := range []string{tr, rl} {
			got := strings.Contains(sNorm(in), "new: panic")
			if got != c.panic {
				t.Errorf("sNorm(%q) = %q: panic %v, want %v", in, sNorm(in), got, c.panic)
			}
		}
	}
}
