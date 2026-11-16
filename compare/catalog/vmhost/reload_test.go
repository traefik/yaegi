package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/host"
	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

const reloadPlug = "github.com/traefik/yaegi/v2/compare/catalog/vmhost/testdata/reloadplug"

// TestReloadSandboxLimits runs 200 reloads of testdata/reloadplug as Traefik
// does them (TODO.md section 1, the two reload items of the catalog): each
// round calls CreateConfig and New with a new context, and the context of
// the round before is canceled first (RouterFactory.CreateRouters). The
// plugin is loaded with the limits of host.Options, an
// Options.Sandbox (sandboxOptions) and a cap of 20 goroutines. Each New
// starts a goroutine that ends with its context: if a canceled context did
// not reach it, the 21st round would pass the cap. After the 200 rounds,
// the limits still hold, on the last handler and on the handler of the
// first round: the budget stops a `for {}`, the sandbox refuses a read and
// a write outside its paths and gives an empty environment, and the cap
// stops 100 new goroutines.
func TestReloadSandboxLimits(t *testing.T) {
	for _, aot := range []bool{false, true} {
		opts := host.Options()
		opts.Dir, opts.AOT = "../..", aot
		opts.MaxGoroutines = 20
		sandboxOptions(&opts)
		load := vm.Load
		if aot {
			load = host.Load // host.Check accepts a Sandbox with limits
		}
		p, err := load(opts, reloadPlug)
		if err != nil {
			t.Fatalf("AOT %v: %v", aot, err)
		}
		var first, last http.Handler
		var cancel context.CancelFunc = func() {}
		for n := 0; n < 200; n++ {
			cancel()
			// The goroutine of the round before sees its context
			// done and ends: about 1 ms after the cancel. Traefik
			// reloads at most every 2 s (providersThrottleDuration).
			deadline := time.Now().Add(2 * time.Second)
			for p.M.Goroutines() > 0 && time.Now().Before(deadline) {
				time.Sleep(100 * time.Microsecond)
			}
			if g := p.M.Goroutines(); g > 0 {
				t.Fatalf("AOT %v, round %d: %d goroutines of the plugin still run 2 s after the cancel of the context of the round before", aot, n, g)
			}
			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())
			cfg, err := pluginhost.CreateConfig(p)
			if err != nil {
				t.Fatalf("AOT %v, round %d: CreateConfig: %v", aot, n, err)
			}
			h, err := pluginhost.NewHandler(ctx, p, http.NotFoundHandler(), cfg, "reload")
			if err != nil {
				t.Fatalf("AOT %v, round %d: New: %v (goroutines of the plugin: %d)", aot, n, err, p.M.Goroutines())
			}
			if n == 0 {
				first = h
			}
			last = h
		}
		// Only the goroutine of the last round runs.
		deadline := time.Now().Add(5 * time.Second)
		for p.M.Goroutines() > 1 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if g := p.M.Goroutines(); g > 1 {
			t.Errorf("AOT %v: %d goroutines of the plugin after 200 reloads, want 1", aot, g)
		}
		for _, hc := range []struct {
			name string
			h    http.Handler
		}{{"last", last}, {"first", first}} {
			hh := &hostileHost{h: hc.h}
			check := func(what string, err error) {
				if err != nil {
					t.Errorf("AOT %v, %s handler, %s: %v", aot, hc.name, what, err)
				}
			}
			check("budget", spinStopped(hh))
			check("goroutines", panicIs(hh.serve("/goroutines", 10*time.Second), vm.ErrTooManyGoroutines))
			for path, want := range map[string]string{
				"/":      "reload ok",
				"/read":  "open /etc/hostname: denied by the plugin sandbox",
				"/write": "denied by the plugin sandbox",
				"/env":   `HOME=""`,
			} {
				s := hh.serve(path, 10*time.Second)
				if s.timeout || s.panic != nil || !strings.Contains(s.body, want) {
					t.Errorf("AOT %v, %s handler, %s: %+v, want a body with %q", aot, hc.name, path, s, want)
				}
			}
		}
		cancel()
	}
}

// spinStopped sends /spin, a loop without a blocking point, and checks that
// the budget stops it (quietly: QuietBudget) after at most
// pluginhost.Budget+1 rounds plus the steps that BudgetRate gives back while
// it runs: each round but the first takes at least one step (the backward
// jump that starts it; with AOT this jump is the only step). /spun counts
// whole steps of 1<<16 rounds, so it is at most the rounds that ran. It
// counts work, not wall time: the budget counts steps, and under -race on a
// loaded runner the 1e8 steps of pluginhost.Budget took more than the 30 s
// of a fixed timeout. It waits as long as the loop makes progress (the count
// of /spun grows), and fails when the loop runs and makes no progress for
// stall, or ends without the budget stop.
func spinStopped(hh *hostileHost) error {
	const stall = 30 * time.Second
	spun := func() (int64, error) {
		s := hh.serve("/spun", stall)
		if s.timeout || s.panic != nil {
			return 0, fmt.Errorf("/spun: %+v", s)
		}
		return strconv.ParseInt(s.body, 10, 64)
	}
	before, err := spun()
	if err != nil {
		return err
	}
	done := make(chan served, 1)
	start := time.Now()
	go func() { done <- hh.serve("/spin", 24*time.Hour) }()
	// The most rounds the budget lets the loop run until now.
	most := func() int64 {
		return pluginhost.Budget + 1 + int64(float64(pluginhost.BudgetRate)*time.Since(start).Seconds())
	}
	last, moved := before, time.Now()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case s := <-done:
			// The budget stops the request quietly (QuietBudget, vm/budget.go).
			if err := panicIs(s, http.ErrAbortHandler); err != nil {
				return err
			}
			after, err := spun()
			if err != nil {
				return err
			}
			if n, m := after-before, most(); n <= 0 || n > m {
				return fmt.Errorf("the loop ran %d rounds before the budget stop, want 1 to %d (pluginhost.Budget+1, and BudgetRate for %v)", n, m, time.Since(start))
			}
			return nil
		case <-tick.C:
			n, err := spun()
			if err != nil {
				return err
			}
			if n > last {
				last, moved = n, time.Now()
			} else if time.Since(moved) > stall {
				return fmt.Errorf("the request still runs after %d rounds, with no progress for %v", n-before, stall)
			}
			if m := most(); n-before > m {
				return fmt.Errorf("the loop still runs after %d rounds, more than %d (pluginhost.Budget+1, and BudgetRate for %v)", n-before, m, time.Since(start))
			}
		}
	}
}
