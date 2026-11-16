package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/vm"
)

const leakPlug = "github.com/traefik/yaegi/v2/internal/pluginhost/testdata/leakplug"

func loadLeakPlug(t *testing.T, aot bool) *vm.Plugin {
	t.Helper()
	opts := Options()
	opts.Dir, opts.AOT = "../..", aot
	p, err := vm.Load(opts, leakPlug)
	if err != nil {
		t.Fatalf("AOT %v: %v", aot, err)
	}
	return p
}

// TestReloadLeakedGoroutines is the reload model of docs/RELOAD.md: one
// Plugin for the life of the host, and New at each reload of the dynamic
// configuration, as Traefik does with yaegi. Each New of leakplug leaves a
// goroutine that never ends (darkweak/souin leaves 2). Before, they all
// counted in the MaxGoroutines of the machine (1000 in Options), so every
// New failed with ErrTooManyGoroutines after 1000 reloads. Now NewHandler
// runs each New in its own vm.Instance, and the cap bounds the goroutines
// of each instance. The package variables keep their state across the New
// calls, and the handler of the first round still serves.
func TestReloadLeakedGoroutines(t *testing.T) {
	for _, aot := range []bool{false, true} {
		testReloadLeakedGoroutines(t, aot)
	}
}

func testReloadLeakedGoroutines(t *testing.T, aot bool) {
	p := loadLeakPlug(t, aot)
	defer p.Close()
	cfg, err := CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	reloads := 2 * Options().MaxGoroutines
	var first http.Handler
	for n := 1; n <= reloads; n++ {
		h, err := NewHandler(context.Background(), p, http.NotFoundHandler(), cfg, "leak")
		if err != nil {
			t.Fatalf("New %d: %v (goroutines of the plugin: %d)", n, err, p.M.Goroutines())
		}
		if first == nil {
			first = h
		}
	}
	if g := p.M.Goroutines(); g != reloads {
		t.Errorf("%d goroutines of the plugin after %d New, want %d (one janitor per New)", g, reloads, reloads)
	}
	if got := call(t, p, "News")[0]; got != int64(reloads) {
		t.Errorf("News: %v, want %d (the package variable lost its state)", got, reloads)
	}
	rec := httptest.NewRecorder()
	first.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if want := "instance 1 of 2000"; rec.Body.String() != want {
		t.Errorf("first handler: %q, want %q", rec.Body.String(), want)
	}
}

// leakConfig returns a configuration of leakplug.
func leakConfig(t *testing.T, p *vm.Plugin, leak int, withCtx bool) *vm.Object {
	t.Helper()
	cfg, err := CreateConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"leak": leak, "withCtx": withCtx})
	if err := cfg.Decode(func(ptr any) error { return json.Unmarshal(data, ptr) }); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// waitGoroutines waits until f returns want, for at most 2 s.
func waitGoroutines(f func() int, want int) bool {
	deadline := time.Now().Add(2 * time.Second)
	for f() != want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	return f() == want
}

// TestInstanceClose checks the tools that the host gets for the goroutines
// of an instance: Instance.Goroutines shows them, Instance.Close stops the
// ones of that instance only (the other instances, the handler and the
// package state are not touched), and Plugin.Close stops the goroutines of
// every instance, also the ones that the host dropped. MaxGoroutines bounds
// each instance, and the goroutines of a request belong to no instance.
func TestInstanceClose(t *testing.T) {
	p := loadLeakPlug(t, false)
	max := Options().MaxGoroutines
	h1, i1, err := NewInstanceHandler(context.Background(), p, http.NotFoundHandler(), leakConfig(t, p, 3, false), "a")
	if err != nil {
		t.Fatal(err)
	}
	h2, i2, err := NewInstanceHandler(context.Background(), p, http.NotFoundHandler(), leakConfig(t, p, 2, false), "b")
	if err != nil {
		t.Fatal(err)
	}
	// A third instance that the host drops.
	if _, err := NewHandler(context.Background(), p, http.NotFoundHandler(), leakConfig(t, p, 4, false), "c"); err != nil {
		t.Fatal(err)
	}
	if i1.Goroutines() != 3 || i2.Goroutines() != 2 || p.M.Goroutines() != 9 {
		t.Fatalf("goroutines: instance 1 %d, instance 2 %d, plugin %d, want 3, 2, 9", i1.Goroutines(), i2.Goroutines(), p.M.Goroutines())
	}
	// One New past the cap of its instance fails; the others do not count.
	_, i4, err := NewInstanceHandler(context.Background(), p, http.NotFoundHandler(), leakConfig(t, p, max+1, false), "d")
	if !errors.Is(err, vm.ErrTooManyGoroutines) {
		t.Fatalf("New with %d goroutines: %v, want %v", max+1, err, vm.ErrTooManyGoroutines)
	}
	// The instance of the failed New is closed: its goroutines stop.
	if !i4.Closed() {
		t.Fatal("the instance of the failed New is not closed")
	}
	if !waitGoroutines(p.M.Goroutines, 9) {
		t.Fatalf("after the New past the cap: %d goroutines, want 9", p.M.Goroutines())
	}

	i1.Close()
	if !waitGoroutines(i1.Goroutines, 0) {
		t.Fatalf("after Close of instance 1: %d goroutines of it still run", i1.Goroutines())
	}
	if i2.Goroutines() != 2 {
		t.Fatalf("after Close of instance 1: instance 2 has %d goroutines, want 2", i2.Goroutines())
	}
	for _, h := range []http.Handler{h1, h2} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/goroutines", nil))
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Errorf("ServeHTTP after Close of instance 1: %d %q", rec.Code, rec.Body.String())
		}
	}
	if got := call(t, p, "News")[0]; got != int64(4) {
		t.Errorf("News: %v, want 4", got)
	}

	p.Close()
	if !waitGoroutines(p.M.Goroutines, 0) {
		t.Fatalf("after Plugin.Close: %d goroutines still run (instance 2: %d)", p.M.Goroutines(), i2.Goroutines())
	}
}

// heapAfterGC returns the live heap.
func heapAfterGC() uint64 {
	var ms runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// TestReloadHeap runs 10000 reloads of leakplug on one Plugin, as Traefik
// does them: a new context for each round, and the one of the round before
// canceled (RouterFactory.CreateRouters). The goroutine of each New ends
// with its context, so the plugin leaks nothing but its package cache: the
// heap must stay flat (the instances, their accounts and the entries of the
// table of the instances of the machine are freed).
func TestReloadHeap(t *testing.T) {
	if testing.Short() {
		t.Skip("10000 reloads")
	}
	p := loadLeakPlug(t, false)
	defer p.Close()
	cfg := leakConfig(t, p, 0, true)
	cancel := context.CancelFunc(func() {})
	var before uint64
	const warm, reloads = 1000, 10000
	for n := 1; n <= reloads; n++ {
		cancel()
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		if _, err := NewHandler(ctx, p, http.NotFoundHandler(), cfg, "heap"); err != nil {
			t.Fatalf("New %d: %v", n, err)
		}
		if n == warm {
			waitGoroutines(p.M.Goroutines, 1)
			before = heapAfterGC()
		}
	}
	if !waitGoroutines(p.M.Goroutines, 1) {
		t.Fatalf("%d goroutines after %d reloads, want 1", p.M.Goroutines(), reloads)
	}
	after := heapAfterGC()
	growth := int64(after) - int64(before)
	t.Logf("heap after %d reloads: %d bytes, after %d: %d bytes (%+d bytes, %+.1f bytes per New)", warm, before, reloads, after, growth, float64(growth)/(reloads-warm))
	// 9000 reloads: 32 bytes per New would be 288 KB. The table of the
	// instances without its cleanup grows by about 50 bytes per New.
	if per := float64(growth) / (reloads - warm); per > 32 {
		t.Errorf("the heap grows by %.1f bytes per New, want it flat", per)
	}
	if got := call(t, p, "News")[0]; got != int64(reloads) {
		t.Errorf("News: %v, want %d (the package variable lost its state)", got, reloads)
	}
	cancel()
}
