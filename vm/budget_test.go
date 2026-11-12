package vm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests of the budget shared by a call from the host and its goroutines,
// of the goroutine cap, of the background rule and of the cancellation by
// the context of the call (budget.go). Plugin: testdata/restricted/budgetplug.

const budgetPlug = "github.com/traefik/yaegi/v2/vm/testdata/restricted/budgetplug"

// loadBudgetPlug loads budgetplug and returns the handler that New returns
// for name.
func loadBudgetPlug(t testing.TB, opts Options, name string) (*Plugin, http.Handler) {
	t.Helper()
	opts.Dir = ".."
	p, err := Load(opts, budgetPlug)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, name)
	if err != nil {
		t.Fatal(err)
	}
	return p, h
}

// serveCtx calls h for path with the context ctx and returns the body and
// the panic of ServeHTTP.
func serveCtx(h http.Handler, ctx context.Context, path string) (body string, r any) {
	rec := httptest.NewRecorder()
	defer func() { body, r = rec.Body.String(), recover() }()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil).WithContext(ctx))
	return
}

func serveBG(h http.Handler, path string) (string, any) {
	return serveCtx(h, context.Background(), path)
}

// waitFor polls cond for up to d.
func waitFor(d time.Duration, cond func() bool) bool {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return true
		}
	}
	return cond()
}

// TestBudgetBackgroundTicker: a goroutine started in New
// that sleeps in a loop, and a ticker loop, run past three times the budget.
// Before, the budget was a total per goroutine: both died after exactly
// Budget rounds.
func TestBudgetBackgroundTicker(t *testing.T) {
	const budget = 2000
	var panics atomic.Value
	p, h := loadBudgetPlug(t, Options{Budget: budget, GoPanic: func(r any) { panics.Store(r) }}, "bg")
	// Stop the two goroutines at the end: running on, they allocated
	// during the next tests (TestFrameLocalAllocs counts allocations).
	defer func() {
		serveBG(h, "/bgstop")
		if !waitFor(30*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
			t.Errorf("%d live goroutines after /bgstop", p.M.Goroutines())
		}
	}()
	ok := waitFor(60*time.Second, func() bool {
		body, _ := serveBG(h, "/ticks")
		f := strings.Fields(body)
		if len(f) != 2 {
			return false
		}
		s, _ := strconv.Atoi(f[0])
		k, _ := strconv.Atoi(f[1])
		return s > 3*budget && k > 3*budget || panics.Load() != nil
	})
	if r := panics.Load(); r != nil {
		t.Fatalf("background goroutine stopped: %v", r)
	}
	if !ok {
		body, _ := serveBG(h, "/ticks")
		t.Fatalf("ticks %s, want more than %d each", body, 3*budget)
	}
}

// TestBudgetBackgroundSpin: a goroutine without any blocking point still
// stops with ErrBudget after its call returned.
func TestBudgetBackgroundSpin(t *testing.T) {
	panics := make(chan any, 1)
	_, h := loadBudgetPlug(t, Options{Budget: 1e5, GoPanic: func(r any) { panics <- r }}, "x")
	for _, path := range []string{"/bgspin", "/bgspingo"} {
		if _, r := serveBG(h, path); r != nil {
			t.Fatal(path, r)
		}
	}
	select {
	case r := <-panics:
		if r != ErrBudget {
			t.Fatalf("goroutine panic %v, want %v", r, ErrBudget)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the goroutine still runs")
	}
}

// TestBudgetBackwardBranch: a loop whose back edge is a conditional branch
// (a goto under an if, with no call) is counted by the budget like a for
// loop. Before, only OpJmp counted, and such a loop ran for ever under
// a budget.
func TestBudgetBackwardBranch(t *testing.T) {
	_, h := loadBudgetPlug(t, Options{Budget: 1e6}, "x")
	done := make(chan any, 1)
	go func() {
		_, r := serveBG(h, "/gotospin")
		done <- r
	}()
	select {
	case r := <-done:
		if r != ErrBudget {
			t.Fatalf("panic %v, want %v", r, ErrBudget)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the loop still runs: the budget does not count its back edge")
	}
}

// TestBudgetBackwardIf: the same with a bool variable as the condition
// (loop: n++; if x { goto loop }). Its back edge is an OpIf, where
// TestBudgetBackwardBranch only runs a compare-and-jump instruction: a
// change that removed the budget step of a backward OpIf was not caught.
func TestBudgetBackwardIf(t *testing.T) {
	_, h := loadBudgetPlug(t, Options{Budget: 1e6}, "x")
	done := make(chan any, 1)
	go func() {
		_, r := serveBG(h, "/gotoboolspin")
		done <- r
	}()
	select {
	case r := <-done:
		if r != ErrBudget {
			t.Fatalf("panic %v, want %v", r, ErrBudget)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the loop still runs: the budget does not count a backward OpIf")
	}
}

// TestBudgetGoroutineCap: `for { go f() }` stops at
// MaxGoroutines with a panic that the plugin can recover. Before, 150000
// goroutines in 3 s.
func TestBudgetGoroutineCap(t *testing.T) {
	const max = 1000
	p, h := loadBudgetPlug(t, Options{Budget: 5e7, MaxGoroutines: max}, "x")
	body, r := serveBG(h, "/bombrecover")
	if r != nil || body != ErrTooManyGoroutines.Error()+" "+strconv.Itoa(max) {
		t.Fatalf("/bombrecover: body %q, panic %v; want %q", body, r, ErrTooManyGoroutines.Error()+" "+strconv.Itoa(max))
	}
	if n := p.M.Goroutines(); n != max {
		t.Fatalf("%d live goroutines, want %d", n, max)
	}
	// Without a recover, the host gets the panic.
	if _, r := serveBG(h, "/bomb"); r != ErrTooManyGoroutines {
		t.Fatalf("/bomb: panic %v, want %v", r, ErrTooManyGoroutines)
	}
	serveBG(h, "/quit")
	if !waitFor(30*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("%d live goroutines after /quit", p.M.Goroutines())
	}
}

// TestBudgetSharedGoroutines: a request that starts 20000
// goroutines of 200000 steps each runs at most Budget steps in total.
// Before, each goroutine had a full budget: 13.9 CPU seconds.
func TestBudgetSharedGoroutines(t *testing.T) {
	const budget = 1e6
	var gopanics atomic.Int64
	p, h := loadBudgetPlug(t, Options{Budget: budget, GoPanic: func(r any) {
		if r != ErrBudget {
			panic(r)
		}
		gopanics.Add(1)
	}}, "x")
	start := time.Now()
	if _, r := serveBG(h, "/spread"); r != ErrBudget {
		t.Fatalf("/spread: panic %v, want %v", r, ErrBudget)
	}
	if !waitFor(60*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("%d live goroutines", p.M.Goroutines())
	}
	body, _ := serveBG(h, "/iters")
	n, _ := strconv.Atoi(body)
	// One loop round is one step (its backward jump). A goroutine that
	// stops does one more round before its jump fails: at most budget
	// rounds plus one per goroutine.
	if n <= 0 || n > budget+20000 {
		t.Fatalf("%d loop rounds in all goroutines, want at most %d", n, int(budget)+20000)
	}
	t.Logf("%d rounds, %d goroutines stopped with ErrBudget, %v", n, gopanics.Load(), time.Since(start))
}

// TestBudgetNativeCallbacks: callbacks that native code runs on goroutines
// of its own (200 time.AfterFunc callbacks of 200000 steps each) while the
// call that made them runs share the budget of that call. Before, each
// callback was a new call with a full budget: 40M steps with a Budget of
// 1e6.
func TestBudgetNativeCallbacks(t *testing.T) {
	const budget = 1e6
	_, h := loadBudgetPlug(t, Options{Budget: budget, GoPanic: func(r any) {
		if r != ErrBudget {
			t.Errorf("GoPanic got %v, want %v", r, ErrBudget)
		}
	}}, "x")
	if _, r := serveBG(h, "/timers"); r != ErrBudget {
		t.Fatalf("/timers: panic %v, want %v", r, ErrBudget)
	}
	var n int
	waitFor(5*time.Second, func() bool {
		body, _ := serveBG(h, "/timeriters")
		m, _ := strconv.Atoi(body)
		stable := m == n
		n = m
		return stable && n > 0
	})
	// At most the budget, plus one round per callback that stops.
	if n <= 0 || n > budget+200 {
		t.Fatalf("%d loop rounds in all the callbacks, want at most %d", n, int(budget)+200)
	}
}

// TestBudgetNativeServerHandler: the handler of a server that the plugin
// starts (a value of a plugin type, whose ServeHTTP net/http calls on its
// own goroutines) shares the budget of the call that gave it to net/http.
// Before, each request was a new call with a full budget: 20 requests of
// 200000 steps ran 4M steps with a Budget of 1e6.
func TestBudgetNativeServerHandler(t *testing.T) {
	const budget = 1e6
	_, h := loadBudgetPlug(t, Options{Budget: budget, GoPanic: func(any) {}}, "x")
	// The call itself may end before its next refill (it waits in
	// http.Get, which is not a blocking point): no panic is needed there.
	if _, r := serveBG(h, "/server"); r != nil && r != ErrBudget {
		t.Fatalf("/server: panic %v", r)
	}
	body, _ := serveBG(h, "/serveriters")
	n, _ := strconv.Atoi(body)
	if n <= 0 || n > budget+20 {
		t.Fatalf("%d loop rounds in the handler, want at most %d", n, int(budget)+20)
	}
}

// TestBudgetOriginInsideCall: a value of the plugin boxed during a call from
// the host (handler{} given to http.StripPrefix in New) keeps the account of
// that call (originAcct). When net/http calls it on a goroutine where
// another call from the host runs (the ServeHTTP of a request, with its
// pending account), the request keeps its own account: its budget and its
// context. Before, withOrigin replaced the account of the request by the
// closed account of New: the request ran in the background and never
// stopped on its context (Cancel) nor on the budget of the request.
func TestBudgetOriginInsideCall(t *testing.T) {
	_, h := loadBudgetPlug(t, Options{Budget: 1e8, Cancel: true}, "strip")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan any, 1)
	go func() {
		_, r := serveCtx(h, ctx, "/p/block")
		done <- r
	}()
	select {
	case r := <-done:
		if r != http.ErrAbortHandler {
			t.Fatalf("/p/block: panic %v, want %v", r, http.ErrAbortHandler)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("/p/block: still running after its context was done")
	}
	// The budget of the request too.
	_, h = loadBudgetPlug(t, Options{Budget: 1e6}, "strip")
	if _, r := serveBG(h, "/p/spin"); r != ErrBudget {
		t.Fatalf("/p/spin: panic %v, want %v", r, ErrBudget)
	}
}

// TestBudgetIterPull: nested iter.Pull sequences share the
// budget of the call, and count in MaxGoroutines. Before, each sequence ran
// on a goroutine with a new budget.
func TestBudgetIterPull(t *testing.T) {
	_, h := loadBudgetPlug(t, Options{Budget: 1e6}, "x")
	// 21 levels of 100000 steps: more than the budget.
	if _, r := serveBG(h, "/pull"); r != ErrBudget {
		t.Fatalf("/pull: panic %v, want %v", r, ErrBudget)
	}
	// 4 levels: under the budget.
	if body, r := serveBG(h, "/pullsmall"); r != nil || body != "19999800000" {
		t.Fatalf("/pullsmall: body %q, panic %v", body, r)
	}
	p, h := loadBudgetPlug(t, Options{MaxGoroutines: 5}, "x")
	if _, r := serveBG(h, "/pull"); r != ErrTooManyGoroutines {
		t.Fatalf("/pull with MaxGoroutines 5: panic %v, want %v", r, ErrTooManyGoroutines)
	}
	if !waitFor(10*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("%d live goroutines", p.M.Goroutines())
	}
}

// TestIterPullNest: the sequence of iter.Pull continues the
// depth and the nested runs of the caller of next. Before, each sequence
// started again at depth 0, so a recursion through iter.Pull never reached
// MaxNest nor MaxDepth.
func TestIterPullNest(t *testing.T) {
	p, h := loadBudgetPlug(t, Options{}, "x")
	if body, r := serveBG(h, "/pullrec"); r != nil || body != "200" {
		t.Fatalf("/pullrec: body %q, panic %v", body, r)
	}
	for _, lim := range []struct {
		name string
		set  func(m *Machine)
	}{
		{"MaxNest", func(m *Machine) { m.MaxNest = 50 }},
		{"MaxDepth", func(m *Machine) { m.MaxDepth = 300 }},
	} {
		p, h = loadBudgetPlug(t, Options{}, "x")
		lim.set(p.M)
		if _, r := serveBG(h, "/pullrec"); r != ErrStackOverflow {
			t.Errorf("/pullrec with %s: panic %v, want %v", lim.name, r, ErrStackOverflow)
		}
		if !waitFor(10*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
			t.Fatalf("%s: %d live goroutines", lim.name, p.M.Goroutines())
		}
	}
}

// TestIterPullCancel: next waits for a sequence that is
// blocked in a native call (sync.Mutex.Lock). With Options.Cancel, the end
// of the context of the request stops the wait: the hand-off between next
// and the sequence is a cancellation point. Before, the request never
// ended. The sequence ends when it is unblocked.
func TestIterPullCancel(t *testing.T) {
	panics := make(chan any, 1)
	p, h := loadBudgetPlug(t, Options{Cancel: true, GoPanic: func(r any) { panics <- r }}, "x")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan any, 1)
	go func() {
		_, r := serveCtx(h, ctx, "/pullblock")
		done <- r
	}()
	select {
	case r := <-done:
		if r != http.ErrAbortHandler {
			t.Errorf("/pullblock: panic %v, want %v", r, http.ErrAbortHandler)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("/pullblock still runs after the cancel")
	}
	if _, r := serveBG(h, "/pullunblock"); r != nil {
		t.Fatal(r)
	}
	if !waitFor(10*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("%d live goroutines", p.M.Goroutines())
	}
	select {
	case r := <-panics:
		t.Fatalf("GoPanic got %v, want nothing", r)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestBudgetCancel: with Options.Cancel, when the context
// of the request is done, a handler that spins, sleeps or waits on a channel
// (also with reflect.Select) stops with ErrCanceled, and its deferred calls
// run. The host gets http.ErrAbortHandler. No budget is set.
func TestBudgetCancel(t *testing.T) {
	_, h := loadBudgetPlug(t, Options{Cancel: true}, "x")
	for _, path := range []string{"/spin", "/sleep", "/block", "/rselect", "/lock"} {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		done := make(chan any, 1)
		go func() {
			_, r := serveCtx(h, ctx, path)
			done <- r
		}()
		select {
		case r := <-done:
			if r != http.ErrAbortHandler {
				t.Errorf("%s: panic %v, want %v", path, r, http.ErrAbortHandler)
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("%s: still running", path)
		}
		cancel()
	}
	// The deferred Unlock of /lock ran.
	if body, _ := serveBG(h, "/locked"); body != "free" {
		t.Errorf("/locked: %q, want free", body)
	}
	// A request whose context is already done does not run.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, r := serveCtx(h, ctx, "/spin"); r != http.ErrAbortHandler {
		t.Errorf("done context: panic %v, want %v", r, http.ErrAbortHandler)
	}
}

// TestNoAccountWithoutLimit checks that a call from the host without Budget
// and without Options.Cancel creates no account: topHandler and cancelCtx
// give context.Background() (never done) instead of a nil context, which
// staticcheck refuses (SA1012).
func TestNoAccountWithoutLimit(t *testing.T) {
	m := &Machine{}
	if a := m.newAccount(m.cancelCtx(context.Background())); a != nil {
		t.Errorf("newAccount without Budget and Cancel = %p, want nil", a)
	}
	if a := m.newAccount(m.cancelCtx(t.Context())); a != nil {
		t.Errorf("newAccount of a cancelable ctx without Cancel = %p, want nil", a)
	}
	m.Cancel = true
	if a := m.newAccount(m.cancelCtx(t.Context())); a == nil {
		t.Error("newAccount with Cancel: no account for a cancelable ctx")
	} else {
		a.close()
	}
}

// TestCancelOffByDefault: without
// Options.Cancel, a request whose client goes away runs to its end, as in Go
// and yaegi, also with a budget: the sync.Once of /onceset sets both values,
// and the host gets no panic. Before, the VM stopped the call at its next
// step: the Once stayed done with one value set, and the host got the panic
// "vm: call canceled" at each disconnect.
func TestCancelOffByDefault(t *testing.T) {
	for _, opts := range []Options{{}, {Budget: 1e8}} {
		_, h := loadBudgetPlug(t, opts, "x")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		body, r := serveCtx(h, ctx, "/onceset")
		cancel()
		if r != nil || body != "1 1" {
			t.Errorf("Budget %d: body %q, panic %v; want \"1 1\" and no panic", opts.Budget, body, r)
		}
		if body, _ := serveBG(h, "/onceset"); body != "1 1" {
			t.Errorf("Budget %d: state after the canceled request %q, want \"1 1\"", opts.Budget, body)
		}
	}
}

// TestBudgetCancelGoroutines: with Options.Cancel, the goroutines of a
// canceled call stop with it, without reaching GoPanic.
func TestBudgetCancelGoroutines(t *testing.T) {
	var panics atomic.Int64
	p, h := loadBudgetPlug(t, Options{Cancel: true, GoPanic: func(any) { panics.Add(1) }}, "x")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// /spread waits for its 20000 goroutines, which loop until canceled
	// (no budget).
	if _, r := serveCtx(h, ctx, "/spread"); r != http.ErrAbortHandler {
		t.Fatalf("/spread: panic %v, want %v", r, http.ErrAbortHandler)
	}
	if !waitFor(60*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("%d live goroutines", p.M.Goroutines())
	}
	if n := panics.Load(); n != 0 {
		t.Fatalf("GoPanic called %d times", n)
	}
}

// TestBudgetInitDeadline: `for {}` in init returns an error when the context
// of LoadContext is done, instead of hanging the host. The error is a
// *LoadError (the load deadline) that also wraps ErrCanceled (the init was
// running), and the init really stops: LoadContext returns at once, so the
// test waits for the "init" load stage, which runs when the init returns.
// The deadline is set when the load reaches the init (stage "checked"): a
// deadline set before the load passed during the parse on a slow runner,
// and the error had no ErrCanceled.
func TestBudgetInitDeadline(t *testing.T) {
	initStopped := make(chan struct{})
	var once sync.Once
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hook := func(s string) {
		switch s {
		case "checked":
			time.AfterFunc(100*time.Millisecond, cancel)
		case "init":
			once.Do(func() { close(initStopped) })
		}
	}
	_, err := LoadContext(ctx, Options{Dir: "..", hooks: &loadHooks{stage: hook}}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/initloop")
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("error %v, want %v", err, ErrCanceled)
	}
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatalf("error %v (%T), want a *LoadError", err, err)
	}
	select {
	case <-initStopped:
	case <-time.After(30 * time.Second):
		t.Fatal("the init still runs after the load was canceled")
	}
}

// TestBudgetCancelGrace: the deferred calls of a canceled call share the
// grace (cancelGrace steps). A deferred cleanup of about 10000 steps runs
// to its end and unlocks the mutex. Before, each refill after the cancel
// panicked again: the cleanup stopped after about 1024 steps, the mutex
// stayed locked, and the next request hung on mu.Lock.
func TestBudgetCancelGrace(t *testing.T) {
	_, h := loadBudgetPlug(t, Options{Cancel: true}, "x")
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		done := make(chan any, 1)
		go func() {
			_, r := serveCtx(h, ctx, "/lockwork")
			done <- r
		}()
		select {
		case r := <-done:
			if r != http.ErrAbortHandler {
				t.Fatalf("/lockwork %d: panic %v, want %v", i, r, http.ErrAbortHandler)
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("/lockwork %d: still running (mutex left locked?)", i)
		}
		cancel()
		if body, _ := serveBG(h, "/locked"); body != "free" {
			t.Fatalf("/locked after /lockwork %d: %q, want free", i, body)
		}
	}
}

// TestBudgetNativeHandler: when the handler that New returns is native
// (http.FileServer over a file system of the plugin), each Open, Stat, Seek
// and Read is a new run of plugin code. All of them share the account of
// the request: its budget and its context. Before, only the first run used
// it: with Budget 1e6, one request ran 36M steps and ignored its deadline.
func TestBudgetNativeHandler(t *testing.T) {
	// Each Read does 900000 steps: the second one goes past the budget.
	_, h := loadBudgetPlug(t, Options{Budget: 1e6}, "fs")
	if body, r := serveBG(h, "/f.txt"); r != ErrBudget {
		t.Fatalf("Budget 1e6: body %q, panic %v, want %v", body, r, ErrBudget)
	}
	// Without budget, the deadline of the request stops it (Options.Cancel).
	_, h = loadBudgetPlug(t, Options{Cancel: true}, "fs")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if body, r := serveCtx(h, ctx, "/f.txt"); r != http.ErrAbortHandler {
		t.Fatalf("deadline: body %q, panic %v, want %v", body, r, http.ErrAbortHandler)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("deadline of 20ms: the request took %v", d)
	}
	// The same file server called by another plugin (a middleware chain):
	// the call from the host starts on a goroutine that already runs
	// interpreted code, of another Machine.
	_, fsh := loadBudgetPlug(t, Options{Budget: 1e6}, "fs")
	p, _ := loadBudgetPlug(t, Options{Budget: 1e6}, "x")
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err = newHandler(context.Background(), p, fsh, cfg, "next")
	if err != nil {
		t.Fatal(err)
	}
	if body, r := serveBG(h, "/f.txt"); r != ErrBudget {
		t.Fatalf("chain, Budget 1e6: body %q, panic %v, want %v", body, r, ErrBudget)
	}
}

// TestBudgetCanceledNativeCallback: a callback that native code runs on a
// goroutine of its own (time.AfterFunc) calls next, a handler of another
// plugin, with the request of the call. When the context of the request is
// done, the call of next stops with ErrCanceled. This is not a panic of the
// plugin: the callback ends quietly, and GoPanic is not called. Before, the
// outermost run of the callback (callOutermost) gave ErrCanceled to GoPanic.
func TestBudgetCanceledNativeCallback(t *testing.T) {
	// next spins until the context of its request is done (no budget).
	_, next := loadBudgetPlug(t, Options{Cancel: true}, "x")
	panics := make(chan any, 4)
	p, err := Load(Options{Dir: "..", Cancel: true, GoPanic: func(r any) { panics <- r }}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	// The timer callback calls next with the request, and the handler waits
	// for the callback to end.
	h, err := newHandler(context.Background(), p, next, cfg, "timernext")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan any, 1)
	go func() {
		_, r := serveCtx(h, ctx, "/spin")
		done <- r
	}()
	select {
	case r := <-done:
		if r != nil && r != http.ErrAbortHandler {
			t.Fatalf("panic %v, want none or %v", r, http.ErrAbortHandler)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("still running")
	}
	select {
	case r := <-panics:
		t.Fatalf("GoPanic got %v, want nothing", r)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestBudgetCanceledPullStop: the plugin calls iter.Pull and gives the stop
// func to context.AfterFunc on the context of the request. When the request
// is canceled, stop runs on a native goroutine (callClosure, goOutermost).
// The sequence is on the canceled account: it stops with ErrCanceled at its
// next wait, and stop panics again with it. This is not a panic of the
// plugin: GoPanic must not get it. Before, goOutermost gave ErrCanceled to
// GoPanic (in about 2 runs of 3).
func TestBudgetCanceledPullStop(t *testing.T) {
	panics := make(chan any, 64)
	p, err := Load(Options{Dir: "..", Cancel: true, GoPanic: func(r any) { panics <- r }}, "github.com/traefik/yaegi/v2/vm/testdata/restricted/asyncpanicplug")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "pullctx")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		done := make(chan any, 1)
		go func() {
			_, r := serveCtx(h, ctx, "/")
			done <- r
		}()
		select {
		case r := <-done:
			if r != nil && r != http.ErrAbortHandler {
				cancel()
				t.Fatalf("run %d: panic %v, want none or %v", i, r, http.ErrAbortHandler)
			}
		case <-time.After(30 * time.Second):
			cancel()
			t.Fatalf("run %d: still running", i)
		}
		cancel()
	}
	select {
	case r := <-panics:
		t.Fatalf("GoPanic got %v, want nothing", r)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestBudgetPendingNested: a call from the host inside another one, on a
// goroutine without interpreted code (a native handler of one plugin that
// calls another plugin), puts back the account of the outer call when it
// returns, and leaves no entry behind. Before, the inner call replaced the
// outer entry, the table counted two entries and only one was removed.
func TestBudgetPendingNested(t *testing.T) {
	ma, mb := &Machine{Budget: 100}, &Machine{Budget: 100}
	g := getg()
	ma.top(context.Background(), func() {
		outer := peekPending(g)
		if outer == nil {
			t.Fatal("no pending account in the outer call")
		}
		mb.top(context.Background(), func() {
			if a := peekPending(g); a == nil || a == outer {
				t.Fatal("the inner call does not use an account of its own")
			}
		})
		if a := peekPending(g); a != outer {
			t.Fatal("the outer account was not put back")
		}
	})
	if peekPending(g) != nil || pslot(g) >= 0 {
		t.Fatalf("pending entry left after the calls (slot %d)", pslot(g))
	}
}

// TestBudgetNamedHandlerInterface: New may return an interface type of the
// plugin with a ServeHTTP method instead of http.Handler. Object.Interface
// must still give a handler whose each ServeHTTP is a call from the host:
// before, only the static type http.Handler was wrapped, so the ServeHTTP
// of such a handler ran without an account of its own and the budget error
// went to Options.GoPanic, while the request returned as if it succeeded.
func TestBudgetNamedHandlerInterface(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, `package p

import (
	"context"
	"net/http"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

type Handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type spin struct{}

func (spin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for n := 0; ; n++ {
	}
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (Handler, error) {
	return spin{}, nil
}
`)
	var goPanics atomic.Int32
	p, err := Load(Options{Dir: dir, Budget: 1_000_000, GoPanic: func(any) { goPanics.Add(1) }}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := createConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(context.Background(), p, http.NotFoundHandler(), cfg, "spin")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(topHandler); !ok {
		t.Errorf("handler %T, want a topHandler", h)
	}
	_, r := serveBG(h, "/")
	if err, ok := r.(error); !ok || !errors.Is(err, ErrBudget) {
		t.Errorf("ServeHTTP panic %v, want ErrBudget", r)
	}
	if n := goPanics.Load(); n != 0 {
		t.Errorf("%d panics went to GoPanic, want 0", n)
	}
}

// TestBudgetBackgroundRate: 12 background goroutines that
// work 10000 steps and sleep for a microsecond run at most BackgroundRate
// steps per second together, and they still run. Before, the budget only
// bounded the steps between two blocking points: such goroutines used 13
// CPU-seconds in 4.6 s after the request ended.
func TestBudgetBackgroundRate(t *testing.T) {
	const rate = 1e6
	p, h := loadBudgetPlug(t, Options{Budget: 1e6, BackgroundRate: rate, GoPanic: func(r any) { t.Errorf("GoPanic: %v", r) }}, "x")
	defer func() {
		serveBG(h, "/ratestop")
		if !waitFor(30*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
			t.Errorf("%d live goroutines after /ratestop", p.M.Goroutines())
		}
	}()
	rounds := func() int {
		body, _ := serveBG(h, "/raterounds")
		n, _ := strconv.Atoi(body)
		return n
	}
	if _, r := serveBG(h, "/ratestart"); r != nil {
		t.Fatal(r)
	}
	time.Sleep(1500 * time.Millisecond) // past the burst of one second
	n1, t1 := rounds(), time.Now()
	time.Sleep(time.Second)
	n2, d := rounds(), time.Since(t1)
	// A round is about 10000 steps: rate/10000 rounds per second, plus one
	// chunk (rate/100 steps) per goroutine.
	want := int(d.Seconds()*rate/10000) + 12*int(rate/100/10000) + 12
	if n2-n1 > 2*want {
		t.Fatalf("%d rounds in %v, want at most about %d", n2-n1, d, want)
	}
	if n2 <= n1 {
		t.Fatalf("no round in %v: the goroutines must be slowed, not stopped", d)
	}
	t.Logf("%d rounds in %v (%d before), want at most about %d", n2-n1, d, n1, want)

	// A call from the host and the goroutines it starts are not slowed,
	// also without a Budget: /pullsmall runs 400000 steps on 4 goroutines,
	// which would take 400 s at this rate.
	_, h2 := loadBudgetPlug(t, Options{BackgroundRate: 1000}, "x")
	start := time.Now()
	if body, r := serveBG(h2, "/pullsmall"); r != nil || body == "" {
		t.Fatalf("/pullsmall: %q, panic %v", body, r)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Fatalf("/pullsmall took %v: a call from the host was slowed by BackgroundRate", d)
	}
}

// TestBudgetNativeCallbackCap: the callbacks that native code runs on
// goroutines of its own (50 time.AfterFunc callbacks of 100 ms each) count
// in MaxGoroutines while they run. Past the cap, a callback does not run and
// GoPanic gets ErrTooManyGoroutines, as for a go statement. Before, they
// were not counted: all 50 ran at the same time with MaxGoroutines 10.
func TestBudgetNativeCallbackCap(t *testing.T) {
	const max = 10
	var refused atomic.Int64
	p, h := loadBudgetPlug(t, Options{MaxGoroutines: max, GoPanic: func(r any) {
		if r != ErrTooManyGoroutines {
			t.Errorf("GoPanic got %v, want %v", r, ErrTooManyGoroutines)
		}
		refused.Add(1)
	}}, "x")
	if _, r := serveBG(h, "/timerscap"); r != nil {
		t.Fatal(r)
	}
	var maxLive, ran int
	ok := waitFor(30*time.Second, func() bool {
		body, r := serveBG(h, "/timerscapstat")
		f := strings.Fields(body)
		if len(f) != 2 {
			t.Fatalf("/timerscapstat: %q, panic %v", body, r)
		}
		maxLive, _ = strconv.Atoi(f[0])
		ran, _ = strconv.Atoi(f[1])
		return ran+int(refused.Load()) == 50
	})
	if !ok {
		t.Fatalf("%d callbacks ran and %d were refused, want 50 in all", ran, refused.Load())
	}
	if maxLive > max || ran == 0 {
		t.Fatalf("%d callbacks ran at the same time (%d ran), want at most %d", maxLive, ran, max)
	}
	if n := p.M.Goroutines(); n != 0 {
		t.Fatalf("%d live goroutines after the callbacks", n)
	}
	t.Logf("%d ran (at most %d at the same time), %d refused", ran, maxLive, refused.Load())
}
