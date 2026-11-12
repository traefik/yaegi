package vm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

// goroutineStacks counts the goroutines of the process whose stack has
// each of the frames.
func goroutineStacks(frames ...string) map[string]int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	counts := map[string]int{}
	for _, g := range strings.Split(string(buf), "\n\n") {
		for _, f := range frames {
			if strings.Contains(g, f) {
				counts[f]++
			}
		}
	}
	return counts
}

// frameReadLoop is in the stack of each open client connection.
const frameReadLoop = "net/http.(*persistConn).readLoop"

// TestGoroutinesAfterCancel (TODO "Goroutines after cancel in VM mode"):
// in the catalog soak, the default mode kept 34 goroutines after the cancel
// of the context of New, against 19 for yaegi and 20 for AOT mode. It is not
// a leak of the VM: the goroutines of the plugin end with the context, and
// what stays are the idle keep-alive connections of the DefaultTransport of
// each machine (a clone of the host's, hostregs.go; two goroutines per
// connection), where yaegi and the AOT copies share the host's. They close
// after IdleConnTimeout (90 s, as the host's), and now also when the
// machine is gone.
func TestGoroutinesAfterCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	// Idle connections of earlier tests (the host's DefaultTransport).
	base := goroutineStacks(frameReadLoop)[frameReadLoop]
	ctx, cancel := context.WithCancel(context.Background())
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/idleconnplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(ctx, p, http.NotFoundHandler(), cfg, "idle")
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if body, r := serveBG(h, "/?u="+srv.URL); body != "ok" || r != nil {
			t.Fatalf("body %q, panic %v", body, r)
		}
	}
	if n := p.M.Goroutines(); n != 2 {
		t.Errorf("%d goroutines of the plugin before the cancel, want 2", n)
	}
	cancel()
	m := p.M
	if !waitFor(10*time.Second, func() bool { return m.Goroutines() == 0 }) {
		t.Fatalf("%d goroutines of the plugin after the cancel", m.Goroutines())
	}
	// The plugin's requests reuse one connection of its DefaultTransport.
	if n := goroutineStacks(frameReadLoop)[frameReadLoop] - base; n != 1 {
		t.Errorf("%d idle connections of the plugin, want 1", n)
	}

	// Once the plugin is gone, its idle connections close.
	p, h, m, cfg = nil, nil, nil, nil
	if !waitFor(20*time.Second, func() bool {
		runtime.GC()
		return goroutineStacks(frameReadLoop)[frameReadLoop] <= base
	}) {
		t.Errorf("%d idle connections after the plugin is gone", goroutineStacks(frameReadLoop)[frameReadLoop]-base)
	}
}

// TestClientTimeoutTransport checks that a request of a plugin client with
// a nil Transport and a Timeout goes through an *http.Transport, as in Go.
// The machine's DefaultTransport was reached through a wrapper type, which
// net/http does not know: for a Client.Timeout it then used the legacy
// cancellation (Request.Cancel, a timer and one more goroutine per request
// until the timer or the end of the body). The catalog provider scenario
// counted them: the Service Fabric provider kept 6 more goroutines than
// native after Stop.
func TestClientTimeoutTransport(t *testing.T) {
	needSource(t)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	rl0 := goroutineStacks(frameReadLoop)[frameReadLoop]
	dir, pkgPath := writePlugin(t, `package p

import (
	"io"
	"net/http"
	"time"
)

func Get(url string) string {
	c := &http.Client{Timeout: 30 * time.Second}
	r, err := c.Get(url)
	if err != nil {
		return err.Error()
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return string(b)
}
`)
	p, err := Load(Options{Dir: dir}, pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	const legacy = "net/http.setRequestCancel"
	base := goroutineStacks(legacy)[legacy]
	done := make(chan string, 1)
	go func() {
		res, err := p.Call(context.Background(), "Get", srv.URL)
		if err != nil {
			done <- err.Error()
			return
		}
		v, _ := res[0].Interface()
		done <- v.(string)
	}()
	<-entered
	n := goroutineStacks(legacy)[legacy] - base
	close(release)
	if got := <-done; got != "ok" {
		t.Fatalf("Get: %q", got)
	}
	if n != 0 {
		t.Errorf("%d goroutines of the legacy cancellation of net/http during the request, want 0 (the Transport is not an *http.Transport)", n)
	}
	// The idle connection of the plugin ends with the server: the next
	// tests count the connections of the process.
	srv.Close()
	waitFor(10*time.Second, func() bool { return goroutineStacks(frameReadLoop)[frameReadLoop] <= rl0 })
}
