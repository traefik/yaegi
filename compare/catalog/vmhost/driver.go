package main

// Shared by the three hosts of the catalog harness (vmhost, yhost and the
// generated native host, see compare/catalog/README.md): the same flags, the
// same Traefik-like construction of the middleware and the same requests, so
// that the transcripts of the hosts can be compared byte for byte.
//
// yhost/driver.go is a symlink to this file and the runner copies it next to
// the native host: it must only import the standard library and mapstructure.
// The same holds for the other driver*.go files: driver_scenarios.go (auth,
// reload, provider) and driver_extra.go (headers, stream, conc and -soak).

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
)

// engine is the plugin runtime under test.
type engine interface {
	// load loads the plugin package imp of the module in dir.
	load(dir, imp, basePkg string, useUnsafe bool) error
	// createConfig calls CreateConfig and decodes testData into its result
	// with traefikDecode, as Traefik does.
	createConfig(testData map[string]any) (any, error)
	// newHandler calls New.
	newHandler(ctx context.Context, next http.Handler, cfg any, name string) (http.Handler, error)
}

// traefikDecode decodes the dynamic configuration of a middleware into the
// result of CreateConfig with the options of Traefik
// (pkg/plugins/middlewareyaegi.go, createConfig).
func traefikDecode(result any, config map[string]any) error {
	if len(config) == 0 {
		return nil
	}
	d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToSliceHookFunc(","),
		WeaklyTypedInput: true,
		Result:           result,
	})
	if err != nil {
		return err
	}
	return d.Decode(config)
}

// request is one request of the scenario.
type request struct {
	method, target, remote, body string
	header                       [][2]string
}

// scenario covers what catalog plugins look at: paths and queries, client
// addresses and forwarding headers, credentials, JSON bodies, HTML responses
// (body rewriting), CORS preflight, another host, and a repeated request
// (caches, rate limits, ban counters).
var scenario = []request{
	{method: "GET", target: "http://example.com/", remote: "203.0.113.7:41000",
		header: [][2]string{{"User-Agent", "catalog-test/1.0"}, {"Accept", "*/*"}}},
	{method: "GET", target: "http://example.com/path/to/page?foo=bar&x=1", remote: "203.0.113.7:41001",
		header: [][2]string{{"User-Agent", "Mozilla/5.0 (X11; Linux x86_64)"}, {"X-Forwarded-For", "198.51.100.4, 10.0.0.2"},
			{"X-Real-Ip", "198.51.100.4"}, {"Cookie", "session=abc123; theme=dark"}, {"Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig"}}},
	{method: "POST", target: "http://example.com/api/data", remote: "198.51.100.9:42000",
		header: [][2]string{{"Content-Type", "application/json"}, {"X-Api-Key", "secret-key"}}, body: `{"name":"test","value":42}`},
	{method: "GET", target: "http://example.com/index.html", remote: "203.0.113.7:41002",
		header: [][2]string{{"Accept", "text/html"}, {"Accept-Encoding", "identity"}, {"Cf-Connecting-Ip", "192.0.2.10"}}},
	{method: "OPTIONS", target: "http://example.com/api/data", remote: "203.0.113.7:41003",
		header: [][2]string{{"Origin", "https://example.org"}, {"Access-Control-Request-Method", "POST"}}},
	{method: "GET", target: "http://other.example.com/admin?debug=${jndi:ldap://x}", remote: "10.0.0.1:5555",
		header: [][2]string{{"User-Agent", "curl/8.0"}, {"Authorization", "Basic dXNlcjpwYXNz"}}},
	{method: "GET", target: "http://example.com/", remote: "203.0.113.7:41000",
		header: [][2]string{{"User-Agent", "catalog-test/1.0"}, {"Accept", "*/*"}}},
}

// next is the upstream handler: it reports what reached it and answers with
// a body that depends on the path (HTML, JSON or text).
type next struct{ seen *seen }

// seen records what reached next during the current request. A handler
// still running after a timeout must not write into the next request's
// record: each request carries its number in its context, and only the
// current one (or a request whose context the plugin replaced) is recorded.
type seen struct {
	mu  sync.Mutex
	cur int
	b   strings.Builder
}

type reqNum struct{}

func (s *seen) start(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = n
	s.b.Reset()
}

func (s *seen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (n next) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	n.seen.mu.Lock()
	if num, ok := r.Context().Value(reqNum{}).(int); !ok || num == n.seen.cur {
		fmt.Fprintf(&n.seen.b, "%s %s host=%s remote=%s body=%q\n", r.Method, r.URL.RequestURI(), r.Host, r.RemoteAddr, body)
		writeHeader(&n.seen.b, "  ", r.Header)
	}
	n.seen.mu.Unlock()
	w.Header().Set("X-Upstream", "next")
	switch {
	case strings.HasSuffix(r.URL.Path, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, "<html><head><title>t</title></head><body><p>hello</p></body></html>\n")
	case strings.HasPrefix(r.URL.Path, "/api/"):
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"items":[1,2,3]}`+"\n")
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "upstream body\n")
	}
}

func writeHeader(b *strings.Builder, indent string, h http.Header) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "%s%s: %q\n", indent, k, h[k])
	}
}

func runMain(e engine) {
	dir := flag.String("dir", ".", "module root of the plugin")
	imp := flag.String("import", "", "import path of the plugin package (manifest import)")
	basePkg := flag.String("basepkg", "", "package name (manifest basePkg)")
	testData := flag.String("testdata", "", "JSON file with the manifest testData")
	useUnsafe := flag.Bool("unsafe", false, "allow unsafe and syscall (manifest useUnsafe)")
	out := flag.String("out", "", "transcript file (default stdout)")
	times := flag.String("times", "", "file for the load and request times")
	timeout := flag.Duration("reqtimeout", 10*time.Second, "timeout of each request")
	flag.Parse()
	if soakRequested() {
		return
	}
	// The one dispatch of -scenario: middleware and the extra scenarios
	// (driver_extra.go) run the request loop below, the other ones
	// (driver_scenarios.go) have their own runner.
	ex, loop := loopScenario(*scenarioName)
	if !loop {
		runScenario(e, hostArgs{dir: *dir, imp: *imp, basePkg: *basePkg, testData: *testData, out: *out,
			times: *times, useUnsafe: *useUnsafe, timeout: *timeout})
		return
	}

	var t strings.Builder
	var tm strings.Builder
	defer func() {
		w := os.Stdout
		if *out != "" {
			f, err := os.Create(*out)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			defer f.Close()
			w = f
		}
		io.WriteString(w, t.String())
		if *times != "" {
			os.WriteFile(*times, []byte(tm.String()), 0o644)
		}
	}()

	data := map[string]any{}
	if *testData != "" {
		b, err := os.ReadFile(*testData)
		if err == nil {
			err = json.Unmarshal(b, &data)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "testdata:", err)
			os.Exit(1)
		}
	}

	t0 := time.Now()
	err := protect(func() error { return e.load(*dir, *imp, *basePkg, *useUnsafe) })
	fmt.Fprintf(&tm, "load_ms\t%d\n", time.Since(t0).Milliseconds())
	if err != nil {
		// Load errors are the engine's own: only the stage is compared.
		fmt.Fprintln(&t, "load: error")
		fmt.Fprintln(os.Stderr, "load:", err)
		return
	}
	fmt.Fprintln(&t, "load: ok")

	var cfg any
	if err := protect(func() (err error) { cfg, err = e.createConfig(data); return }); err != nil {
		fmt.Fprintln(&t, "config: error")
		fmt.Fprintln(os.Stderr, "config:", err)
		return
	}
	fmt.Fprintln(&t, "config: ok")

	seen := &seen{}
	var nx http.Handler = next{seen}
	reqs := scenario
	if ex != nil {
		nx = ex.next(seen)
		if ex.reqs != nil {
			reqs = ex.reqs
		}
	}
	var h http.Handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = protect(func() (err error) { h, err = e.newHandler(ctx, nx, cfg, "catalog-test"); return })
	if err != nil {
		// Errors returned by New are usually the plugin's own messages.
		fmt.Fprintf(&t, "new: error %q\n", err.Error())
		fmt.Fprintln(os.Stderr, "new:", err)
		return
	}
	if h == nil {
		fmt.Fprintln(&t, "new: nil handler")
		return
	}
	fmt.Fprintln(&t, "new: ok")
	if ex != nil && ex.run != nil {
		ex.run(h, &t, &tm, *timeout)
		return
	}

	t1 := time.Now()
	for i, rq := range reqs {
		seen.start(i + 1)
		fmt.Fprintf(&t, "req %d %s %s\n", i+1, rq.method, rq.target)
		res, ok := serve(h, rq, i+1, *timeout)
		if !ok {
			fmt.Fprintln(&t, "  timeout")
			fmt.Fprintf(&tm, "timeout\treq %d\n", i+1)
			continue
		}
		t.WriteString(res)
		if rec := seen.String(); rec == "" {
			fmt.Fprintln(&t, "  next: not called")
		} else {
			fmt.Fprintf(&t, "  next: %s", rec)
		}
	}
	fmt.Fprintf(&tm, "requests_us\t%d\n", time.Since(t1).Microseconds())
}

// newRequest builds the request rq as a server gives it to a handler: the
// caller calls cancel when the handler returns, so the context of the request
// is then done, as net/http does. Without it, a plugin that waits for the end
// of each request (darkweak/souin starts a goroutine on <-ctx.Done()) keeps
// one goroutine per request, natively too.
func newRequest(rq request) (*http.Request, context.CancelFunc) {
	r := httptest.NewRequest(rq.method, rq.target, strings.NewReader(rq.body))
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	r.RemoteAddr = rq.remote
	for _, kv := range rq.header {
		r.Header.Add(kv[0], kv[1])
	}
	return r, cancel
}

// serve sends one request and formats the response; ok is false on timeout.
// A panic of the handler is reported without its message (the wording of
// runtime errors differs between engines).
func serve(h http.Handler, rq request, num int, timeout time.Duration) (string, bool) {
	r, cancel := newRequest(rq)
	r = r.WithContext(context.WithValue(r.Context(), reqNum{}, num))
	w := httptest.NewRecorder()
	done := make(chan string, 1)
	go func() {
		defer cancel()
		defer func() {
			if p := recover(); p != nil {
				fmt.Fprintln(os.Stderr, "panic:", p)
				done <- "  panic\n"
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
		done <- b.String()
	}()
	select {
	case s := <-done:
		return s, true
	case <-time.After(timeout):
		return "", false
	}
}

// protect turns a panic of f into an error.
func protect(f func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return f()
}
