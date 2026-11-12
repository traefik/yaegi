package vm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Tests of Plugin.Close (close.go).

// reloadPlugin starts, in New, the goroutines of a cache plugin (a janitor
// with a time.Ticker, a refresh loop with time.Sleep) that never end, and
// one that ends at once. Traefik calls New at each reload of its dynamic
// configuration.
const reloadPlugin = `package p

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type cache struct {
	mu sync.Mutex
	m  map[string]int
}

func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	c := &cache{m: map[string]int{}}
	go func() { // the janitor
		t := time.NewTicker(time.Millisecond)
		defer t.Stop()
		for range t.C {
			c.mu.Lock()
			clear(c.m)
			c.mu.Unlock()
		}
	}()
	go func() { // a refresh loop
		for {
			time.Sleep(time.Hour)
		}
	}()
	go func() {}() // ends at once: it must not count
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.m[r.URL.Path]++
		c.mu.Unlock()
		if d, err := time.ParseDuration(r.URL.Query().Get("sleep")); err == nil {
			time.Sleep(d)
		}
		if r.URL.Query().Get("next") != "" {
			next.ServeHTTP(w, r)
			return
		}
		w.Write([]byte("ok"))
	}), nil
}

// Watch starts a goroutine that a recover cannot keep alive, and that
// closes done in a deferred call when it stops.
func Watch(done chan struct{}) {
	go func() {
		defer close(done)
		for {
			func() {
				defer func() { recover() }()
				select {}
			}()
		}
	}()
}

// Block waits for ever: a call that runs when the plugin is closed.
func Block() {
	select {}
}

// Later runs a func of the plugin after d with afterFunc: time.AfterFunc
// of the host, which also tells the test that the callback ended.
func Later(d time.Duration, afterFunc func(time.Duration, func())) {
	afterFunc(d, func() {
		for i := 0; i < 1000; i++ {
		}
	})
}

// Fn returns a func value that the host calls: it waits until wait is
// closed (nil: for ever) and returns 1.
func Fn(wait chan struct{}) func() int {
	return func() int {
		if wait == nil {
			select {}
		}
		<-wait
		return 1
	}
}

// FnErr returns a func value that the host calls, with an error result. It
// waits for ever when block is true.
func FnErr(block bool) func() error {
	return func() error {
		if block {
			select {}
		}
		return nil
	}
}
`

func loadReloadPlugin(t *testing.T, opts Options) (*Plugin, *Object) {
	t.Helper()
	needSource(t)
	dir, pkg := writePlugin(t, reloadPlugin)
	opts.Dir = dir
	p, err := Load(opts, pkg)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	return p, cfg
}

// TestReloadGoroutineCap shows the failure that Close fixes: each New
// leaves 2 goroutines that never end, and they count in MaxGoroutines for
// the life of the machine, so New fails for ever after MaxGoroutines/2
// reloads (about 500 with the 1000 of pluginhost). A goroutine that ended
// does not count (it counts while it runs: hence the 1 of the cap).
func TestReloadGoroutineCap(t *testing.T) {
	const reloads = 10
	p, cfg := loadReloadPlugin(t, Options{Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 2*reloads + 1})
	defer p.Close()
	for i := 1; i <= reloads; i++ {
		if _, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x"); err != nil {
			t.Fatalf("New %d: %v", i, err)
		}
		if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 2*i }) {
			t.Fatalf("after New %d: %d goroutines, want %d (the goroutine that ended must not count)", i, p.M.Goroutines(), 2*i)
		}
	}
	_, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if !errors.Is(err, ErrTooManyGoroutines) {
		t.Fatalf("New past the cap: %v, want %v", err, ErrTooManyGoroutines)
	}
}

// TestPluginClose checks that Close stops all the goroutines of a plugin
// (also one that recovers, and a call that runs), that the deferred calls
// run, that the calls and the handlers fail cleanly after it, and that a
// time.AfterFunc callback that fires later stops without a panic for
// GoPanic. Before, a host had no way to release a plugin: its background
// goroutines ran for ever.
func TestPluginClose(t *testing.T) {
	var panics atomic.Int32
	opts := Options{Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 1000, GoPanic: func(r any) {
		panics.Add(1)
		t.Errorf("GoPanic: %v", r)
	}}
	p, cfg := loadReloadPlugin(t, opts)
	var hs []http.Handler
	for range 5 {
		h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
		if err != nil {
			t.Fatal(err)
		}
		hs = append(hs, h)
	}
	if body, r := serveBG(hs[0], "/a"); r != nil || body != "ok" {
		t.Fatalf("ServeHTTP before Close: %q, panic %v", body, r)
	}
	done := make(chan struct{})
	if _, err := p.Call(context.Background(), "Watch", done); err != nil {
		t.Fatal(err)
	}
	fired := make(chan struct{})
	afterFunc := func(d time.Duration, f func()) {
		time.AfterFunc(d, func() {
			f()
			close(fired)
		})
	}
	if _, err := p.Call(context.Background(), "Later", 100*time.Millisecond, afterFunc); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() {
		_, err := p.Call(context.Background(), "Block")
		blocked <- err
	}()
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 11 }) {
		t.Fatalf("before Close: %d goroutines, want 11", p.M.Goroutines())
	}

	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil { // idempotent
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("after Close: %d goroutines still run", p.M.Goroutines())
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the deferred call of the stopped goroutine did not run")
	}
	select {
	case err := <-blocked:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("the call that ran: %v, want %v", err, ErrClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call that ran did not stop")
	}
	if _, err := p.Call(context.Background(), "CreateConfig"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Call after Close: %v, want %v", err, ErrClosed)
	}
	rec := httptest.NewRecorder()
	hs[1].ServeHTTP(rec, httptest.NewRequest("GET", "/a", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ServeHTTP after Close: status %d, want 503", rec.Code)
	}
	select {
	case <-fired: // the callback after Close returned: no panic, no Goexit
	case <-time.After(2 * time.Second):
		t.Fatal("the time.AfterFunc callback did not end")
	}
	if n := panics.Load(); n != 0 {
		t.Fatalf("%d panics for GoPanic", n)
	}
}

// TestPluginCloseNoLimits checks Close on a machine without Budget,
// BackgroundRate and MaxGoroutines: the goroutines that wait stop too.
func TestPluginCloseNoLimits(t *testing.T) {
	p, cfg := loadReloadPlugin(t, Options{})
	for range 3 {
		if _, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 6 }) {
		t.Fatalf("before Close: %d goroutines, want 6", p.M.Goroutines())
	}
	p.Close()
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("after Close: %d goroutines still run", p.M.Goroutines())
	}
}

// TestReloadByLoad is the host pattern for a reload: load the plugin again
// (LoadCached reuses the program of the first load), build the new
// instance, and close the old Plugin. The goroutines never pile up. It logs
// the cost of each reload.
func TestReloadByLoad(t *testing.T) {
	opts := Options{Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 4}
	needSource(t)
	dir, pkg := writePlugin(t, reloadPlugin)
	opts.Dir = dir
	base, err := Load(opts, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	var old *Plugin
	var total time.Duration
	const reloads = 10
	for i := range reloads {
		t0 := time.Now()
		p, err := base.LoadCached(dir, opts)
		if err != nil {
			t.Fatal(err)
		}
		total += time.Since(t0)
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x"); err != nil {
			t.Fatalf("reload %d: %v", i, err)
		}
		if old != nil {
			old.Close()
			if !waitFor(2*time.Second, func() bool { return old.M.Goroutines() == 0 }) {
				t.Fatalf("reload %d: %d goroutines of the old plugin still run", i, old.M.Goroutines())
			}
		}
		old = p
	}
	old.Close()
	t.Logf("LoadCached: %v per reload", total/reloads)
}

// serveCode serves path with h and returns the status and the body, or the
// panic of ServeHTTP.
func serveCode(h http.Handler, path string) (code int, body string, r any) {
	rec := httptest.NewRecorder()
	defer func() { code, body, r = rec.Code, rec.Body.String(), recover() }()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return
}

// TestPluginCloseContextDrain checks the drain of CloseContext: a new
// request gets 503 at once, a request that runs ends normally (also when it
// calls another instance of the plugin through next: that call is not new),
// and the background goroutines stop after the drain.
func TestPluginCloseContextDrain(t *testing.T) {
	p, cfg := loadReloadPlugin(t, Options{Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 1000})
	// The request waits in the next of the second instance (a handler of
	// the host, called by the plugin) until the test releases it.
	release := make(chan struct{})
	reached := make(chan struct{})
	last := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(reached)
		<-release
		http.NotFound(w, r)
	})
	h2, err := newHandler(context.Background(), p, last, cfg, "second")
	if err != nil {
		t.Fatal(err)
	}
	h1, err := newHandler(context.Background(), p, h2, cfg, "first")
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		code int
		body string
		r    any
	}
	running := make(chan result, 1)
	go func() {
		code, body, r := serveCode(h1, "/a?next=1")
		running <- result{code, body, r}
	}()
	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("the request did not start")
	}
	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closed <- p.CloseContext(ctx)
	}()
	if !waitFor(2*time.Second, p.M.draining.Load) {
		t.Fatal("CloseContext did not start the drain")
	}
	if code, _, r := serveCode(h1, "/b"); code != http.StatusServiceUnavailable || r != nil {
		t.Fatalf("new request during the drain: status %d, panic %v, want 503", code, r)
	}
	if _, err := p.Call(context.Background(), "CreateConfig"); !errors.Is(err, ErrClosed) {
		t.Fatalf("new call during the drain: %v, want %v", err, ErrClosed)
	}
	if p.M.Closed() {
		t.Fatal("closed before the request that runs ended")
	}
	close(release)
	select {
	case res := <-running:
		// The second instance gets the request through next, and calls
		// its own next: the handler of the host.
		if res.r != nil || res.code != http.StatusNotFound {
			t.Fatalf("the request that ran: status %d, body %q, panic %v; want the 404 of the second next", res.code, res.body, res.r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the request that ran did not end")
	}
	if err := <-closed; err != nil {
		t.Fatalf("CloseContext: %v, want nil (every call ended)", err)
	}
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("after CloseContext: %d goroutines still run", p.M.Goroutines())
	}
}

// TestPluginCloseContextDeadline checks that a request that still runs at
// the end of the drain is stopped (net/http gets http.ErrAbortHandler), and
// that CloseContext reports the deadline.
func TestPluginCloseContextDeadline(t *testing.T) {
	p, cfg := loadReloadPlugin(t, Options{Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 1000})
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		t.Fatal(err)
	}
	running := make(chan any, 1)
	go func() {
		_, _, r := serveCode(h, "/a?sleep=1h")
		running <- r
	}()
	if !waitFor(2*time.Second, func() bool { return p.M.inflight.Load() == 1 }) {
		t.Fatal("the request did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	t0 := time.Now()
	if err := p.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseContext: %v, want %v", err, context.DeadlineExceeded)
	}
	if d := time.Since(t0); d > 2*time.Second {
		t.Fatalf("CloseContext took %v", d)
	}
	select {
	case r := <-running:
		if r != http.ErrAbortHandler {
			t.Fatalf("the request at the deadline: panic %v, want %v", r, http.ErrAbortHandler)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the request at the deadline did not stop")
	}
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("after CloseContext: %d goroutines still run", p.M.Goroutines())
	}
	if err := p.CloseContext(context.Background()); err != nil { // idempotent
		t.Fatal(err)
	}
}

// TestPluginCloseFuncValue checks Close and CloseContext with func values
// of the plugin that the host calls (not through Plugin.Call): the drain
// waits for such a call, a call after Close is refused with ErrClosed, and a
// call that Close stops gives ErrClosed to the host (a panic, or the error
// result) instead of ending its goroutine with runtime.Goexit. Before, such
// a call ran after Close, was not counted by the drain, and a call stopped
// by Close ended the goroutine of the host without an error.
func TestPluginCloseFuncValue(t *testing.T) {
	opts := Options{Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 1000, GoPanic: func(r any) {
		t.Errorf("GoPanic: %v", r)
	}}
	fnOf := func(p *Plugin, name string, arg any) any {
		t.Helper()
		objs, err := p.Call(context.Background(), name, arg)
		if err != nil {
			t.Fatal(err)
		}
		v, err := objs[0].Interface()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// callInt calls f on a goroutine of its own, and gives its result, its
	// panic, or "goexit".
	callInt := func(f func() int) chan any {
		out := make(chan any, 1)
		go func() {
			ok := false
			defer func() {
				r := recover()
				if !ok && r == nil {
					out <- "goexit"
				} else if r != nil {
					out <- r
				}
			}()
			n := f()
			ok = true
			out <- n
		}()
		return out
	}
	isErrClosed := func(r any) bool {
		err, ok := r.(error)
		return ok && errors.Is(err, ErrClosed)
	}

	// The drain waits for a call of a func value, and a call after Close is
	// refused.
	p, _ := loadReloadPlugin(t, opts)
	wait := make(chan struct{})
	f := fnOf(p, "Fn", wait).(func() int)
	ready := make(chan struct{})
	close(ready)
	later := fnOf(p, "Fn", ready).(func() int)
	ferr := fnOf(p, "FnErr", false).(func() error)
	running := callInt(f)
	if !waitFor(2*time.Second, func() bool { return p.M.inflight.Load() == 1 }) {
		t.Fatalf("the call of the func value is not counted: inflight %d, want 1", p.M.inflight.Load())
	}
	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closed <- p.CloseContext(ctx)
	}()
	if waitFor(100*time.Millisecond, p.M.Closed) {
		t.Fatal("closed before the call of the func value ended")
	}
	close(wait)
	if r := <-running; r != 1 {
		t.Fatalf("the call that ran: %v, want 1", r)
	}
	if err := <-closed; err != nil {
		t.Fatalf("CloseContext: %v, want nil", err)
	}
	if r := <-callInt(later); !isErrClosed(r) {
		t.Fatalf("call after Close: %v, want the panic %v", r, ErrClosed)
	}
	if err := ferr(); !errors.Is(err, ErrClosed) {
		t.Fatalf("call after Close: %v, want %v", err, ErrClosed)
	}

	// A call that Close stops gives ErrClosed to the host.
	p2, _ := loadReloadPlugin(t, opts)
	blocked := callInt(fnOf(p2, "Fn", (chan struct{})(nil)).(func() int))
	blockedErr := make(chan error, 1)
	berr := fnOf(p2, "FnErr", true).(func() error)
	go func() { blockedErr <- berr() }()
	if !waitFor(2*time.Second, func() bool { return p2.M.inflight.Load() == 2 }) {
		t.Fatalf("the calls of the func values are not counted: inflight %d, want 2", p2.M.inflight.Load())
	}
	p2.Close()
	select {
	case r := <-blocked:
		if !isErrClosed(r) {
			t.Fatalf("the call stopped by Close: %v, want the panic %v", r, ErrClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call stopped by Close did not end")
	}
	select {
	case err := <-blockedErr:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("the call stopped by Close: %v, want %v", err, ErrClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call stopped by Close did not end")
	}
}

// TestPluginCloseFuncValueUnderCatcher checks that a func() error of the
// plugin called after Close returns ErrClosed also when a native catcher is
// on the Go stack of the host: an HTTP handler of the host (net/http
// recovers its panics). Before, the panic ErrClosed went on to the catcher:
// net/http logged a panic and dropped the connection.
func TestPluginCloseFuncValueUnderCatcher(t *testing.T) {
	opts := Options{Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 1000, GoPanic: func(r any) {
		t.Errorf("GoPanic: %v", r)
	}}
	p, _ := loadReloadPlugin(t, opts)
	objs, err := p.Call(context.Background(), "FnErr", false)
	if err != nil {
		t.Fatal(err)
	}
	v, err := objs[0].Interface()
	if err != nil {
		t.Fatal(err)
	}
	ferr := v.(func() error)
	p.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := ferr(); !errors.Is(err, ErrClosed) {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("call after Close in an HTTP handler: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("call after Close in an HTTP handler: status %d, want %d", resp.StatusCode, http.StatusTeapot)
	}
}
