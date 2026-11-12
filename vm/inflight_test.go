package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"

	"github.com/mitchellh/mapstructure"
)

// inflightHeap returns the heap in bytes that one request keeps while it is
// blocked in the next handler of a plugin, measured over n concurrent
// requests (the requests and recorders are made before the first measure).
func inflightHeap(t testing.TB, n int) float64 {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/plugins/w1headers")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Decode(func(ptr any) error {
		return mapstructure.Decode(map[string]any{"prefix": "X-"}, ptr)
	})
	if err != nil {
		t.Fatal(err)
	}
	in, release := make(chan struct{}), make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in <- struct{}{}
		<-release
	})
	h, err := newHandler(context.Background(), p, next, cfg, "w1")
	if err != nil {
		t.Fatal(err)
	}
	// Warm up: the first request fills the caches of the call sites.
	go func() { <-in; release <- struct{}{} }()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	reqs := make([]*http.Request, n)
	recs := make([]*httptest.ResponseRecorder, n)
	for i := range reqs {
		reqs[i] = httptest.NewRequest("GET", "/", nil)
		recs[i] = httptest.NewRecorder()
	}
	var before, after runtime.MemStats
	// Two collections empty the ctx pool (sync.Pool keeps a victim cache
	// for one cycle): each request then gets a new ctx.
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { h.ServeHTTP(recs[i], reqs[i]) })
	}
	for range n {
		<-in
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	close(release)
	wg.Wait()
	runtime.KeepAlive(reqs)
	runtime.KeepAlive(recs)
	return float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / float64(n)
}

// TestInflightHeap checks the heap that a request blocked in a plugin keeps.
// Each ServeHTTP runs on its own ctx: a register stack of 4096 Values made
// up front cost about 100 KB per in-flight request (hundreds of MB with
// thousands of slow clients). The stack now starts at the size of the entry
// frame and grows on demand: about 6 KB per request.
func TestInflightHeap(t *testing.T) {
	if testing.Short() {
		t.Skip("heap measure")
	}
	const limit = 32 << 10
	got := inflightHeap(t, 1000)
	t.Logf("heap per in-flight request: %.0f bytes", got)
	if got > limit {
		t.Errorf("heap per in-flight request: %.0f bytes, want at most %d", got, limit)
	}
}
