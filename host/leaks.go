package host

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"github.com/traefik/yaegi/v2/vm"
)

// LeakDelay is the delay that docs/RELOAD.md recommends for Leaks.Check:
// a goroutine that ends with the context of New stops about 1 ms after the
// cancel, and one that waits for a short timer a little later.
const LeakDelay = 5 * time.Second

// Leaks follows the instances of the New calls of one plugin, and reports
// the ones that still have goroutines after the context of their New was
// canceled. In Traefik this context is the one of a configuration, canceled
// when the next configuration is built, so the goroutines of an instance
// that still run then are goroutines that New left: under the reload model
// of docs/RELOAD.md they live until Plugin.Close at shutdown. Leaks does
// not log: the host logs each Leak that Check returns, once.
//
// Leaks keeps no instance alive (weak pointers), and forgets an instance
// once it is reported, once it has no goroutine left after the delay, or
// once nothing uses it. It is safe for concurrent use. The zero value is
// ready to use.
type Leaks struct {
	mu      sync.Mutex
	entries []*leakEntry
}

// Leak is an instance that Check reports.
type Leak struct {
	Name       string // the name given to Add (the middleware)
	Goroutines int    // live goroutines of the instance at the check
}

type leakEntry struct {
	inst   weak.Pointer[vm.Instance]
	name   string
	done   atomic.Int64 // UnixNano of the cancel of the context of New, 0 before
	cancel func() bool  // stops the context.AfterFunc
}

// Add follows inst, the instance of a New call made with the context ctx
// (the ctx argument of New), under the name name.
func (l *Leaks) Add(ctx context.Context, inst *vm.Instance, name string) {
	e := &leakEntry{inst: weak.Make(inst), name: name}
	e.cancel = context.AfterFunc(ctx, func() { e.done.Store(time.Now().UnixNano()) })
	l.mu.Lock()
	l.entries = append(l.entries, e)
	l.mu.Unlock()
}

// Check returns the instances whose New context was canceled at least
// delay ago (LeakDelay) and that still have more than max goroutines
// (docs/RELOAD.md recommends 0). Each instance is reported once.
func (l *Leaks) Check(max int, delay time.Duration) []Leak {
	now := time.Now().UnixNano()
	var out []Leak
	l.mu.Lock()
	defer l.mu.Unlock()
	keep := l.entries[:0]
	for _, e := range l.entries {
		inst := e.inst.Value()
		if inst == nil {
			e.cancel()
			continue // nothing uses it any more: no goroutine
		}
		d := e.done.Load()
		if d == 0 || now-d < int64(delay) {
			keep = append(keep, e) // its configuration is live, or the delay runs
			continue
		}
		if g := inst.Goroutines(); g > max {
			out = append(out, Leak{Name: e.name, Goroutines: g})
		}
	}
	clear(l.entries[len(keep):])
	l.entries = keep
	return out
}

// Len returns the number of instances that l follows.
func (l *Leaks) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
