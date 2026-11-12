// Package budgetplug checks the budget of the calls from the host, shared
// with the goroutines they start, the goroutine cap, the background
// goroutines and the cancellation by the context of the call (budget.go).
package budgetplug

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

var (
	sleepTicks  atomic.Int64
	tickerTicks atomic.Int64
	spawned     atomic.Int64
	iters       atomic.Int64
	timerIters  atomic.Int64
	serverIters atomic.Int64
	quit        = make(chan struct{})
	bgStop      = make(chan struct{})
	spinGo      = make(chan struct{})
	mu          sync.Mutex
	setOnce     sync.Once
	half, full  atomic.Int64
	rateRounds  atomic.Int64
	rateStop    = make(chan struct{})
	cbLive      atomic.Int64
	cbMax       atomic.Int64
	cbRan       atomic.Int64
)

type handler struct{}

// New starts two background goroutines for the name "bg": a loop that
// sleeps and a ticker loop, as cache janitors and refreshers do. /bgstop
// stops them: running on, they allocated during the next tests, and the
// tests that count allocations failed.
func New(_ context.Context, next http.Handler, _ *Config, name string) (http.Handler, error) {
	if name == "bg" {
		go func() {
			for {
				select {
				case <-bgStop:
					return
				default:
				}
				sleepTicks.Add(1)
				time.Sleep(time.Microsecond)
			}
		}()
		go func() {
			t := time.NewTicker(time.Microsecond)
			defer t.Stop()
			for {
				select {
				case <-bgStop:
					return
				case <-t.C:
					tickerTicks.Add(1)
				}
			}
		}()
	}
	if name == "fs" {
		// A native handler that calls plugin code many times: each
		// Open, Stat, Seek and Read is a new run of interpreted code.
		return http.FileServer(http.FS(slowFS{})), nil
	}
	if name == "strip" {
		// A native handler of net/http around the handler of the plugin: the
		// plugin value is boxed during this call (New).
		return http.StripPrefix("/p", handler{}), nil
	}
	if name == "next" {
		// Calls the next handler: a call from the host inside this one.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		}), nil
	}
	return handler{}, nil
}

// slowFS holds one file of 40 bytes. Each Read does 900000 steps of work
// and returns one byte.
type slowFS struct{}

func (slowFS) Open(name string) (fs.File, error) {
	if name != "f.txt" {
		return nil, fs.ErrNotExist
	}
	return &slowFile{}, nil
}

type slowFile struct{ off int64 }

const slowSize = 40

func (f *slowFile) Stat() (fs.FileInfo, error) { return slowInfo{}, nil }
func (f *slowFile) Close() error               { return nil }

func (f *slowFile) Read(p []byte) (int, error) {
	work(900000)
	if f.off >= slowSize {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = 'a'
	f.off++
	return 1, nil
}

func (f *slowFile) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.off = off
	case io.SeekCurrent:
		f.off += off
	case io.SeekEnd:
		f.off = slowSize + off
	}
	return f.off, nil
}

type slowInfo struct{}

func (slowInfo) Name() string       { return "f.txt" }
func (slowInfo) Size() int64        { return slowSize }
func (slowInfo) Mode() fs.FileMode  { return 0o444 }
func (slowInfo) ModTime() time.Time { return time.Time{} }
func (slowInfo) IsDir() bool        { return false }
func (slowInfo) Sys() any           { return nil }

// work runs n steps. The sum is an int64: work(100000) overflows a 32-bit
// int.
func work(n int) int64 {
	var s int64
	for i := 0; i < n; i++ {
		s += int64(i)
	}
	return s
}

// pullChain runs d nested iter.Pull sequences: each level does 100000
// steps of work in its own goroutine.
func pullChain(d int) int64 {
	if d == 0 {
		return work(100000)
	}
	next, stop := iter.Pull(func(yield func(int64) bool) { yield(pullChain(d - 1)) })
	defer stop()
	v, _ := next()
	return v + work(100000)
}

// pullRec recurses through iter.Pull: each level runs on the goroutine of
// a new sequence, while the level above waits in next.
func pullRec(d int) int {
	if d == 0 {
		return 0
	}
	next, stop := iter.Pull(func(yield func(int) bool) { yield(pullRec(d-1) + 1) })
	defer stop()
	v, _ := next()
	return v
}

// pullMu is locked by /pullblock and unlocked by /pullunblock: the
// sequence of /pullblock waits for it in a native call, which no
// cancellation stops.
var pullMu sync.Mutex

type spinHandler struct{}

// gotoCount is the counter of /gotoboolspin.
var gotoCount int

func (spinHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for j := 0; j < 200000; j++ {
		serverIters.Add(1)
	}
}

func (handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/ticks":
		fmt.Fprint(w, sleepTicks.Load(), " ", tickerTicks.Load())
	case "/bomb":
		// One goroutine per loop, each blocked until /quit.
		for {
			go func() { <-quit }()
			spawned.Add(1)
		}
	case "/bombrecover":
		defer func() { fmt.Fprint(w, recover(), " ", spawned.Load()) }()
		for {
			go func() { <-quit }()
			spawned.Add(1)
		}
	case "/quit":
		close(quit)
	case "/bgstop":
		close(bgStop)
	case "/spread":
		// 20000 goroutines of 200000 steps each, all inside this call.
		done := make(chan bool)
		for i := 0; i < 20000; i++ {
			go func() {
				for j := 0; j < 200000; j++ {
					iters.Add(1)
				}
				done <- true
			}()
		}
		for i := 0; i < 20000; i++ {
			<-done
		}
	case "/ratestart":
		// 12 background goroutines that work 10000 steps, then sleep for a
		// microsecond, until /ratestop (Options.BackgroundRate).
		for i := 0; i < 12; i++ {
			go func() {
				for {
					select {
					case <-rateStop:
						return
					default:
					}
					work(10000)
					rateRounds.Add(1)
					time.Sleep(time.Microsecond)
				}
			}()
		}
	case "/timerscap":
		// 50 callbacks that the timer goroutines of the runtime run at the
		// same time, each for 100 ms (MaxGoroutines).
		for i := 0; i < 50; i++ {
			time.AfterFunc(0, func() {
				n := cbLive.Add(1)
				for {
					m := cbMax.Load()
					if n <= m || cbMax.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(100 * time.Millisecond)
				cbLive.Add(-1)
				cbRan.Add(1)
			})
		}
	case "/timerscapstat":
		fmt.Fprint(w, cbMax.Load(), " ", cbRan.Load())
	case "/raterounds":
		fmt.Fprint(w, rateRounds.Load())
	case "/ratestop":
		close(rateStop)
	case "/iters":
		fmt.Fprint(w, iters.Load())
	case "/timers":
		// 200 callbacks of 200000 steps each, that the timer goroutines of
		// the runtime run, while this call waits for them.
		done := make(chan bool)
		for i := 0; i < 200; i++ {
			time.AfterFunc(0, func() {
				for j := 0; j < 200000; j++ {
					timerIters.Add(1)
				}
				done <- true
			})
		}
		for i := 0; i < 200; i++ {
			<-done
		}
	case "/timeriters":
		fmt.Fprint(w, timerIters.Load())
	case "/server":
		// 20 requests to a server of the plugin, whose handler (a value
		// of a plugin type) does 200000 steps on a goroutine of net/http.
		srv := httptest.NewServer(spinHandler{})
		defer srv.Close()
		for i := 0; i < 20; i++ {
			if resp, err := http.Get(srv.URL); err == nil {
				resp.Body.Close()
			}
		}
	case "/serveriters":
		fmt.Fprint(w, serverIters.Load())
	case "/pull":
		fmt.Fprint(w, pullChain(20))
	case "/pullsmall":
		fmt.Fprint(w, pullChain(3))
	case "/pullrec":
		fmt.Fprint(w, pullRec(200))
	case "/pullblock":
		pullMu.Lock()
		next, _ := iter.Pull(func(yield func(int) bool) {
			pullMu.Lock()
			pullMu.Unlock()
			yield(1)
		})
		next()
	case "/pullunblock":
		pullMu.Unlock()
	case "/spin":
		for {
		}
	case "/gotospin":
		// A loop whose back edge is a conditional branch, with no call
		// and no unconditional jump: the budget must count it too.
		i := 0
	loop:
		i++
		if i > 0 {
			goto loop
		}
	case "/gotoboolspin":
		// The same with a bool variable as the condition, and a counter in
		// memory: the loop has no phi, so its back edge is the OpIf itself,
		// not a compare-and-jump instruction or a jump of an edge stub.
		x := r.URL.Path != ""
	boolLoop:
		gotoCount++
		if x {
			goto boolLoop
		}
	case "/sleep":
		time.Sleep(time.Hour)
	case "/block":
		<-quit
	case "/rselect":
		// reflect.Select waits like a select statement
		reflect.Select([]reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(quit)}})
	case "/lock":
		// The deferred Unlock must run when the call is canceled.
		mu.Lock()
		defer mu.Unlock()
		for {
		}
	case "/lockwork":
		// A deferred cleanup of about 10000 steps before the Unlock: it
		// must run to the end when the call is canceled.
		mu.Lock()
		defer func() {
			work(10000)
			mu.Unlock()
		}()
		for {
		}
	case "/onceset":
		// A sync.Once whose function sleeps between two writes. Without
		// Options.Cancel it runs to its end when the client goes away, as
		// in Go: stopped in the middle, the Once stayed done with full 0.
		setOnce.Do(func() {
			half.Store(1)
			time.Sleep(100 * time.Millisecond)
			work(200000)
			full.Store(1)
		})
		fmt.Fprint(w, half.Load(), " ", full.Load())
	case "/locked":
		if mu.TryLock() {
			mu.Unlock()
			fmt.Fprint(w, "free")
		} else {
			fmt.Fprint(w, "locked")
		}
	case "/bgspin":
		// A goroutine that never blocks once /bgspingo lets it go, after
		// this call returned (before, it would use the budget of this
		// call).
		go func() {
			<-spinGo
			for {
			}
		}()
	case "/bgspingo":
		close(spinGo)
	}
}
