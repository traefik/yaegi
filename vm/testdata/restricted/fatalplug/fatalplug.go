// Package fatalplug makes the errors that the Go runtime treats as fatal
// (unlock of an unlocked lock, concurrent map writes, a misuse of
// runtime.SetFinalizer, a nil func that native code starts later on a
// goroutine of its own), for Options.PanicOnFatal (fatal.go). Each route
// recovers the panic and writes it, so the test sees what the plugin sees.
package fatalplug

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type handler struct{}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return handler{}, nil
}

type pair struct{ a, b int }

type guarded struct {
	mu sync.Mutex
	rw sync.RWMutex
}

// try runs f and returns its panic, as text ("ok" without a panic).
func try(f func()) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	f()
	return "ok"
}

// race runs f on n goroutines, rounds times each, and returns the first
// panic ("ok": none). A goroutine that is done starts its rounds again until
// every goroutine has done its rounds once (or 5 s): on a busy machine, the
// rounds of one goroutine can all end before the next goroutine starts, and
// then the goroutines never overlap. A panic stops them all.
func race(n, rounds int, f func(g, i int)) string {
	var wg sync.WaitGroup
	var stop atomic.Bool
	var once atomic.Int32 // the goroutines that have done their rounds once
	end := time.Now().Add(5 * time.Second)
	res := make(chan string, n)
	for g := 0; g < n; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := try(func() {
				for first := true; !stop.Load(); first = false {
					for i := 0; i < rounds && !stop.Load(); i++ {
						f(g, i)
					}
					if first {
						once.Add(1)
					}
					if once.Load() == int32(n) || time.Now().After(end) {
						return
					}
				}
			})
			if r != "ok" {
				stop.Store(true)
			}
			res <- r
		}()
	}
	wg.Wait()
	close(res)
	out := "ok"
	for r := range res {
		if r != "ok" {
			out = r
		}
	}
	return out
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/locks":
		// The uses of Go that must keep working: no false panic.
		var g guarded
		var l sync.Locker = &g.mu
		fmt.Fprintln(w, try(func() {
			g.mu.Lock()
			g.mu.Unlock()
			l.Lock()
			l.Unlock()
			unlock := g.mu.Unlock
			g.mu.Lock()
			unlock()
			func() {
				g.mu.Lock()
				defer g.mu.Unlock()
			}()
			g.rw.Lock()
			g.rw.Unlock()
			g.rw.RLock()
			g.rw.RLock()
			g.rw.RUnlock()
			g.rw.RUnlock()
			rl := g.rw.RLocker()
			rl.Lock()
			rl.Unlock()
		}))
		// Readers and writers on many goroutines.
		n, k := 0, 0
		fmt.Fprintln(w, race(8, 2000, func(gr, i int) {
			if i%4 == 0 {
				g.rw.Lock()
				n++
				g.rw.Unlock()
				return
			}
			g.rw.RLock()
			_ = n
			g.rw.RUnlock()
			g.mu.Lock()
			k++
			g.mu.Unlock()
		}))
	case "/unlock":
		var g guarded
		var l sync.Locker = &g.mu
		fmt.Fprintln(w, try(func() { g.mu.Unlock() }))
		fmt.Fprintln(w, try(func() { l.Unlock() }))
		fmt.Fprintln(w, try(func() { f := g.mu.Unlock; f() }))
		fmt.Fprintln(w, try(func() { defer g.mu.Unlock() }))
		fmt.Fprintln(w, try(func() { g.rw.Unlock() }))
		fmt.Fprintln(w, try(func() { g.rw.RUnlock() }))
		fmt.Fprintln(w, try(func() { g.rw.RLock(); g.rw.Unlock() }))
		g.rw.RUnlock()
		fmt.Fprintln(w, try(func() { g.rw.Lock(); g.rw.RUnlock() }))
		g.rw.Unlock()
		// The locks still work after the refused calls.
		fmt.Fprintln(w, try(func() { g.mu.Lock(); g.mu.Unlock(); g.rw.Lock(); g.rw.Unlock() }))
	case "/cond":
		// Wait unlocks the lock of the Cond first: a lock that is not held.
		var rw sync.RWMutex
		for _, l := range []sync.Locker{&sync.Mutex{}, &rw, rw.RLocker()} {
			c := sync.NewCond(l)
			fmt.Fprintln(w, try(func() { c.Wait() }))
		}
		fmt.Fprintln(w, try(func() { rw.RLocker().Unlock() }))
		// The correct uses: Wait with the lock held, the RLocker.
		var mu sync.Mutex
		c := sync.NewCond(&mu)
		ready := false
		fmt.Fprintln(w, try(func() {
			go func() { mu.Lock(); ready = true; c.Broadcast(); mu.Unlock() }()
			mu.Lock()
			for !ready {
				c.Wait()
			}
			mu.Unlock()
			rl := rw.RLocker()
			rl.Lock()
			rl.Unlock()
			rc := sync.NewCond(rl)
			done := false
			go func() { rw.Lock(); done = true; rw.Unlock(); rc.Broadcast() }()
			rl.Lock()
			for !done {
				rc.Wait()
			}
			rl.Unlock()
		}))
	case "/nilfunc":
		// Native code starts these funcs later on a goroutine of its own.
		var f func()
		fmt.Fprintln(w, try(func() { time.AfterFunc(time.Millisecond, f) }))
		ctx, cancel := context.WithCancel(context.Background())
		fmt.Fprintln(w, try(func() { context.AfterFunc(ctx, f) }))
		cancel()
		var wg sync.WaitGroup
		fmt.Fprintln(w, try(func() { wg.Go(f); wg.Wait() }))
		srv := &http.Server{}
		fmt.Fprintln(w, try(func() { srv.RegisterOnShutdown(f); srv.Shutdown(context.Background()) }))
		time.Sleep(20 * time.Millisecond) // a timer would fire here
		// Funcs that are not nil.
		done := make(chan bool, 3)
		fmt.Fprintln(w, try(func() {
			time.AfterFunc(time.Millisecond, func() { done <- true })
			ctx, cancel := context.WithCancel(context.Background())
			context.AfterFunc(ctx, func() { done <- true })
			cancel()
			wg.Go(func() { done <- true })
			wg.Wait()
			<-done
			<-done
			<-done
		}))
	case "/finalizer":
		x := &pair{}
		var n int
		for _, c := range []func(){
			func() { runtime.SetFinalizer(nil, nil) },
			func() { runtime.SetFinalizer(pair{}, nil) },
			func() { runtime.SetFinalizer(n, nil) },
			func() { runtime.SetFinalizer((*pair)(nil), nil) },
			func() { runtime.SetFinalizer(&x.b, func(*int) {}) },
			func() { runtime.SetFinalizer(x, 3) },
			func() { runtime.SetFinalizer(x, func(int) {}) },
			func() { runtime.SetFinalizer(x, func(*pair, int) {}) },
			func() { runtime.SetFinalizer(x, func(...*pair) {}) },
			func() { runtime.SetFinalizer(x, func(*pair) {}); runtime.SetFinalizer(x, func(*pair) {}) },
			// The correct uses (as in Go): the first field, the first
			// element, an interface parameter, a reset, a
			// finalizer that sets itself again.
			func() { runtime.SetFinalizer(&x.a, nil) },
			func() { s := make([]*int, 4); runtime.SetFinalizer(&s[0], func(**int) {}) },
			func() { runtime.SetFinalizer(&pair{}, func(any) {}) },
			func() {
				y := &pair{}
				runtime.SetFinalizer(y, func(*pair) {})
				runtime.SetFinalizer(y, nil)
				runtime.SetFinalizer(y, func(*pair) {})
			},
			func() {
				// A finalizer that sets itself again once: it runs twice.
				runs := make(chan int, 2)
				n := 0
				var again func(*pair)
				again = func(p *pair) {
					n++
					runs <- n
					if n == 1 {
						runtime.SetFinalizer(p, again)
					}
				}
				runtime.SetFinalizer(&pair{}, again)
				for got := 0; got < 2; {
					runtime.GC()
					select {
					case <-runs:
						got++
					case <-time.After(10 * time.Millisecond):
					}
				}
			},
		} {
			fmt.Fprintln(w, try(c))
		}
	case "/finalizerrace":
		// Goroutines set and clear the finalizers of shared objects. Two of
		// them can set a finalizer on the same object at the same time: one
		// gets "finalizer already set" (recovered here), and the runtime
		// never sees a second finalizer.
		ps := make([]*pair, 16)
		for i := range ps {
			ps[i] = &pair{}
		}
		fmt.Fprintln(w, race(8, 2000, func(g, i int) {
			p := ps[(g+i)%len(ps)]
			try(func() { runtime.SetFinalizer(p, func(*pair) {}) })
			try(func() { runtime.SetFinalizer(p, nil) })
		}))
	case "/mapwrites":
		m := map[int]int{}
		fmt.Fprintln(w, race(8, 200000, func(g, i int) { m[g*1000000+i] = i; delete(m, g*1000000+i-1) }))
	case "/mapreadwrite":
		m := map[int]int{1: 1}
		fmt.Fprintln(w, race(8, 200000, func(g, i int) {
			if g == 0 {
				m[i] = i
				return
			}
			_ = m[i]
		}))
	case "/maprange":
		m := map[int]int{}
		for i := 0; i < 100; i++ {
			m[i] = i
		}
		fmt.Fprintln(w, race(4, 20000, func(g, i int) {
			if g == 0 {
				m[i%100] = i
				return
			}
			for range m {
			}
		}))
	case "/mapok":
		// A map written and read under a lock, and written during its own
		// range (allowed by the spec): no false panic.
		var mu sync.Mutex
		m := map[int]int{}
		res := race(8, 5000, func(g, i int) {
			mu.Lock()
			m[g*10000+i] = i
			_ = m[i]
			mu.Unlock()
		})
		k := map[int]int{1: 1, 2: 2, 3: 3}
		res += " " + try(func() {
			for key := range k {
				delete(k, key)
				k[key+100] = key
			}
		})
		fmt.Fprintln(w, res, len(m))
	}
}
