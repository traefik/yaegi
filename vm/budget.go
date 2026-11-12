package vm

// The execution budget: one budget per call from the host, shared with the
// goroutines that the call starts, tied to the context of the call; a cap
// on the live goroutines of a Machine; and a rule without a total for the
// goroutines that run in the background.
//
// Steps. The VM counts one step per interpreted call and per backward jump
// (and per link of an error chain, errors.go). Each ctx counts its steps in
// ctx.steps and compares them with ctx.limit: this is the only work on the
// hot path. When steps reach limit, the slow path (refill) decides what
// happens next.
//
// Calls from the host. A call from the host into the VM (init, each
// Plugin.Call and Object.Call, each ServeHTTP) owns an account: Machine.Budget steps, and the
// context of the call when there is one (the request of ServeHTTP, the
// context of Plugin.Call and Object.Call, the context of LoadContext for init). The
// ctx of the call and every goroutine started during the call (go
// statements and iter.Pull sequences, also those started by these
// goroutines) take steps from the same account, in chunks (minChunk, doubled
// at each refill up to maxChunk), so the hot path never touches shared
// memory. A goroutine gives back the unused part of its chunk when it ends.
// When the account is empty, every goroutine that uses it stops with
// ErrBudget at its next refill, or at once when it waits on a channel, in a
// select or in time.Sleep. So a call and all its goroutines run at most
// Budget steps together, whatever the number of goroutines or iter.Pull
// levels (before, each goroutine got a full budget of its own).
//
// Cancellation (Options.Cancel; always for the init of LoadContext). When
// the context of the call is done (context.AfterFunc), the account is
// canceled: every goroutine that uses it stops with
// ErrCanceled at its next refill (at most maxChunk steps later), or at once
// when it waits on a channel, in a select or in time.Sleep. ErrCanceled is
// not a fault of the plugin: the deferred calls of the stopped frames run,
// so a deferred Unlock still unlocks (recover cannot stop it, as for
// runtime.Goexit). After the first ErrCanceled of a goroutine, its refills
// do not panic again: they give steps from a grace of cancelGrace steps,
// shared by all the deferred calls of the goroutine, so a deferred cleanup
// of a few thousand steps runs to its end. Past the grace, the goroutine
// stops at once, without running the remaining deferred calls
// (errCanceledHard). A wait (channel, select, time.Sleep) during the
// deferred calls cannot end any more: it panics ErrCanceled again, and the
// deferred calls below it still run. The host gets ErrCanceled in all
// cases. Native code cannot be
// stopped: a blocking read, or a long computation inside native code, ends
// when it ends; the VM stops at the first step after it.
//
// Callbacks on goroutines of native code. A func or a value of the plugin
// that native code runs on a goroutine of its own (time.AfterFunc,
// context.AfterFunc, the handlers of an http.Server of the plugin) uses the
// account of the call that gave it to native code (originAcct, withOrigin):
// it shares the budget and the context of that call while the call runs,
// and follows the background rule after it. A callback made outside any
// call (from a background goroutine, or with neither Budget nor Cancel)
// owns an account, as a call from the host.
//
// Background goroutines. When the call from the host returns, its account
// closes. The goroutines that still run (a cache janitor, a ticker loop
// started in New, the Provide loop of a provider) and the goroutines
// started outside any call become background goroutines: they have no
// total any more, but each one may run at most Budget steps between two
// blocking points. A blocking point is a channel send or receive, a select
// without default, or a call to time.Sleep. So `for { work(); time.Sleep(d)
// }` and `for range ticker.C { work() }` run forever while each round stays
// under the budget, and `for {}` stops with ErrBudget after Budget steps.
// The steps that a goroutine ran since its last blocking point while its
// call was still running count too. Mutex waits, WaitGroup.Wait and the
// blocking calls of native code (a network read) are not blocking points
// for this rule. The rule bounds each round, and MaxGoroutines the number
// of goroutines, so a background goroutine that waits often could still use
// a CPU for ever: Machine.BackgroundRate (off by default) also bounds the
// steps per second of all the background goroutines of the machine
// together. They take their steps in chunks from a token bucket
// (rateBucket), and a goroutine that finds it empty sleeps until its chunk
// is paid: it is slowed down, never stopped. With a BackgroundRate, each
// call from the host has an account, also without a Budget, so that the
// calls and their goroutines are not slowed.
//
// Goroutine cap. Machine.MaxGoroutines bounds the live goroutines that the
// program started (go statements and iter.Pull sequences). A go statement
// or the first next of an iter.Pull past the cap panics with
// ErrTooManyGoroutines in the goroutine that runs it; the program can
// recover it. A callback that native code runs on a goroutine of its own
// (time.AfterFunc, the handlers of an http.Server of the plugin) counts too
// while it runs (nativeCallback); past the cap, it panics with
// ErrTooManyGoroutines before it starts, and the panic goes to GoPanic (or
// to net/http, which recovers the panics of handlers).
//
// Callbacks. Native code can call back into the VM during a call, on the
// same goroutine: a func value given to strings.Map, a String method
// called by fmt, a Less function... Each callback runs on its own ctx, but
// it uses the account and the steps of the ctx below it, and gives back
// the steps it used (nestIn and nestOut in nest.go). So a plugin cannot get
// a new budget by moving its work into callbacks. The same per-goroutine
// state (gstate) also counts the depth of these callbacks.
//
// Native code can also recover the panic of a callback: fmt prints
// "%!v(PANIC=String method: ...)" and goes on. ErrBudget and
// ErrStackOverflow must stop the execution, so the ctx that raises them
// also records them in the gstate of its goroutine (gstate.fault), and the
// VM raises them again as soon as the native call returns (checkFault).
//
// A call from the host is not a callback, even when it comes from inside
// another call: when the same plugin comes twice in a middleware chain, the
// first instance calls the second one through next, a host handler. Each
// call from the host must have its own account, so the Plugin API runs
// each call through Machine.top: the next run on the goroutine then uses
// the account of top, and does not give its steps back. It still counts in
// the depth of the goroutine. f may run interpreted code many times (a
// native handler such as http.FileServer over a file system of the plugin:
// each Read is a new run): every run that f starts directly uses the
// account of top, and gives back the unused part of its chunk when it ends.

import (
	"context"
	"errors"
	"go/types"
	"math"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// ErrCanceled is the panic of the goroutines of a call from the host whose
// context is done.
var ErrCanceled = errors.New("vm: call canceled (context done)")

// errCanceledHard is ErrCanceled once the deferred calls have used their
// grace: no deferred call runs any more (isFatal).
var errCanceledHard error = canceledHard{}

type canceledHard struct{}

func (canceledHard) Error() string        { return ErrCanceled.Error() }
func (canceledHard) Is(target error) bool { return target == ErrCanceled }

// ErrTooManyGoroutines is the panic of a go statement (or of the first next
// of an iter.Pull) past Machine.MaxGoroutines.
var ErrTooManyGoroutines = errors.New("vm: too many goroutines (MaxGoroutines)")

const (
	minChunk    = 1 << 10 // first chunk a ctx takes from its account
	maxChunk    = 1 << 16 // largest chunk: the delay of a cancellation, in steps
	cancelGrace = 1 << 16 // steps of all the deferred calls of a goroutine after a cancellation
)

// States of an account.
const (
	acctOpen     int32 = iota // the call from the host runs
	acctClosed                // the call returned: its goroutines run in the background
	acctSpent                 // no step left: ErrBudget
	acctCanceled              // the context is done: ErrCanceled
)

// account is the budget of one call from the host, shared by the
// goroutines that the call starts.
type account struct {
	left    atomic.Int64 // steps not handed out yet (when limited)
	state   atomic.Int32
	limited bool        // Machine.Budget > 0
	release func() bool // stops the context.AfterFunc
	mu      sync.Mutex
	stop    chan struct{} // made by the first wait (stopChan), closed when the account is spent or canceled
	closed  bool          // halt ran (guarded by mu)
}

// newAccount returns the account of a call from the host with the context
// ctx (nil: none), or nil when there is nothing to count: no budget, and a
// context that is never done.
func (m *Machine) newAccount(ctx context.Context) *account {
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}
	if done == nil && m.Budget <= 0 && m.BackgroundRate <= 0 && m.MaxGoroutines <= 0 {
		return nil
	}
	// With a BackgroundRate, a call needs an account so that it is not a
	// background run; with a MaxGoroutines, so that a callback inside it is
	// not counted as a goroutine of native code (nativeCallback).
	a := &account{limited: m.Budget > 0}
	a.left.Store(m.Budget)
	if done != nil {
		a.release = context.AfterFunc(ctx, func() { a.halt(acctCanceled) })
	}
	return a
}

// halt ends an open account (state acctSpent or acctCanceled) and wakes
// the goroutines that wait for it.
func (a *account) halt(state int32) {
	if a.state.CompareAndSwap(acctOpen, state) {
		a.mu.Lock()
		if a.stop != nil && !a.closed {
			close(a.stop)
		}
		a.closed = true
		a.mu.Unlock()
	}
}

// stopChan returns the channel that is closed when a is spent or canceled.
// Most calls never wait, so it is made at the first wait.
func (a *account) stopChan() chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stop == nil {
		a.stop = make(chan struct{})
		if a.closed {
			close(a.stop)
		}
	}
	return a.stop
}

// close ends the call that owns a: its goroutines go on in the background.
func (a *account) close() {
	if a == nil {
		return
	}
	a.state.CompareAndSwap(acctOpen, acctClosed)
	if a.release != nil {
		a.release()
		// The values of the plugin that native code keeps (a func of
		// time.AfterFunc, a boxed value) hold a (originAcct): a must not
		// hold the context of the request.
		a.release = nil
	}
}

// take returns up to n steps from pool (0: none left).
func take(pool *atomic.Int64, n int64) int64 {
	left := pool.Add(-n)
	switch {
	case left >= 0:
		return n
	case left+n > 0:
		return left + n
	}
	return 0
}

// giveBack returns the unused steps of a goroutine that ends.
func (cx *ctx) giveBack() {
	if a := cx.acct; a != nil && a.limited && cx.limit > cx.steps && a.state.Load() == acctOpen {
		a.left.Add(cx.limit - cx.steps)
		cx.limit = cx.steps
	}
}

// useAcct makes cx count its steps against a (nil: the background rule, or
// no limit without a budget).
func (cx *ctx) useAcct(a *account) {
	if a == nil || a.state.Load() == acctClosed {
		cx.background()
		return
	}
	cx.acct, cx.limit = a, cx.steps // the next step refills
}

// background makes cx a background goroutine: at most Budget steps since
// its last blocking point, and steps of BackgroundRate.
func (cx *ctx) background() {
	cx.acct = nil
	if b := cx.m.Budget; b > 0 && cx.steps >= cx.slice+b {
		cx.outOfBudget()
	}
	cx.rateEnd = cx.steps // the next step takes a chunk of the rate
	cx.bgLimit()
}

// bgLimit sets the limit of the background goroutine cx: Budget steps since
// its last blocking point, and the end of its chunk of BackgroundRate.
func (cx *ctx) bgLimit() {
	lim := int64(math.MaxInt64)
	if b := cx.m.Budget; b > 0 {
		lim = cx.slice + b
	}
	if cx.m.BackgroundRate > 0 {
		lim = min(lim, cx.rateEnd)
	}
	cx.limit = lim
}

// bgRefill is refill for a background goroutine: ErrBudget past Budget steps
// since the last blocking point, else the next chunk of BackgroundRate.
func (cx *ctx) bgRefill() {
	m := cx.m
	if m.Budget > 0 && cx.steps >= cx.slice+m.Budget {
		cx.outOfBudget() // a background goroutine that did not block
	}
	if m.BackgroundRate > 0 && cx.steps >= cx.rateEnd {
		b := m.rateBucket()
		if d := b.reserve(b.chunk); d > 0 {
			time.Sleep(d)
		}
		cx.rateEnd = cx.steps + b.chunk
	}
	cx.bgLimit()
}

// rateBucket is the token bucket of Machine.BackgroundRate: the background
// goroutines of a machine take their steps from it in chunks, and a
// goroutine that finds it empty sleeps until its chunk is paid. So all of
// them together run at most BackgroundRate steps per second, after a first
// burst of one second. Before, a background goroutine had no total: only
// Budget steps between two blocking points, so a loop that sleeps for a
// microsecond between two rounds of work kept a CPU busy for ever.
type rateBucket struct {
	mu     sync.Mutex
	rate   float64 // steps per second
	tokens float64 // steps left (negative: steps owed, paid by the sleeps)
	last   time.Time
	chunk  int64 // steps per take: about 10 ms of the rate
}

// rateBucket returns the bucket of m.BackgroundRate, made at the first use.
func (m *Machine) rateBucket() *rateBucket {
	if b := m.bgRate.Load(); b != nil {
		return b
	}
	r := float64(m.BackgroundRate)
	b := &rateBucket{rate: r, tokens: r, last: time.Now(), chunk: min(max(m.BackgroundRate/100, minChunk), maxChunk)}
	if !m.bgRate.CompareAndSwap(nil, b) {
		return m.bgRate.Load()
	}
	return b
}

// reserve takes n steps from b and returns how long the caller must sleep
// before it runs them (0: at once).
func (b *rateBucket) reserve(n int64) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens = min(b.rate, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	b.tokens -= float64(n)
	if b.tokens >= 0 {
		return 0
	}
	return time.Duration(-b.tokens / b.rate * float64(time.Second))
}

// refill is the slow path of the step count: cx.steps reached cx.limit.
func (cx *ctx) refill() {
	if cx.m.closed.Load() {
		cx.closeStop(false) // close.go
		return
	}
	a := cx.acct
	if a == nil {
		cx.bgRefill()
		return
	}
	switch a.state.Load() {
	case acctClosed:
		cx.background()
		return
	case acctSpent:
		cx.outOfBudget()
	case acctCanceled:
		cx.cancel(false)
		return
	}
	if cx.chunk < minChunk {
		cx.chunk = minChunk
	}
	n := cx.chunk
	if a.limited {
		if n = take(&a.left, n); n == 0 {
			a.halt(acctSpent)
			cx.outOfBudget()
		}
	}
	cx.limit = cx.steps + n
	if cx.chunk < maxChunk {
		cx.chunk *= 2
	}
}

// halted stops cx when its machine was closed, or its account was spent or
// canceled, while cx waited.
func (cx *ctx) halted() {
	if cx.m.closed.Load() {
		cx.closeStop(true) // close.go
	}
	if cx.acct == nil {
		return
	}
	switch cx.acct.state.Load() {
	case acctSpent:
		cx.outOfBudget()
	case acctCanceled:
		cx.cancel(true)
	}
}

// cancel stops cx with ErrCanceled when its account is canceled (wait: cx
// waited and cannot go on). The first time on the goroutine, it panics
// ErrCanceled: the deferred calls run, with steps of the grace. Later, while
// the deferred calls run, a refill gives more steps of the grace and does
// not panic, but a wait panics ErrCanceled again. Without grace left, cx
// stops with errCanceledHard.
func (cx *ctx) cancel(wait bool) {
	gs := cx.gs
	if gs.graceAcct != cx.acct {
		gs.graceAcct, gs.graceLeft = cx.acct, cancelGrace
		cx.grace(errCanceledHard)
		panic(ErrCanceled)
	}
	cx.grace(errCanceledHard)
	if wait {
		panic(ErrCanceled)
	}
}

// grace gives cx the next steps of the grace of its goroutine, or stops it
// with hard (errCanceledHard, errClosedHard) when the grace is spent.
func (cx *ctx) grace(hard error) {
	gs := cx.gs
	n := min(gs.graceLeft, minChunk)
	if n <= 0 {
		cx.fail(hard)
	}
	gs.graceLeft -= n
	cx.limit = cx.steps + n
}

// blocked records a blocking point of cx (see the background rule above).
func (cx *ctx) blocked() {
	cx.slice = cx.steps
	if cx.acct == nil && (cx.m.Budget > 0 || cx.m.BackgroundRate > 0) {
		cx.bgLimit()
	}
}

// isCanceled reports whether r stops a goroutine because the context of its
// call is done.
func isCanceled(r any) bool { return r == ErrCanceled || r == errCanceledHard }

// isStopped reports whether r stops a goroutine because the context of its
// call is done or its machine closed (close.go).
func isStopped(r any) bool { return isCanceled(r) || isClosed(r) }

// quietStop reports whether r ends a goroutine of the plugin quietly: its
// call was canceled, or a handler of another plugin that it called (next)
// was (topHandler gives http.ErrAbortHandler to its caller). The deferred
// calls ran, and it is not a panic of the plugin: GoPanic does not get it.
func quietStop(r any) bool { return isStopped(r) || r == http.ErrAbortHandler }

// goReserve counts one more live goroutine of the program, or panics with
// ErrTooManyGoroutines past the cap.
func (m *Machine) goReserve() {
	if m.closed.Load() {
		panic(ErrClosed) // close.go
	}
	if n := m.nGo.Add(1); m.MaxGoroutines > 0 && n > int64(m.MaxGoroutines) {
		m.nGo.Add(-1)
		panic(ErrTooManyGoroutines)
	}
}

// goRelease counts one goroutine of the program less.
func (m *Machine) goRelease() { m.nGo.Add(-1) }

// Goroutines returns the number of live goroutines that the program started
// (go statements and iter.Pull sequences), the number that MaxGoroutines
// bounds.
func (m *Machine) Goroutines() int { return int(m.nGo.Load()) }

// goCtx returns the ctx of a goroutine that parent (nil: native code)
// starts: it uses the account of parent while the call of parent runs. Its
// register stack is made by its first run (grow), as for Call.
func (m *Machine) goCtx(parent *ctx) *ctx {
	sub := &ctx{m: m, maxFrames: m.MaxDepth, kind: ctxGo}
	if parent != nil && parent.m == m {
		sub.useAcct(parent.acct)
	} else {
		sub.background()
	}
	return sub
}

// chanSend sends v on the interpreted or native channel c (t: the element
// type). In a call from the host, the wait ends when the account stops;
// always when the machine closes.
func (cx *ctx) chanSend(c any, v Value, t types.Type) {
	if rv, ok := c.(reflect.Value); ok { // native channel
		if cx.fastSend(rv, v) {
			return // nativechan.go
		}
		nv := cx.m.toNative(v, t)
		if !rv.TrySend(nv) {
			as, ms := cx.stopChans()
			if i, _, _ := reflect.Select([]reflect.SelectCase{
				{Dir: reflect.SelectSend, Chan: rv, Send: nv},
				{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(as)},
				{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ms)},
			}); i > 0 {
				cx.halted()
			}
		}
		cx.blocked()
		return
	}
	ch, _ := c.(chan Value)
	select {
	case ch <- v:
	default:
		as, ms := cx.stopChans()
		select {
		case ch <- v:
		case <-as:
			cx.halted()
		case <-ms:
			cx.halted()
		}
	}
	cx.blocked()
}

// chanRecv receives from the interpreted channel c. In a call from the
// host, the wait ends when the account stops; always when the machine
// closes.
func (cx *ctx) chanRecv(ch chan Value) (v Value, ok bool) {
	select {
	case v, ok = <-ch:
	default:
		as, ms := cx.stopChans()
		select {
		case v, ok = <-ch:
		case <-as:
			cx.halted()
		case <-ms:
			cx.halted()
		}
	}
	cx.blocked()
	return v, ok
}

// natRecv receives from the native channel rv, as chanRecv.
func (cx *ctx) natRecv(rv reflect.Value) (x reflect.Value, ok bool) {
	if x, ok = rv.TryRecv(); !x.IsValid() { // no value, and the channel is open
		as, ms := cx.stopChans()
		var i int
		i, x, ok = reflect.Select([]reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: rv},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(as)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ms)},
		})
		if i > 0 {
			cx.halted()
		}
	}
	cx.blocked()
	return x, ok
}

// sleep replaces time.Sleep: a blocking point, which ends early when the
// account of the call stops.
func (m *Machine) sleep(d time.Duration) {
	var cx *ctx
	if gs := lookupG(getg()); gs != nil {
		cx = gs.cur
	}
	if cx == nil {
		time.Sleep(d)
		return
	}
	if d > 0 {
		as, ms := cx.stopChans()
		t := time.NewTimer(d)
		select {
		case <-t.C:
		case <-as:
			t.Stop()
			cx.halted()
		case <-ms:
			t.Stop()
			cx.halted()
		}
	}
	cx.blocked()
}

// The accounts of calls from the host that run on a goroutine without
// interpreted code (top), for the whole call: every run that the call
// starts on the goroutine uses it (callCtx). Only goroutine g sets, reads
// and removes the account of g, so the table has the layout of gkeys/gvals
// (nest.go): buckets of gWays slots, found by key without a lock, and a map
// under a lock (pspill) when the bucket of g is full. Before, one map under
// a process-wide lock was taken three times per request: the requests of
// all the plugins waited for each other.
var (
	pkeys  [gBuckets * gWays]atomic.Uintptr
	pvals  [gBuckets * gWays]*account
	pspill struct {
		n  atomic.Int32
		mu sync.Mutex
		m  map[uintptr]*account
	}
)

// pslot returns the slot of goroutine g in pkeys, or -1.
func pslot(g uintptr) int {
	b := gbucket(g)
	for i := b; i < b+gWays; i++ {
		if pkeys[i].Load() == g {
			return i
		}
	}
	return -1
}

// setPending makes a the account of goroutine g, and returns the account
// it replaces (a call from the host inside another one, with no
// interpreted code between them: a native handler of one plugin that calls
// another plugin), for restorePending.
func setPending(g uintptr, a *account) (prev *account, had bool) {
	if i := pslot(g); i >= 0 {
		prev, pvals[i] = pvals[i], a
		return prev, true
	}
	if pspill.n.Load() > 0 {
		pspill.mu.Lock()
		prev, had = pspill.m[g]
		if had {
			pspill.m[g] = a
		}
		pspill.mu.Unlock()
		if had {
			return prev, true
		}
	}
	b := gbucket(g)
	for i := b; i < b+gWays; i++ {
		if pkeys[i].Load() == 0 && pkeys[i].CompareAndSwap(0, g) {
			pvals[i] = a
			return nil, false
		}
	}
	pspill.mu.Lock()
	if pspill.m == nil {
		pspill.m = map[uintptr]*account{}
	}
	pspill.m[g] = a
	pspill.n.Add(1)
	pspill.mu.Unlock()
	return nil, false
}

// restorePending puts back the account of goroutine g that setPending
// replaced, or forgets the entry of g.
func restorePending(g uintptr, prev *account, had bool) {
	if i := pslot(g); i >= 0 {
		if had {
			pvals[i] = prev
			return
		}
		pvals[i] = nil
		pkeys[i].Store(0)
		return
	}
	pspill.mu.Lock()
	defer pspill.mu.Unlock()
	if had {
		pspill.m[g] = prev
		return
	}
	if _, ok := pspill.m[g]; ok {
		delete(pspill.m, g)
		pspill.n.Add(-1)
	}
}

// peekPending returns the account of goroutine g, or nil.
func peekPending(g uintptr) *account {
	if i := pslot(g); i >= 0 {
		return pvals[i]
	}
	if pspill.n.Load() == 0 {
		return nil
	}
	pspill.mu.Lock()
	defer pspill.mu.Unlock()
	return pspill.m[g]
}

// top runs f, a call from the host with the context ctx (nil: none), with
// an account of its own, also when the goroutine already runs interpreted
// code (the call then comes from inside that code, through host code).
func (m *Machine) top(ctx context.Context, f func()) {
	// Counted before the check, so that CloseContext waits for every call
	// that passed it (close.go).
	m.inflight.Add(1)
	defer m.inflight.Add(-1)
	if m.refused() {
		panic(ErrClosed)
	}
	a := m.newAccount(ctx)
	defer a.close()
	defer func() {
		if r := recover(); r != nil {
			switch r {
			case errCanceledHard:
				r = ErrCanceled // the host sees one error
			case errClosedHard:
				r = ErrClosed
			}
			panic(r)
		}
	}()
	g := getg()
	gs := lookupG(g)
	if gs == nil {
		if a != nil {
			prev, had := setPending(g, a)
			defer restorePending(g, prev, had)
		}
		f()
		return
	}
	// The runs that f starts directly are nested on gs.cur (topOuter).
	fault, top, pend, outer := gs.fault, gs.top, gs.pending, gs.topOuter
	gs.top, gs.pending, gs.topOuter = true, a, gs.cur
	defer func() {
		// A fault of f is a fault of f only: the panic goes on, but the
		// code below does not record it.
		gs.fault, gs.top, gs.pending, gs.topOuter = fault, top, pend, outer
	}()
	f()
}

// originAcct returns the account of the run of m on this goroutine, or nil
// (no budget and no Cancel, a background goroutine, or no VM code on this
// goroutine). A func or a value of the plugin that native code gets keeps
// it (withOrigin).
func (m *Machine) originAcct() *account {
	if m.Budget <= 0 && !m.Cancel && m.BackgroundRate <= 0 {
		return nil // there are no accounts
	}
	gs := lookupG(getg())
	if gs == nil || gs.cur == nil || gs.cur.m != m {
		return nil
	}
	return gs.cur.acct
}

// withOrigin makes a callback that native code runs on a goroutine of its
// own (time.AfterFunc, context.AfterFunc, the handlers of an http.Server
// of the plugin) use a, the account of the call that gave the func or the
// value to native code (originAcct): while that call runs, the callback
// takes its steps from it and stops with it (ErrBudget, ErrCanceled); after
// the call, the account is closed and the callback is a background run
// (useAcct: Budget steps between two blocking points). Before, each such
// callback was a new call from the host with a full budget of its own. On a
// goroutine that already runs VM code, the callback nests in it (nest.go)
// and a is not used; on a goroutine where a call from the host runs (top:
// a native handler such as http.StripPrefix that New returned, called by
// the ServeHTTP of the request), the callback keeps the account of that
// call. The returned func restores the state.
func withOrigin(a *account) func() {
	if a == nil {
		return func() {}
	}
	g := getg()
	if lookupG(g) != nil || peekPending(g) != nil {
		return func() {}
	}
	prev, had := setPending(g, a)
	return func() { restorePending(g, prev, had) }
}

// nativeCBs holds the goroutines of native code that run a callback of a
// machine with a MaxGoroutines (nativeCallback): the outermost run of the
// callback counts as a live goroutine of the plugin (callOutermost).
var nativeCBs sync.Map // g (uintptr) -> struct{}

// nativeCallback starts a func or a method of the plugin that native code
// calls. On a goroutine that runs no interpreted code and no call from the
// host (a goroutine of native code: time.AfterFunc, context.AfterFunc, the
// handlers of an http.Server that the plugin started), the callback uses a,
// the account of the call that gave the value to native code (withOrigin),
// and with a MaxGoroutines it counts as a goroutine of the plugin while it
// runs (countNativeCallback). Before, these runs were not counted: 200
// time.AfterFunc callbacks ran at the same time with MaxGoroutines 10. The
// returned func ends it.
func (m *Machine) nativeCallback(a *account) func() {
	if m.MaxGoroutines <= 0 {
		return withOrigin(a)
	}
	g := getg()
	if lookupG(g) != nil || peekPending(g) != nil {
		return func() {} // nested in interpreted code, or in a call from the host
	}
	nativeCBs.Store(g, struct{}{})
	restore := withOrigin(a)
	return func() {
		restore()
		nativeCBs.Delete(g)
	}
}

// countNativeCallback counts the outermost run cx of goroutine g as a live
// goroutine of the plugin when g runs a callback of native code
// (nativeCallback), and returns the func that ends the count. Past
// MaxGoroutines, the callback panics with ErrTooManyGoroutines before it
// runs, as a go statement does: the caller of countNativeCallback
// (callOutermost) gives the panic to GoPanic, or to net/http when it
// recovers it (the handler of a server).
func (cx *ctx) countNativeCallback(g uintptr) func() {
	m := cx.m
	if m.MaxGoroutines <= 0 {
		return nil
	}
	if _, ok := nativeCBs.Load(g); !ok {
		return nil
	}
	if n := m.nGo.Add(1); n > int64(m.MaxGoroutines) {
		m.nGo.Add(-1)
		// The run does not start: forget it as callIn would.
		leaveG(g, cx.gs)
		cx.owned.close()
		panic(ErrTooManyGoroutines)
	}
	return m.goRelease
}

// callTop is call (hostGoexit false) as a call from the host with the
// context ctx (see top).
func (m *Machine) callTop(ctx context.Context, fn *Func, args []Value, env []Value) (rv Value) {
	m.top(ctx, func() { rv = m.call(fn, args, env, false) })
	return rv
}

// topHandler is the http.Handler that Object.Interface returns: each ServeHTTP
// is a call from the host, with its own account, tied to the context of the
// request with Options.Cancel.
type topHandler struct {
	m *Machine
	h http.Handler
}

func (t topHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.m.refused() {
		serveClosed(w) // close.go
		return
	}
	if t.m.recoverHandler {
		defer t.recoverServe(w)
	}
	defer func() {
		if rec := recover(); rec != nil {
			if isStopped(rec) {
				// the client is gone: net/http aborts the response, and
				// does not log it as a panic of the handler
				// (or the plugin was closed while the request ran)
				panic(http.ErrAbortHandler)
			}
			panic(rec)
		}
	}()
	if !t.m.Cancel {
		// Background is never done: the account is only for the budget.
		t.m.top(context.Background(), func() { t.h.ServeHTTP(w, r) })
		return
	}
	t.m.top(r.Context(), func() { t.h.ServeHTTP(w, r) })
}

// recoverServe is the deferred call of ServeHTTP with
// Options.RecoverHandler: a panic goes to GoPanic and the response gets the
// status 500; an aborted or canceled request ends quietly.
func (t topHandler) recoverServe(w http.ResponseWriter) {
	r := recover()
	if r == nil || quietStop(r) {
		return
	}
	t.m.GoPanic(r)
	w.WriteHeader(http.StatusInternalServerError)
}

// cancelCtx returns ctx when a call from the host stops when its context is
// done (Options.Cancel), context.Background() otherwise (never done).
func (m *Machine) cancelCtx(ctx context.Context) context.Context {
	if m.Cancel {
		return ctx
	}
	return context.Background()
}
