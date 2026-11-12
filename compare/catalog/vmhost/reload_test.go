package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/v2/internal/pluginhost"
	"github.com/traefik/yaegi/v2/vm"
)

const reloadPlug = "github.com/traefik/yaegi/v2/compare/catalog/vmhost/testdata/reloadplug"

// TestReloadSandboxLimits runs 200 reloads of testdata/reloadplug as Traefik
// does them (TODO.md section 1, the two reload items of the catalog): each
// round calls CreateConfig and New with a new context, and the context of
// the round before is canceled first (RouterFactory.CreateRouters). The
// plugin is loaded with the limits of pluginhost.Options, an
// Options.Sandbox (sandboxOptions) and a cap of 20 goroutines. Each New
// starts a goroutine that ends with its context: if a canceled context did
// not reach it, the 21st round would pass the cap. After the 200 rounds,
// the limits still hold, on the last handler and on the handler of the
// first round: the budget stops a `for {}`, the sandbox refuses a read and
// a write outside its paths and gives an empty environment, and the cap
// stops 100 new goroutines.
func TestReloadSandboxLimits(t *testing.T) {
	for _, aot := range []bool{false, true} {
		opts := pluginhost.Options()
		opts.Dir, opts.AOT = "../..", aot
		opts.MaxGoroutines = 20
		sandboxOptions(&opts)
		p, err := vm.Load(opts, reloadPlug)
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
			check("budget", panicIs(hh.serve("/spin", 30*time.Second), vm.ErrBudget))
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
