package vm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Tests of the deferred calls of a goroutine that stops with ErrBudget or
// ErrStackOverflow: they run with the grace of a cancellation (cancelGrace
// steps, then a hard stop), so a deferred Unlock, a deferred wg.Done or a
// deferred flag reset still runs. Before, these errors skipped every
// deferred call: a plugin that ran over its budget under a lock wedged all
// the later calls that took the lock.

const faultDeferPlugin = `package p

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
)

var (
	mu   sync.Mutex
	m    = map[int]int{}
	busy sync.Mutex
	flag bool
	n    int
)

// Spin runs over any budget with mu locked.
func Spin() {
	mu.Lock()
	defer mu.Unlock()
	for {
		m[1]++
	}
}

// Free reports whether mu is free.
func Free() bool {
	if mu.TryLock() {
		mu.Unlock()
		return true
	}
	return false
}

func rec(d int) int { return rec(d+1) + 1 }

// Deep overflows the stack with busy locked and flag set.
func Deep() {
	busy.Lock()
	flag = true
	defer func() { flag = false; busy.Unlock() }()
	rec(0)
}

// Idle reports whether busy is free and flag is reset.
func Idle() bool {
	if busy.TryLock() {
		busy.Unlock()
		return !flag
	}
	return false
}

// SpinGo waits for a goroutine that runs over the budget and has a
// deferred wg.Done.
func SpinGo() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			m[2]++
		}
	}()
	wg.Wait()
}

// Hostile runs over the budget, and its deferred call loops for ever.
func Hostile() {
	defer func() {
		for {
			n++
		}
	}()
	for {
		n++
	}
}

// HostileRecover tries to stop ErrBudget with recover, then loops for ever.
func HostileRecover() (s string) {
	defer func() {
		recover()
		s = "recovered"
	}()
	for {
		n++
	}
}

var cleaned bool

// SortExit calls runtime.Goexit in the less func of sort.Slice.
func SortExit() {
	cleaned = false
	defer func() { cleaned = true }()
	s := []int{3, 1, 2}
	sort.Slice(s, func(i, j int) bool {
		runtime.Goexit()
		return s[i] < s[j]
	})
}

// Cleaned reports whether the deferred call of SortExit or ErrorExit ran.
func Cleaned() bool { return cleaned }

type exitErr struct{}

func (exitErr) Error() string {
	runtime.Goexit()
	return "never"
}

// ErrorExit gives fmt an error whose Error method calls runtime.Goexit:
// fmt recovers the panics of Error methods.
func ErrorExit() string {
	cleaned = false
	defer func() { cleaned = true }()
	return fmt.Errorf("wrap: %w", error(exitErr{})).Error()
}

var _ = errors.New

// DeepDefers overflows the stack, and each frame has a deferred call that
// overflows the stack again.
func DeepDefers(d int) int {
	defer func() { rec(0) }()
	return DeepDefers(d+1) + 1
}
`

func loadFaultDefer(t *testing.T, opts Options) *Plugin {
	t.Helper()
	needSource(t)
	dir, pkg := writePlugin(t, faultDeferPlugin)
	opts.Dir = dir
	p, err := Load(opts, pkg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// callTimeout calls name with a deadline and fails the test when the call
// does not return.
func callTimeout(t *testing.T, p *Plugin, name string, args ...any) ([]*Object, error) {
	t.Helper()
	type result struct {
		res []*Object
		err error
	}
	done := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		res, err := p.Call(ctx, name, args...)
		done <- result{res, err}
	}()
	select {
	case r := <-done:
		return r.res, r.err
	case <-time.After(30 * time.Second):
		t.Fatalf("%s: still running", name)
		return nil, nil
	}
}

func callBool(t *testing.T, p *Plugin, name string) bool {
	t.Helper()
	res, err := callTimeout(t, p, name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	v, err := res[0].Interface()
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return v.(bool)
}

// TestBudgetDeferUnlock: a call that runs over its budget with a mutex
// locked unlocks it in its deferred call. Before, the next call that took
// the mutex hung for ever, even with a deadline.
func TestBudgetDeferUnlock(t *testing.T) {
	p := loadFaultDefer(t, Options{Budget: 100000, Cancel: true})
	for i := 0; i < 2; i++ {
		if _, err := callTimeout(t, p, "Spin"); !errors.Is(err, ErrBudget) {
			t.Fatalf("Spin %d: %v, want ErrBudget", i, err)
		}
		if !callBool(t, p, "Free") {
			t.Fatalf("after Spin %d: the mutex is still locked", i)
		}
	}
}

// TestOverflowDeferReset: after a stack overflow, the deferred calls of the
// frames below run: the mutex is unlocked and the flag is reset.
func TestOverflowDeferReset(t *testing.T) {
	p := loadFaultDefer(t, Options{})
	p.M.MaxDepth = 2000
	for i := 0; i < 2; i++ {
		if _, err := callTimeout(t, p, "Deep"); !errors.Is(err, ErrStackOverflow) {
			t.Fatalf("Deep %d: %v, want ErrStackOverflow", i, err)
		}
		if !callBool(t, p, "Idle") {
			t.Fatalf("after Deep %d: the deferred reset did not run", i)
		}
	}
}

// TestBudgetDeferWaitGroup: a goroutine that runs over the budget of the
// call runs its deferred wg.Done, so the call returns. Before, wg.Wait hung
// for ever. The goroutine calls GoPanic after wg.Done, so the test waits
// for its panic on a channel (a plain variable is a data race).
func TestBudgetDeferWaitGroup(t *testing.T) {
	got := make(chan any, 1)
	p := loadFaultDefer(t, Options{Budget: 100000, Cancel: true, GoPanic: func(r any) {
		select {
		case got <- r:
		default:
		}
	}})
	if _, err := callTimeout(t, p, "SpinGo"); err != nil && !errors.Is(err, ErrBudget) {
		t.Fatalf("SpinGo: %v", err)
	}
	select {
	case r := <-got:
		if r != ErrBudget {
			t.Fatalf("goroutine panic %v, want ErrBudget", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the goroutine did not stop")
	}
}

// TestBudgetDeferHostile: a deferred call that loops for ever after
// ErrBudget stops once the grace is used, and recover cannot stop ErrBudget.
func TestBudgetDeferHostile(t *testing.T) {
	p := loadFaultDefer(t, Options{Budget: 100000})
	if _, err := callTimeout(t, p, "Hostile"); !errors.Is(err, ErrBudget) {
		t.Fatalf("Hostile: %v, want ErrBudget", err)
	}
	if res, err := callTimeout(t, p, "HostileRecover"); !errors.Is(err, ErrBudget) {
		t.Fatalf("HostileRecover: %v %v, want ErrBudget", res, err)
	}
}

// TestOverflowDeferHostile: deferred calls that overflow the stack again
// during the unwinding of an overflow stop within the grace.
func TestOverflowDeferHostile(t *testing.T) {
	p := loadFaultDefer(t, Options{})
	p.M.MaxDepth = 2000
	if _, err := callTimeout(t, p, "DeepDefers", 0); !errors.Is(err, ErrStackOverflow) {
		t.Fatalf("DeepDefers: %v, want ErrStackOverflow", err)
	}
}

// TestGoexitNativeCallbackHost: runtime.Goexit in a callback of native
// code (the less func of sort.Slice, an Error method that fmt calls) under
// Plugin.Call returns ErrGoexit after the deferred calls ran. Before, the
// VM called the real runtime.Goexit: it ended the goroutine of the host
// and Plugin.Call never returned.
func TestGoexitNativeCallbackHost(t *testing.T) {
	p := loadFaultDefer(t, Options{})
	for _, name := range []string{"SortExit", "ErrorExit"} {
		if res, err := callTimeout(t, p, name); !errors.Is(err, ErrGoexit) {
			t.Fatalf("%s: %v %v, want ErrGoexit", name, res, err)
		}
		if !callBool(t, p, "Cleaned") {
			t.Fatalf("%s: the deferred call did not run", name)
		}
	}
}

const goChainPlugin = `package p

import (
	"sync"
	"sync/atomic"
)

var (
	gate  sync.WaitGroup
	count int64
)

func chain() {
	defer func() {
		for i := 0; i < 100; i++ {
			atomic.AddInt64(&count, 1)
		}
		go chain()
	}()
	for {
	}
}

// Attack starts a chain of goroutines, each started in a deferred call of
// the one before, and waits in native code (gate.Wait) in its own
// deferred call once it is stopped: the call does not end, so its account
// stays spent or canceled.
func Attack() {
	gate.Add(1)
	defer gate.Wait()
	go chain()
	for {
	}
}

// Release lets Attack return.
func Release() { gate.Done() }

// Count returns the work done by the deferred calls of the chain.
func Count() int64 { return atomic.LoadInt64(&count) }
`

// testGoChain runs Attack with ctx and checks that the chain of goroutines
// stops: a goroutine whose account is spent or canceled, or that stops with
// a fault, starts no goroutine. Before, each goroutine of the chain got a
// grace of its own and the chain ran for ever.
func testGoChain(t *testing.T, opts Options, ctx context.Context, want error) {
	t.Helper()
	testChain(t, goChainPlugin, opts, ctx, want)
}

// testChain runs Attack of the plugin src with ctx, checks that the work of
// its chain (Count) stops, and that Attack returns want after Release.
func testChain(t *testing.T, src string, opts Options, ctx context.Context, want error) {
	t.Helper()
	needSource(t)
	dir, pkg := writePlugin(t, src)
	opts.Dir = dir
	opts.GoPanic = func(any) {}
	p, err := Load(opts, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	done := make(chan error, 1)
	go func() {
		_, err := p.Call(ctx, "Attack")
		done <- err
	}()
	count := func() int64 {
		res, err := callTimeout(t, p, "Count")
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		v, err := res[0].Interface()
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		return v.(int64)
	}
	// Wait until the first link has run its deferred call (the count is
	// not 0) and the count stays the same for 500 ms. The first link can
	// take more than a second to stop on a slow runner (a stack overflow of
	// rec takes time), so one fixed wait is not enough. A chain that runs
	// for ever adds 100 per link: it goes over 1000 or keeps changing until
	// the deadline.
	deadline := time.Now().Add(time.Minute)
	c1 := count()
	for {
		time.Sleep(500 * time.Millisecond)
		c2 := count()
		late := time.Now().After(deadline)
		if c2 > 1000 || c2 != c1 && late {
			t.Fatalf("the chain still runs: count %d then %d, %d goroutines", c1, c2, p.M.Goroutines())
		}
		if c2 == c1 && (c2 > 0 || late) {
			break
		}
		c1 = c2
	}
	// No link starts later.
	time.Sleep(time.Second)
	if c2 := count(); c2 != c1 {
		t.Fatalf("the chain still runs: count %d then %d, %d goroutines", c1, c2, p.M.Goroutines())
	}
	if _, err := callTimeout(t, p, "Release"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Attack: %v, want %v", err, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Attack: still running")
	}
}

// TestBudgetGoChain: the chain of goroutines stops after ErrBudget, also
// with BackgroundRate.
func TestBudgetGoChain(t *testing.T) {
	testGoChain(t, Options{Budget: 100000, MaxGoroutines: 10, BackgroundRate: 100000}, context.Background(), ErrBudget)
}

// TestCancelGoChain: the chain of goroutines stops after ErrCanceled.
func TestCancelGoChain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	testGoChain(t, Options{Cancel: true, MaxGoroutines: 10}, ctx, ErrCanceled)
}

// afterFuncChainPlugin is goChainPlugin with callbacks of native code in
// place of go statements: START starts chain on a goroutine of native code
// (time.AfterFunc, context.AfterFunc), and LOOP is the endless work of each
// link.
const afterFuncChainPlugin = `package p

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

var (
	gate  sync.WaitGroup
	count int64

	_ = context.Background
	_ = time.Now
)

func start() { START }

func rec(n int) int { return rec(n+1) + 1 }

func chain() {
	defer func() {
		for i := 0; i < 100; i++ {
			atomic.AddInt64(&count, 1)
		}
		start()
	}()
	LOOP
}

func Attack() {
	gate.Add(1)
	defer gate.Wait()
	start()
	LOOP
}

func Release() { gate.Done() }

func Count() int64 { return atomic.LoadInt64(&count) }
`

var afterFuncStarts = map[string]string{
	"time":    `time.AfterFunc(0, chain)`,
	"context": `ctx, cancel := context.WithCancel(context.Background()); context.AfterFunc(ctx, chain); cancel()`,
}

// testAfterFuncChain runs the chain of afterFuncChainPlugin with each kind
// of callback. A callback of native code whose account is spent or
// canceled, or that a goroutine with a fault gave to native code, does not
// run: before, it took the stopped account, stopped at its first refill
// and got a grace of its own for its deferred calls, which started the
// next callback, for ever.
func testAfterFuncChain(t *testing.T, loop string, opts Options, timeout time.Duration, want error) {
	for name, start := range afterFuncStarts {
		t.Run(name, func(t *testing.T) {
			src := strings.NewReplacer("START", start, "LOOP", loop).Replace(afterFuncChainPlugin)
			ctx, cancel := context.WithCancel(context.Background())
			if timeout > 0 {
				ctx, cancel = context.WithTimeout(context.Background(), timeout)
			}
			defer cancel()
			testChain(t, src, opts, ctx, want)
		})
	}
}

// TestBudgetAfterFuncChain: the chain of callbacks stops after ErrBudget,
// also with BackgroundRate.
func TestBudgetAfterFuncChain(t *testing.T) {
	testAfterFuncChain(t, "for {\n\t}", Options{Budget: 100000, MaxGoroutines: 10, BackgroundRate: 100000}, 0, ErrBudget)
}

// TestCancelAfterFuncChain: the chain of callbacks stops after ErrCanceled.
func TestCancelAfterFuncChain(t *testing.T) {
	testAfterFuncChain(t, "for {\n\t}", Options{Cancel: true, MaxGoroutines: 10}, 100*time.Millisecond, ErrCanceled)
}

// TestStackAfterFuncChain: the chain of callbacks stops after
// ErrStackOverflow, while the account of the call is still open: the
// callback that the deferred call of the fault gives to native code does
// not run.
func TestStackAfterFuncChain(t *testing.T) {
	testAfterFuncChain(t, "rec(0)", Options{Cancel: true, MaxGoroutines: 10}, 0, ErrStackOverflow)
}

const nestFaultPlugin = `package p

import (
	"context"
	"fmt"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

func New(_ context.Context, next http.Handler, _ *Config, name string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if name == "a" {
			// The next handler runs in a deferred call of the cleanup
			// of ErrBudget.
			defer func() { next.ServeHTTP(w, r) }()
			for {
			}
		}
		defer fmt.Fprint(w, "b-defer")
		for {
		}
	}), nil
}
`

// TestBudgetDeferNextHandler: the next handler (plugin B) that a deferred
// call of plugin A calls during the cleanup of ErrBudget is a new call from
// the host: it has its own budget and runs its own deferred calls when it
// goes over it. Before, B ran on the grace of A and then stopped with the
// ErrBudget of A, without its deferred calls.
func TestBudgetDeferNextHandler(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, nestFaultPlugin)
	p, err := Load(Options{Dir: dir, Budget: 100000}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	var h http.Handler = http.NotFoundHandler()
	for _, name := range []string{"b", "a"} {
		cfg, err := createConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		if h, err = newHandler(context.Background(), p, h, cfg, name); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if r := recover(); r != ErrBudget {
				t.Errorf("panic %v, want %v", r, ErrBudget)
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	}()
	if body := rec.Body.String(); body != "b-defer" {
		t.Errorf("body %q, want %q: the deferred call of the next handler did not run", body, "b-defer")
	}
}
