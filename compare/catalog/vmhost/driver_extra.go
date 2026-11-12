package main

// The extra scenarios of the catalog harness (compare/catalog/README.md,
// "Extra scenarios"). Like driver.go, this file is shared by the three hosts:
// yhost/driver_extra.go is a symlink to it and the runner copies it next to
// the native host (all the vmhost/driver*.go files). It must only import the
// standard library.
//
// The base scenario of driver.go sends seven sequential requests into an
// httptest.ResponseRecorder. That hides what a real proxy does with a plugin:
// - headers: several values per header, read and rewritten in place, on the
//   request (echoed back by next) and on the response (set by next);
// - stream: a real HTTP server, a next handler that flushes (SSE style) and a
//   next handler that hijacks the connection (WebSocket style upgrade);
// - conc: many concurrent requests through one handler (the hosts are built
//   with -race for this scenario);
// - soak: several plugins in one process under traffic for a long time, with
//   the memory and goroutine counts sampled over time.

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// -scenario is defined in driver_scenarios.go.
	concShape   = flag.String("conc", "32x50", "conc scenario: goroutines x requests per goroutine")
	soakList    = flag.String("soak", "", "soak: TSV file of plugins (dir, import, basepkg, testdata file, unsafe), all loaded in this process")
	soakDur     = flag.Duration("soakdur", 10*time.Minute, "soak: duration of the traffic")
	soakSample  = flag.Duration("soaksample", 0, "soak: sampling period (default soakdur/20)")
	soakWorkers = flag.Int("soakworkers", 8, "soak: goroutines sending requests")
)

// newEngine makes a fresh engine; the hosts that can load several plugins in
// one process (vmhost, yhost) set it, for the soak.
var newEngine func() engine

// extra is an extra scenario selected with -scenario.
type extra struct {
	// next makes the upstream handler given to New.
	next func(*seen) http.Handler
	// reqs replaces the requests of the base loop of runMain, when run is nil.
	reqs []request
	// run replaces the base loop of runMain.
	run func(h http.Handler, t, tm *strings.Builder, timeout time.Duration)
}

// loopScenario tells whether the -scenario name runs the request loop of
// runMain: "middleware" (the base scenario, ex is nil) and the extra
// scenarios of this file. The other names (auth, reload, provider, and the
// unknown ones, which runScenario rejects) go to driver_scenarios.go.
func loopScenario(name string) (ex *extra, loop bool) {
	switch name {
	case "middleware":
		return nil, true
	case "headers":
		return &extra{next: func(s *seen) http.Handler { return echoNext{s} }, reqs: headerScenario}, true
	case "stream":
		return &extra{next: func(*seen) http.Handler { return streamNext{} }, run: runStream}, true
	case "conc":
		return &extra{next: func(*seen) http.Handler { return ctxNext{} }, run: runConc}, true
	}
	return nil, false
}

// upstreamBody answers like next of driver.go: a body that depends on the
// path (HTML, JSON or text).
func upstreamBody(w http.ResponseWriter, r *http.Request) {
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

// --- headers -------------------------------------------------------------

// headerScenario sends several values for the headers that plugins read or
// rewrite (forwarding headers, cookies, accept lists, credentials).
var headerScenario = []request{
	{method: "GET", target: "http://example.com/", remote: "203.0.113.7:41000",
		header: [][2]string{{"X-Forwarded-For", "198.51.100.4"}, {"X-Forwarded-For", "10.0.0.2, 10.0.0.3"},
			{"X-Real-Ip", "198.51.100.4"}, {"X-Real-Ip", "10.0.0.2"},
			{"Accept", "text/html"}, {"Accept", "application/json"},
			{"Cookie", "session=abc123"}, {"Cookie", "theme=dark"},
			{"X-Multi", "one"}, {"X-Multi", "two"}, {"X-Multi", "three"},
			{"User-Agent", "catalog-test/1.0"}}},
	{method: "GET", target: "http://example.com/index.html?lang=en&lang=fr", remote: "203.0.113.7:41001",
		header: [][2]string{{"Accept-Encoding", "identity"}, {"Accept-Language", "en"}, {"Accept-Language", "fr;q=0.8"},
			{"Cf-Connecting-Ip", "192.0.2.10"}, {"Cf-Connecting-Ip", "192.0.2.11"},
			{"X-Forwarded-Proto", "https"}, {"X-Forwarded-Proto", "http"}, {"Forwarded", "for=198.51.100.4;proto=https"}}},
	{method: "POST", target: "http://example.com/api/data", remote: "198.51.100.9:42000", body: `{"name":"test","value":42}`,
		header: [][2]string{{"Content-Type", "application/json"}, {"Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig"},
			{"Authorization", "Basic dXNlcjpwYXNz"}, {"X-Api-Key", "secret-key"}, {"X-Api-Key", "other-key"}}},
	{method: "GET", target: "http://other.example.com/admin", remote: "10.0.0.1:5555",
		header: [][2]string{{"X-Forwarded-Host", "a.example.com"}, {"X-Forwarded-Host", "b.example.com"},
			{"X-Forwarded-For", "203.0.113.50"}, {"Via", "1.1 proxy-a"}, {"Via", "1.1 proxy-b"}, {"X-Request-Id", "r1"}}},
	{method: "GET", target: "http://example.com/", remote: "203.0.113.7:41000",
		header: [][2]string{{"X-Forwarded-For", "198.51.100.4"}, {"X-Forwarded-For", "10.0.0.2, 10.0.0.3"},
			{"X-Multi", "one"}, {"X-Multi", "two"}, {"User-Agent", "catalog-test/1.0"}}},
}

// echoNext records the request as next of driver.go does, sets response
// headers with several values, and echoes the request headers in the body:
// a plugin that rewrites request or response headers in place (r.Header[k][i]
// = v, or through w.Header()) shows it in the transcript.
type echoNext struct{ seen *seen }

func (n echoNext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	n.seen.mu.Lock()
	if num, ok := r.Context().Value(reqNum{}).(int); !ok || num == n.seen.cur {
		fmt.Fprintf(&n.seen.b, "%s %s host=%s remote=%s body=%q\n", r.Method, r.URL.RequestURI(), r.Host, r.RemoteAddr, body)
		writeHeader(&n.seen.b, "  ", r.Header)
	}
	n.seen.mu.Unlock()
	h := w.Header()
	h.Add("Set-Cookie", "up1=a; Path=/")
	h.Add("Set-Cookie", "up2=b; Path=/; HttpOnly")
	h.Add("Vary", "Accept")
	h.Add("Vary", "Accept-Encoding")
	h.Add("X-Upstream-Multi", "u1")
	h.Add("X-Upstream-Multi", "u2")
	h.Set("Cache-Control", "public, max-age=60")
	h.Set("Server", "upstream/1.0")
	h.Set("X-Powered-By", "catalog")
	var b strings.Builder
	writeHeader(&b, "", r.Header)
	switch {
	case strings.HasSuffix(r.URL.Path, ".html"):
		h.Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><head><title>t</title></head><body><pre>%s</pre><p>hello</p></body></html>\n", b.String())
	default:
		h.Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, b.String())
	}
}

// --- stream --------------------------------------------------------------

// streamRec is what streamNext saw during the current stream request; the
// stream requests are sequential.
var streamRec struct {
	mu    sync.Mutex
	b     strings.Builder
	early chan struct{} // closed by the client when it has read the first chunk
}

func streamNote(format string, args ...any) {
	streamRec.mu.Lock()
	defer streamRec.mu.Unlock()
	fmt.Fprintf(&streamRec.b, format, args...)
}

// streamNext records its notes as "  next <fact>" lines, without a colon, so
// that the coarse comparison of the runner (used when the native output is
// not deterministic) keeps them.
//
// streamNext serves /events (SSE: write, Flush, wait for the client to see the
// first chunk, write), /ws (WebSocket style upgrade through Hijack, then an
// echo of one line), and the body of next of driver.go for other paths.
type streamNext struct{}

func (streamNext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	switch r.URL.Path {
	case "/events":
		_, isFlusher := w.(http.Flusher)
		streamNote("  next called flusher=%v\n", isFlusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Upstream", "next")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "data: first\n\n")
		var err error
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		} else {
			err = http.NewResponseController(w).Flush()
		}
		streamNote("  next flush-error=%v\n", err != nil)
		streamRec.mu.Lock()
		early := streamRec.early
		streamRec.mu.Unlock()
		select {
		case <-early:
			streamNote("  next early-first-chunk=yes\n")
		case <-time.After(2 * time.Second):
			streamNote("  next early-first-chunk=no\n")
		}
		io.WriteString(w, "data: second\n\n")
	case "/ws":
		hj, isHijacker := w.(http.Hijacker)
		streamNote("  next called hijacker=%v\n", isHijacker)
		var conn net.Conn
		var brw *bufio.ReadWriter
		var err error
		if isHijacker {
			conn, brw, err = hj.Hijack()
		} else {
			conn, brw, err = http.NewResponseController(w).Hijack()
		}
		if err != nil {
			streamNote("  next hijack-error=true\n")
			w.Header().Set("X-Upstream", "next")
			http.Error(w, "cannot hijack", http.StatusNotImplemented)
			return
		}
		streamNote("  next hijack-error=false\n")
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
			wsAccept(r.Header.Get("Sec-WebSocket-Key")))
		brw.Flush()
		line, err := brw.ReadString('\n')
		if err != nil {
			streamNote("  next read-error-after-upgrade\n")
			return
		}
		brw.WriteString("echo " + line)
		brw.Flush()
	default:
		streamNote("  next called\n")
		upstreamBody(w, r)
	}
}

func wsAccept(key string) string {
	h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h[:])
}

// runStream serves the plugin with a real HTTP server and sends an SSE
// request, a WebSocket style upgrade and a plain request.
func runStream(h http.Handler, t, tm *strings.Builder, timeout time.Duration) {
	srv := httptest.NewUnstartedServer(protectHandler(h))
	srv.Config.ErrorLog = nil
	srv.Start()
	defer srv.Close()
	t1 := time.Now()
	for _, step := range []struct {
		name string
		f    func(addr string, t *strings.Builder)
	}{{"sse GET /events", streamSSE}, {"ws GET /ws", streamWS}, {"plain GET /index.html", streamPlain}} {
		streamRec.mu.Lock()
		streamRec.b.Reset()
		streamRec.early = make(chan struct{})
		streamRec.mu.Unlock()
		fmt.Fprintf(t, "%s\n", step.name)
		var b strings.Builder
		done := make(chan struct{})
		go func() { defer close(done); step.f(srv.Listener.Addr().String(), &b) }()
		select {
		case <-done:
			t.WriteString(b.String())
		case <-time.After(timeout):
			fmt.Fprintln(t, "  timeout")
			fmt.Fprintf(tm, "timeout\t%s\n", step.name)
		}
		// The handler may still be running (a buffering plugin writes after
		// next returns): give it a moment before reading what next saw.
		time.Sleep(50 * time.Millisecond)
		streamRec.mu.Lock()
		if rec := streamRec.b.String(); rec == "" {
			fmt.Fprintln(t, "  next not called")
		} else {
			t.WriteString(rec)
		}
		streamRec.mu.Unlock()
	}
	fmt.Fprintf(tm, "requests_us\t%d\n", time.Since(t1).Microseconds())
}

// protectHandler reports a panic of the plugin in the response (as a
// connection closed without a response) instead of the server log.
func protectHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				fmt.Fprintln(os.Stderr, "panic:", p)
				streamNote("  panic\n")
				panic(http.ErrAbortHandler)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

func dialStream(addr string) (net.Conn, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err == nil {
		c.SetDeadline(time.Now().Add(8 * time.Second))
	}
	return c, err
}

func streamHeaders(t *strings.Builder, res *http.Response) {
	fmt.Fprintf(t, "  status %d\n", res.StatusCode)
	h := res.Header.Clone()
	h.Del("Date")
	writeHeader(t, "  header ", h)
	if len(res.TransferEncoding) > 0 {
		fmt.Fprintf(t, "  transfer-encoding %q\n", res.TransferEncoding)
	}
}

func streamSSE(addr string, t *strings.Builder) {
	c, err := dialStream(addr)
	if err != nil {
		fmt.Fprintln(t, "  dial error")
		return
	}
	defer c.Close()
	io.WriteString(c, "GET /events HTTP/1.1\r\nHost: example.com\r\nAccept: text/event-stream\r\nUser-Agent: catalog-test/1.0\r\nConnection: close\r\n\r\n")
	br := bufio.NewReader(c)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Fprintln(t, "  no response")
		return
	}
	defer res.Body.Close()
	streamHeaders(t, res)
	var body []byte
	buf := make([]byte, 512)
	signaled := false
	streamRec.mu.Lock()
	early := streamRec.early
	streamRec.mu.Unlock()
	for {
		n, err := res.Body.Read(buf)
		body = append(body, buf[:n]...)
		if !signaled && strings.Contains(string(body), "data: first\n\n") {
			signaled = true
			close(early)
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintln(t, "  body read error")
			}
			break
		}
	}
	if len(body) > 4096 {
		body = append(body[:4096:4096], "..."...)
	}
	fmt.Fprintf(t, "  body %q\n", body)
}

func streamWS(addr string, t *strings.Builder) {
	c, err := dialStream(addr)
	if err != nil {
		fmt.Fprintln(t, "  dial error")
		return
	}
	defer c.Close()
	io.WriteString(c, "GET /ws HTTP/1.1\r\nHost: example.com\r\nUser-Agent: catalog-test/1.0\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n"+
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	br := bufio.NewReader(c)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Fprintln(t, "  no response")
		return
	}
	streamHeaders(t, res)
	if res.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		fmt.Fprintf(t, "  body %q\n", body)
		return
	}
	io.WriteString(c, "ping\n")
	line, err := br.ReadString('\n')
	if err != nil {
		fmt.Fprintln(t, "  no echo after upgrade")
		return
	}
	fmt.Fprintf(t, "  echo %q\n", line)
}

func streamPlain(addr string, t *strings.Builder) {
	c, err := dialStream(addr)
	if err != nil {
		fmt.Fprintln(t, "  dial error")
		return
	}
	defer c.Close()
	io.WriteString(c, "GET /index.html HTTP/1.1\r\nHost: example.com\r\nAccept: text/html\r\nAccept-Encoding: identity\r\nUser-Agent: catalog-test/1.0\r\nConnection: close\r\n\r\n")
	res, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		fmt.Fprintln(t, "  no response")
		return
	}
	defer res.Body.Close()
	streamHeaders(t, res)
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		fmt.Fprintln(t, "  body read error")
	}
	fmt.Fprintf(t, "  body %q\n", body)
}

// --- conc ----------------------------------------------------------------

// recKey carries, in the request context, the buffer where ctxNext records
// what reached it: concurrent requests each get their own record.
type recKey struct{}

type recBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

// ctxNext is next of driver.go with a record per request (in the context).
type ctxNext struct{}

func (ctxNext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if rb, ok := r.Context().Value(recKey{}).(*recBuf); ok {
		rb.mu.Lock()
		fmt.Fprintf(&rb.b, "%s %s host=%s remote=%s body=%q\n", r.Method, r.URL.RequestURI(), r.Host, r.RemoteAddr, body)
		writeHeader(&rb.b, "  ", r.Header)
		rb.mu.Unlock()
	}
	upstreamBody(w, r)
}

// serveRec sends one request of the base scenario through h, with its own
// record for ctxNext, and formats it as the base loop does. ok is false on
// timeout (the handler goroutine is left behind).
func serveRec(h http.Handler, rq request, timeout time.Duration) (string, bool) {
	r, cancel := newRequest(rq)
	rb := &recBuf{}
	r = r.WithContext(context.WithValue(r.Context(), recKey{}, rb))
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
		rb.mu.Lock()
		if rb.b.Len() == 0 {
			b.WriteString("  next: not called\n")
		} else {
			fmt.Fprintf(&b, "  next: %s", rb.b.String())
		}
		rb.mu.Unlock()
		done <- b.String()
	}()
	select {
	case s := <-done:
		return s, true
	case <-time.After(timeout):
		return "", false
	}
}

// runConc sends the base requests from many goroutines at once through the
// one handler (a goroutine stops at its first timeout), and writes, for each request of the base scenario, the
// distinct responses with their counts.
func runConc(h http.Handler, t, tm *strings.Builder, timeout time.Duration) {
	var g, n int
	if _, err := fmt.Sscanf(*concShape, "%dx%d", &g, &n); err != nil || g <= 0 || n <= 0 {
		fmt.Fprintln(os.Stderr, "bad -conc", *concShape)
		os.Exit(2)
	}
	type key struct {
		req int
		out string
	}
	var mu sync.Mutex
	counts := map[key]int{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	t1 := time.Now()
	for i := 0; i < g; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				k := (i + j) % len(scenario)
				s, ok := serveRec(h, scenario[k], timeout)
				if !ok {
					s = "  timeout\n"
				}
				mu.Lock()
				counts[key{k + 1, s}]++
				mu.Unlock()
				if !ok {
					// A plugin that waits on something that never comes
					// (a host to wake up, say) would make the scenario last
					// n timeouts: this goroutine stops at its first one.
					return
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	fmt.Fprintf(tm, "requests_us\t%d\n", time.Since(t1).Microseconds())
	fmt.Fprintf(t, "conc %dx%d\n", g, n)
	keys := make([]key, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].req != keys[b].req {
			return keys[a].req < keys[b].req
		}
		return keys[a].out < keys[b].out
	})
	for _, k := range keys {
		rq := scenario[k.req-1]
		fmt.Fprintf(t, "req %d %s %s x %d\n%s", k.req, rq.method, rq.target, counts[k], k.out)
	}
}

// --- soak ----------------------------------------------------------------

// soakRequested runs the soak when -soak is set; it reports whether it did.
func soakRequested() bool {
	if *soakList == "" {
		return false
	}
	if newEngine == nil {
		fmt.Fprintln(os.Stderr, "soak: this host cannot load several plugins")
		os.Exit(2)
	}
	runSoak()
	return true
}

type soakPlugin struct {
	name string
	h    http.Handler
	// Counters, updated by the workers.
	reqs, panics, timeouts atomic.Int64
	mu                     sync.Mutex
	status                 map[int]int
}

// runSoak loads every plugin of the -soak list in this process, sends the
// base requests through them from -soakworkers goroutines for -soakdur, and
// samples the heap (after a GC), the RSS and the goroutines. It writes the
// per plugin counts to -out and the samples to -times.
func runSoak() {
	// -out, -times and -reqtimeout belong to runMain (unset in go test).
	flagValue := func(name string) string {
		if f := flag.Lookup(name); f != nil {
			return f.Value.String()
		}
		return ""
	}
	out, samplesFile := flagValue("out"), flagValue("times")
	timeout, err := time.ParseDuration(flagValue("reqtimeout"))
	if err != nil {
		timeout = 10 * time.Second
	}
	b, err := os.ReadFile(*soakList)
	if err != nil {
		fmt.Fprintln(os.Stderr, "soak:", err)
		os.Exit(1)
	}
	var t strings.Builder
	var samples strings.Builder
	samples.WriteString("phase\tseconds\theap_alloc_mb\theap_inuse_mb\tsys_mb\trss_mb\tgoroutines\trequests\n")
	ctx, cancel := context.WithCancel(context.Background())
	var ps []*soakPlugin
	var total atomic.Int64
	t0 := time.Now()
	sample := func(phase string) {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		mb := func(v uint64) string { return strconv.FormatFloat(float64(v)/(1<<20), 'f', 1, 64) }
		fmt.Fprintf(&samples, "%s\t%.0f\t%s\t%s\t%s\t%s\t%d\t%d\n", phase, time.Since(t0).Seconds(), mb(ms.HeapAlloc), mb(ms.HeapInuse),
			mb(ms.Sys), mb(rss()), runtime.NumGoroutine(), total.Load())
	}
	sample("start")
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		name, dir, imp, base, tdFile, unsafe := f[0], f[1], f[2], f[3], f[4], f[5] == "true"
		data := map[string]any{}
		if tb, err := os.ReadFile(tdFile); err == nil {
			json.Unmarshal(tb, &data)
		}
		e := newEngine()
		var h http.Handler
		err := protect(func() error { return e.load(dir, imp, base, unsafe) })
		stage := "load"
		if err == nil {
			var cfg any
			stage = "config"
			err = protect(func() (err error) { cfg, err = e.createConfig(data); return })
			if err == nil {
				stage = "new"
				err = protect(func() (err error) { h, err = e.newHandler(ctx, ctxNext{}, cfg, "soak-"+name); return })
				if err == nil && h == nil {
					err = fmt.Errorf("nil handler")
				}
			}
		}
		if err != nil {
			fmt.Fprintf(&t, "plugin %s: %s error\n", name, stage)
			fmt.Fprintf(os.Stderr, "soak %s: %s: %v\n", name, stage, err)
			continue
		}
		ps = append(ps, &soakPlugin{name: name, h: h, status: map[int]int{}})
	}
	sample("loaded")
	period := *soakSample
	if period <= 0 {
		period = *soakDur / 20
	}
	if period < time.Second {
		period = time.Second
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < *soakWorkers && len(ps) > 0; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := w; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				p := ps[i%len(ps)]
				rq := scenario[(i/len(ps))%len(scenario)]
				s, ok := serveRec(p.h, rq, timeout)
				p.reqs.Add(1)
				total.Add(1)
				switch {
				case !ok:
					p.timeouts.Add(1)
				case s == "  panic\n":
					p.panics.Add(1)
				default:
					var code int
					fmt.Sscanf(s, "  status %d", &code)
					p.mu.Lock()
					p.status[code]++
					p.mu.Unlock()
				}
			}
		}(w)
	}
	tick := time.NewTicker(period)
	end := time.After(*soakDur)
loop:
	for {
		select {
		case <-tick.C:
			sample("traffic")
		case <-end:
			break loop
		}
	}
	tick.Stop()
	close(stop)
	wg.Wait()
	sample("end")
	// Cancel the context given to New: plugins with background work should
	// stop it. What stays is what the process keeps after the traffic.
	cancel()
	time.Sleep(2 * time.Second)
	sample("after-cancel")
	fmt.Fprintf(&t, "soak %s, %d workers, %d plugins running\n", *soakDur, *soakWorkers, len(ps))
	for _, p := range ps {
		var codes []string
		p.mu.Lock()
		for c, n := range p.status {
			codes = append(codes, fmt.Sprintf("%d:%d", c, n))
		}
		p.mu.Unlock()
		sort.Strings(codes)
		fmt.Fprintf(&t, "plugin %s: requests %d panics %d timeouts %d status %s\n", p.name, p.reqs.Load(), p.panics.Load(), p.timeouts.Load(), strings.Join(codes, ","))
	}
	if out != "" {
		os.WriteFile(out, []byte(t.String()), 0o644)
	} else {
		io.WriteString(os.Stdout, t.String())
	}
	if samplesFile != "" {
		os.WriteFile(samplesFile, []byte(samples.String()), 0o644)
	}
}

// rss is the resident set size of the process (Linux only, else 0).
func rss() uint64 {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.ParseUint(f[1], 10, 64)
	return n * uint64(os.Getpagesize())
}
