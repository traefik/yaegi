package main

// The scenarios of the catalog harness other than the default "middleware"
// one (driver.go): "auth" (requests with real credentials, read from a file),
// "reload" (CreateConfig, decoding and New repeated with configurations that
// change, then requests) and "provider" (a provider plugin run as Traefik runs
// it). Like driver.go, this file is shared by the three hosts: yhost has a
// symlink to it and the runner copies it next to the native host, so it must
// only import the standard library.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	scenarioName   = flag.String("scenario", "middleware", "scenario: middleware, auth, reload, provider (this file), headers, stream or conc (driver_extra.go)")
	requestsFile   = flag.String("requests", "", "auth: JSON file with the requests (authRequest)")
	reloadCount    = flag.Int("reloads", 200, "reload: number of CreateConfig + decode + New rounds")
	reloadWarm     = flag.Int("reloadwarm", 20, "reload: round after which goroutines and heap are measured first")
	reloadDeadline = flag.Duration("reloaddeadline", 120*time.Second, "reload: stop the rounds after this time")
	reloadConfigs  = flag.String("reloadconfigs", "", "reload: JSON file with the list of configurations used in turn")
	reloadCtx      = flag.String("reloadctx", "shared", "reload: context of New: \"shared\" (one context for all the rounds) or \"cancel\" (one per round, canceled before the next round, as Traefik does)")
	provideTime    = flag.Duration("providetime", 15*time.Second, "provider: how long to wait for the messages")
	provideCount   = flag.Int("providemsgs", 3, "provider: number of messages to wait for")
)

// providerPlugin is the interface of a provider plugin in Traefik
// (pkg/plugins/providers.go, PP).
type providerPlugin interface {
	Init() error
	Provide(cfgChan chan<- json.Marshaler) error
	Stop() error
}

// providerEngine is an engine that can build provider plugins.
type providerEngine interface {
	// newProvider calls New(ctx, cfg, name) of a provider plugin, as the
	// wrapper of Traefik does.
	newProvider(ctx context.Context, cfg any, name string) (providerPlugin, error)
}

// hostArgs are the flags of runMain that the scenarios need.
type hostArgs struct {
	dir, imp, basePkg, testData, out, times string
	useUnsafe                               bool
	timeout                                 time.Duration
}

// runScenario runs the scenario named by -scenario, which is not
// "middleware", and writes the transcript and the times as runMain does.
func runScenario(e engine, a hostArgs) {
	var t, tm strings.Builder
	defer func() {
		w := os.Stdout
		if a.out != "" {
			f, err := os.Create(a.out)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			defer f.Close()
			w = f
		}
		io.WriteString(w, t.String())
		if a.times != "" {
			os.WriteFile(a.times, []byte(tm.String()), 0o644)
		}
	}()
	data := map[string]any{}
	if a.testData != "" {
		if err := readJSON(a.testData, &data); err != nil {
			fmt.Fprintln(os.Stderr, "testdata:", err)
			os.Exit(1)
		}
	}
	t0 := time.Now()
	err := protect(func() error { return e.load(a.dir, a.imp, a.basePkg, a.useUnsafe) })
	fmt.Fprintf(&tm, "load_ms\t%d\n", time.Since(t0).Milliseconds())
	if err != nil {
		fmt.Fprintln(&t, "load: error")
		fmt.Fprintln(os.Stderr, "load:", err)
		return
	}
	fmt.Fprintln(&t, "load: ok")
	switch *scenarioName {
	case "auth":
		runAuth(e, a, data, &t, &tm)
	case "reload":
		runReload(e, a, data, &t, &tm)
	case "provider":
		runProvider(e, data, &t, &tm)
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario %q\n", *scenarioName)
		os.Exit(2)
	}
}

func readJSON(file string, v any) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// build calls CreateConfig, decodes data and calls New, and writes the stages
// to t as runMain does. It returns nil when a stage fails.
func build(ctx context.Context, e engine, data map[string]any, nx http.Handler, t *strings.Builder) http.Handler {
	var cfg any
	if err := protect(func() (err error) { cfg, err = e.createConfig(data); return }); err != nil {
		fmt.Fprintln(t, "config: error")
		fmt.Fprintln(os.Stderr, "config:", err)
		return nil
	}
	fmt.Fprintln(t, "config: ok")
	var h http.Handler
	err := protect(func() (err error) { h, err = e.newHandler(ctx, nx, cfg, "catalog-test"); return })
	if err != nil {
		fmt.Fprintf(t, "new: error %q\n", err.Error())
		fmt.Fprintln(os.Stderr, "new:", err)
		return nil
	}
	if h == nil {
		fmt.Fprintln(t, "new: nil handler")
		return nil
	}
	fmt.Fprintln(t, "new: ok")
	return h
}

// ---- auth ----

// authRequest is one request of the auth scenario. Target and header values
// may hold placeholders that the driver fills from the last redirection seen
// (an OIDC code flow): {{state}} and {{redirect_uri}} are the query
// parameters of the last Location header, {{code}} is an authorization code
// that the local OIDC provider of the runner turns back into the nonce of
// that Location ("c." + base64url(nonce)), and {{callback}} is redirect_uri
// with the code and the state added.
type authRequest struct {
	Name    string      `json:"name"`
	Method  string      `json:"method"`
	Target  string      `json:"target"`
	Remote  string      `json:"remote"`
	Body    string      `json:"body"`
	Header  [][2]string `json:"header"`
	Cookies bool        `json:"cookies"` // send the cookies set by the previous responses
}

func runAuth(e engine, a hostArgs, data map[string]any, t, tm *strings.Builder) {
	var reqs []authRequest
	if err := readJSON(*requestsFile, &reqs); err != nil {
		fmt.Fprintln(os.Stderr, "requests:", err)
		os.Exit(1)
	}
	seen := &seen{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := build(ctx, e, data, next{seen}, t)
	if h == nil {
		return
	}
	jar := map[string]string{}
	var loc *url.URL
	t1 := time.Now()
	for i, ar := range reqs {
		rq := request{method: ar.Method, target: fill(ar.Target, loc), remote: ar.Remote, body: ar.Body}
		if rq.method == "" {
			rq.method = "GET"
		}
		if rq.remote == "" {
			rq.remote = "203.0.113.7:41000"
		}
		for _, kv := range ar.Header {
			rq.header = append(rq.header, [2]string{kv[0], fill(kv[1], loc)})
		}
		if ar.Cookies && len(jar) > 0 {
			names := make([]string, 0, len(jar))
			for n := range jar {
				names = append(names, n)
			}
			sort.Strings(names)
			var cs []string
			for _, n := range names {
				v := jar[n]
				if strings.ContainsAny(v, " ,") {
					v = `"` + v + `"` // as net/http writes such values
				}
				cs = append(cs, n+"="+v)
			}
			rq.header = append(rq.header, [2]string{"Cookie", strings.Join(cs, "; ")})
		}
		seen.start(i + 1)
		fmt.Fprintf(t, "req %d %s: %s %s\n", i+1, ar.Name, rq.method, ar.Target)
		res, w, ok := serveAuth(h, rq, i+1, a.timeout)
		if !ok {
			fmt.Fprintln(t, "  timeout")
			fmt.Fprintf(tm, "timeout\treq %d\n", i+1)
			continue
		}
		t.WriteString(res)
		if rec := seen.String(); rec == "" {
			fmt.Fprintln(t, "  next: not called")
		} else {
			fmt.Fprintf(t, "  next: %s", rec)
		}
		if w == nil {
			continue
		}
		for _, c := range w.Result().Cookies() {
			if c.MaxAge < 0 || c.Value == "" {
				delete(jar, c.Name)
			} else {
				jar[c.Name] = c.Value
			}
		}
		if l := w.Header().Get("Location"); l != "" {
			if u, err := url.Parse(l); err == nil {
				loc = u
			}
		}
	}
	fmt.Fprintf(tm, "requests_us\t%d\n", time.Since(t1).Microseconds())
}

// fill replaces the flow placeholders of s with the values of the last
// redirection.
func fill(s string, loc *url.URL) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	var q url.Values
	if loc != nil {
		q = loc.Query()
	}
	code := "c." + base64.RawURLEncoding.EncodeToString([]byte(q.Get("nonce")))
	cb := q.Get("redirect_uri")
	if cb != "" {
		sep := "?"
		if strings.Contains(cb, "?") {
			sep = "&"
		}
		cb += sep + "code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(q.Get("state"))
	}
	return strings.NewReplacer(
		"{{state}}", url.QueryEscape(q.Get("state")),
		"{{redirect_uri}}", q.Get("redirect_uri"),
		"{{code}}", code,
		"{{callback}}", cb,
	).Replace(s)
}

// serveAuth is serve (driver.go) that also returns the recorder, to read
// cookies and redirections; w is nil after a panic.
func serveAuth(h http.Handler, rq request, num int, timeout time.Duration) (string, *httptest.ResponseRecorder, bool) {
	type result struct {
		s string
		w *httptest.ResponseRecorder
	}
	done := make(chan result, 1)
	r, cancel := newRequest(rq)
	r = r.WithContext(context.WithValue(r.Context(), reqNum{}, num))
	go func() {
		defer cancel()
		w := httptest.NewRecorder()
		defer func() {
			if p := recover(); p != nil {
				fmt.Fprintln(os.Stderr, "panic:", p)
				done <- result{"  panic\n", nil}
			}
		}()
		h.ServeHTTP(w, r)
		var b strings.Builder
		res := w.Result()
		fmt.Fprintf(&b, "  status %d\n", res.StatusCode)
		writeHeader(&b, "  header ", res.Header)
		body, _ := io.ReadAll(res.Body)
		if len(body) > 4096 {
			body = append(body[:4096:4096], "..."...)
		}
		fmt.Fprintf(&b, "  body %q\n", body)
		done <- result{b.String(), w}
	}()
	select {
	case res := <-done:
		return res.s, res.w, true
	case <-time.After(timeout):
		return "", nil, false
	}
}

// ---- reload ----

// runReload builds the middleware -reloads times, with the configurations of
// -reloadconfigs in turn (the first one is the manifest testData), as Traefik
// does at each change of the dynamic configuration. The transcript records the
// outcome of each configuration (once, and again if it changes), then the
// requests of the middleware scenario through a last handler built with the
// first configuration. Goroutines and heap after the warm-up rounds and at the
// end go to the times file: the runner compares their growth with native.
//
// With -reloadctx cancel, each round has its own context, and the context of
// round N is canceled before round N+1 calls New, as Traefik does: at each
// new dynamic configuration, RouterFactory.CreateRouters (pkg/server)
// cancels the context of the previous one first, then builds the new
// handlers with a new context, and the old handlers serve requests until the
// new ones replace them. So after the cancel, one request (the first of the
// middleware scenario) goes through the handler of round N, whose context is
// done; its status goes to the transcript (once per configuration, and again
// if it changes). The goroutines that a plugin starts in New and stops with
// its context are then gone at each round.
func runReload(e engine, a hostArgs, data map[string]any, t, tm *strings.Builder) {
	configs := []map[string]any{data}
	if *reloadConfigs != "" {
		configs = nil
		if err := readJSON(*reloadConfigs, &configs); err != nil {
			fmt.Fprintln(os.Stderr, "reloadconfigs:", err)
			os.Exit(1)
		}
	}
	seen := &seen{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	perRound := *reloadCtx == "cancel"
	// prev is the last round: its handler, its configuration and the
	// cancel of its context.
	var prev struct {
		h      http.Handler
		k      int
		cancel context.CancelFunc
	}
	oldStatus := map[int]string{}
	cancelPrev := func() {
		if prev.cancel != nil {
			prev.cancel()
			prev.cancel = nil
		}
	}
	defer cancelPrev()
	first := map[int]string{}
	t0 := time.Now()
	n := 0
	for ; n < *reloadCount && time.Since(t0) < *reloadDeadline; n++ {
		k := n % len(configs)
		rctx := ctx
		if perRound {
			cancelPrev()
			if prev.h != nil {
				st := oldHandlerStatus(prev.h, a.timeout)
				switch old, ok := oldStatus[prev.k]; {
				case !ok:
					fmt.Fprintf(t, "config %d, old handler after cancel: %s\n", prev.k, st)
				case old != st:
					fmt.Fprintf(t, "config %d, old handler after cancel changes at round %d: %s\n", prev.k, n, st)
				}
				oldStatus[prev.k] = st
			}
			rctx, prev.cancel = context.WithCancel(context.Background())
		}
		var b strings.Builder
		h := build(rctx, e, configs[k], next{seen}, &b)
		prev.h, prev.k = h, k
		// "config: ok\nnew: error ..." becomes "config ok, new error ...":
		// the runner reads "new: error" in a transcript as a failed New.
		out := strings.ReplaceAll(strings.TrimSpace(b.String()), "\n", ", ")
		out = strings.ReplaceAll(out, ": ", " ")
		switch prev, ok := first[k]; {
		case !ok:
			first[k] = out
			fmt.Fprintf(t, "config %d: %s\n", k, out)
		case prev != out:
			first[k] = out
			fmt.Fprintf(t, "config %d changes at round %d: %s\n", k, n, out)
		}
		if n+1 == *reloadWarm {
			g, h := measureHost()
			fmt.Fprintf(tm, "warm_goroutines\t%d\nwarm_heap\t%d\n", g, h)
		}
	}
	if perRound {
		// The last context is canceled too: what is left are the
		// goroutines that do not end with their context.
		cancelPrev()
		prev.h = nil
	}
	g, h := measureHost()
	fmt.Fprintf(tm, "rounds\t%d\nreload_ms\t%d\nend_goroutines\t%d\nend_heap\t%d\n", n, time.Since(t0).Milliseconds(), g, h)
	if n < *reloadCount {
		// The number of rounds depends on the speed of the engine: it is
		// only in the times file.
		fmt.Fprintf(tm, "deadline\t%d rounds of %d\n", n, *reloadCount)
	}
	fmt.Fprintln(t, "final:")
	hf := build(ctx, e, configs[0], next{seen}, t)
	if hf == nil {
		return
	}
	for i, rq := range scenario {
		seen.start(i + 1)
		fmt.Fprintf(t, "req %d %s %s\n", i+1, rq.method, rq.target)
		res, ok := serve(hf, rq, i+1, a.timeout)
		if !ok {
			fmt.Fprintln(t, "  timeout")
			continue
		}
		t.WriteString(res)
		if rec := seen.String(); rec == "" {
			fmt.Fprintln(t, "  next: not called")
		} else {
			fmt.Fprintf(t, "  next: %s", rec)
		}
	}
}

// oldHandlerStatus sends the first request of the middleware scenario to h,
// a handler whose context is canceled, and returns its status ("status
// 200"), "panic" or "timeout".
func oldHandlerStatus(h http.Handler, timeout time.Duration) string {
	// No min builtin: the native host is built with the go version of the
	// plugin module, which can be older than Go 1.21.
	if timeout > 2*time.Second {
		timeout = 2 * time.Second
	}
	res, ok := serve(h, scenario[0], -1, timeout)
	if !ok {
		return "timeout"
	}
	line, _, _ := strings.Cut(strings.TrimSpace(res), "\n")
	return line
}

// measureHost returns the number of goroutines and the live heap after a
// garbage collection.
func measureHost() (int, uint64) {
	var ms runtime.MemStats
	for i := 0; i < 2; i++ {
		runtime.GC()
	}
	time.Sleep(50 * time.Millisecond)
	runtime.ReadMemStats(&ms)
	return runtime.NumGoroutine(), ms.HeapAlloc
}

// ---- provider ----

// runProvider runs a provider plugin as Traefik does (pkg/plugins/providers.go):
// CreateConfig, decoding, New, Init, then Provide with a channel of
// json.Marshaler read by another goroutine, which calls MarshalJSON. The first
// -providemsgs messages go to the transcript as canonical JSON (keys sorted),
// then Stop is called. Traefik decodes the messages into its
// dynamic.Configuration; the harness has no such type, so it compares the
// whole JSON instead, which is stricter.
func runProvider(e engine, data map[string]any, t, tm *strings.Builder) {
	pe, ok := e.(providerEngine)
	if !ok {
		fmt.Fprintln(t, "new: engine without providers")
		return
	}
	var cfg any
	if err := protect(func() (err error) { cfg, err = e.createConfig(data); return }); err != nil {
		fmt.Fprintln(t, "config: error")
		fmt.Fprintln(os.Stderr, "config:", err)
		return
	}
	fmt.Fprintln(t, "config: ok")
	g0, _ := measureHost()
	var p providerPlugin
	err := protect(func() (err error) { p, err = pe.newProvider(context.Background(), cfg, "plugin-catalog-test"); return })
	if err != nil {
		fmt.Fprintf(t, "new: error %q\n", err.Error())
		fmt.Fprintln(os.Stderr, "new:", err)
		return
	}
	if p == nil {
		fmt.Fprintln(t, "new: nil provider")
		return
	}
	fmt.Fprintln(t, "new: ok")
	if err := protect(p.Init); err != nil {
		fmt.Fprintf(t, "init: error %q\n", err.Error())
		fmt.Fprintln(os.Stderr, "init:", err)
		return
	}
	fmt.Fprintln(t, "init: ok")

	cfgChan := make(chan json.Marshaler)
	var mu sync.Mutex
	var msgs []string
	got := make(chan struct{}, 1)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case m := <-cfgChan:
				s := marshalMessage(m)
				mu.Lock()
				msgs = append(msgs, s)
				mu.Unlock()
				select {
				case got <- struct{}{}:
				default:
				}
			}
		}
	}()
	t0 := time.Now()
	provided := make(chan error, 1)
	go func() { provided <- protect(func() error { return p.Provide(cfgChan) }) }()
	select {
	case err := <-provided:
		if err != nil {
			fmt.Fprintf(t, "provide: error %q\n", err.Error())
			fmt.Fprintln(os.Stderr, "provide:", err)
		} else {
			fmt.Fprintln(t, "provide: ok")
		}
	case <-time.After(*provideTime):
		// Traefik calls Provide on its own goroutine; one that never
		// returns is a plugin bug, but it is not fatal.
		fmt.Fprintln(t, "provide: does not return")
	}
	deadline := time.After(*provideTime - time.Since(t0))
wait:
	for {
		mu.Lock()
		n := len(msgs)
		mu.Unlock()
		if n >= *provideCount {
			break
		}
		select {
		case <-got:
		case <-deadline:
			break wait
		}
	}
	fmt.Fprintf(tm, "provide_ms\t%d\n", time.Since(t0).Milliseconds())
	mu.Lock()
	all := msgs
	mu.Unlock()
	if len(all) > *provideCount {
		all = all[:*provideCount]
	}
	for i, s := range all {
		fmt.Fprintf(t, "message %d: %s\n", i+1, s)
	}
	if len(all) < *provideCount {
		fmt.Fprintf(t, "messages: %d of %d\n", len(all), *provideCount)
	}
	if err := protect(p.Stop); err != nil {
		fmt.Fprintf(t, "stop: error %q\n", err.Error())
	} else {
		fmt.Fprintln(t, "stop: ok")
	}
	close(stop)
	time.Sleep(time.Second)
	g1, _ := measureHost()
	fmt.Fprintf(tm, "goroutines_before\t%d\ngoroutines_after_stop\t%d\n", g0, g1)
}

// marshalMessage calls MarshalJSON as Traefik does and returns the JSON with
// sorted keys, or the error.
func marshalMessage(m json.Marshaler) string {
	var b []byte
	err := protect(func() (err error) { b, err = m.MarshalJSON(); return })
	if err != nil {
		return fmt.Sprintf("marshal error %q", err.Error())
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Sprintf("invalid JSON %q", b)
	}
	c, _ := json.Marshal(v)
	return string(c)
}
