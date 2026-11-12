package vm

import (
	"context"
	"net/http"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests of Options.MaxMemory (memquota.go). Plugin:
// testdata/restricted/memplug.

const memPlug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/memplug"

// TestMaxMemory: with a quota of 64 MiB, a plugin that keeps 256 KiB
// buffers in a global, grows a slice of ints, doubles a string or appends
// large arrays gets ErrMemory near the quota, and recovers it. A plugin
// that makes and drops 2 GiB of 1 MiB buffers is not refused: the bytes of
// the freed buffers are given back. Before, nothing counted the memory of a
// plugin: these routes allocate until the host runs out of memory (the
// routes stop at 512 MiB here).
func TestMaxMemory(t *testing.T) {
	const max = 64 << 20
	opts := Options{Dir: "..", MaxMemory: max}
	p, err := Load(opts, memPlug)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "x")
	if err != nil {
		t.Fatal(err)
	}
	serve := func(path string) string {
		body, r := serveBG(h, path)
		if r != nil {
			t.Fatalf("%s: panic %v", path, r)
		}
		return body
	}
	body := serve("/keep")
	f := strings.Fields(body)
	if len(f) < 2 || strings.Join(f[:len(f)-1], " ") != ErrMemory.Error() {
		t.Fatalf("/keep: %q, want the panic %q", body, ErrMemory)
	}
	// The global slice of 256 KiB buffers: about max/256 KiB of them.
	if n, _ := strconv.Atoi(f[len(f)-1]); n < max/(256<<10)/2 || n > max/(256<<10) {
		t.Errorf("/keep: %d buffers of 256 KiB kept, want about %d", n, max/(256<<10))
	}
	if used := p.M.Memory(); used > max {
		t.Errorf("Memory() = %d, more than the quota %d", used, max)
	}
	for _, path := range []string{"/ints", "/concat", "/arrays"} {
		serve("/reset")
		if got := serve(path); got != ErrMemory.Error() {
			t.Errorf("%s: %q, want the panic %q", path, got, ErrMemory)
		}
	}
	serve("/reset")
	if got := serve("/churn"); got != "ok" {
		t.Errorf("/churn: %q, want no panic (the dropped buffers are freed)", got)
	}
}

// TestMaxMemoryGCRate: plugins that recover ErrMemory in a loop (4 machines
// at their quota, 2 goroutines each, for 1.2 s) force few garbage
// collections: at most one every gcEvery in the whole process, and each
// machine waits longer after each collection that made no room. Before,
// each machine forced one every 50 ms: about 96 collections here, and a
// plugin per core could make the host collect all the time.
func TestMaxMemoryGCRate(t *testing.T) {
	forced := func() uint64 {
		s := []metrics.Sample{{Name: "/gc/cycles/forced:gc-cycles"}}
		metrics.Read(s)
		return s[0].Value.Uint64()
	}
	before := forced()
	var wg sync.WaitGroup
	for range 4 {
		m := &Machine{mem: &memQuota{max: 1, sweepAt: 64}}
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for end := time.Now().Add(1200 * time.Millisecond); time.Now().Before(end); {
					func() {
						defer func() {
							if r := recover(); r != ErrMemory {
								t.Errorf("panic %v, want %v", r, ErrMemory)
							}
						}()
						m.memCheck(2)
					}()
				}
			}()
		}
	}
	wg.Wait()
	// One machine: collections at 0, 50, 150, 350 and 750 ms, so 5; the
	// global period lets the others in between.
	if n := forced() - before; n > 24 {
		t.Errorf("%d forced collections in 1.2 s, want at most 24", n)
	} else {
		t.Logf("%d forced collections", n)
	}
}

// TestMaxMemoryOff: without MaxMemory nothing is counted.
func TestMaxMemoryOff(t *testing.T) {
	p, err := Load(Options{Dir: ".."}, memPlug)
	if err != nil {
		t.Fatal(err)
	}
	if p.M.mem != nil || p.M.Memory() != 0 {
		t.Fatal("a memory count without MaxMemory")
	}
}
