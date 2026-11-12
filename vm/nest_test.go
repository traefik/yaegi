package vm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/debug"
	"testing"
	"time"
)

// TestNestStackCost measures the Go stack that a recursion through native
// callbacks uses before it stops with ErrStackOverflow, at the default
// MaxNest. It must stay far below the 1 GB limit of gc (250 MB on 32-bit
// platforms): past it, the process dies.
//
// StackInuse counts the stacks of the whole process: before each path,
// the test waits until the large stack of the previous path is freed, so
// the measure only sees the stack of the current path.
func TestNestStackCost(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/nativerecplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := createConfig(p)
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "rec")
	if err != nil {
		t.Fatal(err)
	}
	stackInuse := func() int64 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return int64(ms.StackInuse)
	}
	runtime.GC()
	base := stackInuse()
	limit := int64(128<<20) * raceStackFactor
	defer debug.SetGCPercent(debug.SetGCPercent(-1)) // a GC could shrink the stack
	for _, path := range []string{"/map", "/sort", "/stringer", "/defer"} {
		used := make(chan int64)
		ended := make(chan bool)
		go func() {
			defer close(ended)
			before := stackInuse()
			func() {
				defer func() {
					if r := recover(); r != ErrStackOverflow {
						t.Errorf("%s: panic %v, want %v", path, r, ErrStackOverflow)
					}
				}()
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
			}()
			// The stack of this goroutine is still as large as at the
			// deepest point: gc only shrinks it during a GC.
			used <- stackInuse() - before
		}()
		n := <-used
		<-ended
		t.Logf("%s: %d MB of Go stack for %d nested runs (at most %d bytes per run)", path, n>>20, DefaultMaxNest, n/DefaultMaxNest)
		if n <= 0 || n > limit {
			t.Errorf("%s: %d MB of Go stack, want more than 0 and at most %d MB", path, n>>20, limit>>20)
		}
		// Wait until the goroutine has exited and gc has freed its stack.
		deadline := time.Now().Add(30 * time.Second)
		for runtime.GC(); stackInuse() > base+(8<<20); runtime.GC() {
			if time.Now().After(deadline) {
				t.Fatalf("after %s: the stack of the path is not freed (%d MB in use, %d MB at the start)", path, stackInuse()>>20, base>>20)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// TestGoroutineTable fills one bucket of the goroutine table past its
// slots: the next goroutines go to the spill map, and all are found again
// until they leave.
func TestGoroutineTable(t *testing.T) {
	const n = gWays + 3
	var keys []uintptr
	b := gbucket(1 << 6)
	for k := uintptr(1); len(keys) < n; k++ {
		if gbucket(k<<6) == b {
			keys = append(keys, k<<6) // not a real goroutine address
		}
	}
	states := map[uintptr]*gstate{}
	for _, g := range keys {
		gs, fresh := enterG(g, &ctx{})
		if !fresh {
			t.Fatalf("key %#x: found before it entered", g)
		}
		states[g] = gs
	}
	if gspill.n.Load() < 3 {
		t.Errorf("spill map: %d goroutines, want at least 3", gspill.n.Load())
	}
	for _, g := range keys {
		if gs, fresh := enterG(g, &ctx{}); fresh || gs != states[g] {
			t.Errorf("key %#x: lookup gives another state", g)
		}
	}
	for _, g := range keys {
		leaveG(g, states[g])
	}
	for _, g := range keys {
		if gs, fresh := enterG(g, &ctx{}); !fresh {
			t.Errorf("key %#x: still found after it left", g)
		} else {
			leaveG(g, gs)
		}
	}
}
