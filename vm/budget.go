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
// Rate (Machine.BudgetRate, off by default; host.Options sets it). Budget
// is then a burst, not a total: the account is a token bucket of size
// Budget that gets BudgetRate steps back per second of wall time (addRate,
// at each chunk taken, so the hot path does not read the clock). A call
// that uses on average less than BudgetRate steps per second runs for
// ever: a long stream, or a large download through a wrapper of the
// ResponseWriter that reads each byte. Before, Budget was a total per
// call: a stream of 1-byte writes through one wrapper died after about
// Budget writes, and a 256 MiB download through a byte loop was cut at
// 95 MiB. A call that uses more empties the bucket and stops with
// ErrBudget: `for {}` runs about 4e8 steps per second, so with the host
// defaults (1e8 and 1e8 per second) it stops after about 0.3 s, against
// 0.23 s with a total. A heavy loop can run fewer steps per second than
// the rate (a map write per round: about 1e7), so a call with a rate also
// stops when one of its goroutines runs Budget steps without a blocking
// point, the background rule below; here the end of a callback that native
// code made (the Write of a wrapper, called by the next handler for each
// part of the response) is a blocking point too. The bucket is per call,
// not per Machine: a stuck call does not take the steps of the other
// requests, and a Machine-wide cap would make one bad request slow down or
// stop the others. Time spent in native code (a network read) refills the
// bucket, and native code is still not stopped in the middle.
//
// Weights. A native call counts nativeCallSteps, plus steps for the bytes
// of its []byte and string arguments (more for package regexp), and a
// select counts selectSteps, so that the steps follow the CPU better
// (charge; the measures are with the constants).
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
// Their deferred calls still run, with the grace of a cancellation, and
// the fault is not raised again inside them (fail and reraise, nest.go).
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
	"fmt"
	"go/types"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/tools/go/ssa"
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
	left    atomic.Int64 // steps not handed out yet (when limited), never negative
	state   atomic.Int32
	limited bool  // Machine.Budget > 0
	burst   int64 // Machine.Budget: the most steps the account holds
	rate    int64 // Machine.BudgetRate: steps added per second (0: a total)
	start   time.Time
	last    atomic.Int64 // time of the last add of the rate, in ns since start
	inst    *Instance    // the instance of the call (instance.go), or nil
	release func() bool  // stops the context.AfterFunc
	mu      sync.Mutex
	stop    chan struct{} // made by the first wait (stopChan), closed when the account is spent or canceled
	closed  bool          // halt ran (guarded by mu)
	// fault is set on the account that a goroutine with a fault gives to
	// native code with a value of the plugin (stoppedOrigin): the
	// callbacks of that value do not run (refuseStopped).
	fault error
}

// stoppedOrigin returns the account that originAcct gives on a goroutine
// that stops with fault: it is spent for good, so a callback that uses it
// does not start (refuseStopped), and a wait on it ends at once.
func stoppedOrigin(fault error) *account {
	a := &account{fault: fault, closed: true}
	a.state.Store(acctSpent)
	return a
}

// newAccount returns the account of a call from the host with the context
// ctx (nil: none), or nil when there is nothing to count: no budget, and a
// context that is never done.
func (m *Machine) newAccount(ctx context.Context) *account {
	return m.newAccountIn(ctx, nil)
}

// newAccountIn is newAccount for a call in the instance inst (nil: none).
func (m *Machine) newAccountIn(ctx context.Context, inst *Instance) *account {
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}
	if done == nil && m.Budget <= 0 && m.BackgroundRate <= 0 && m.MaxGoroutines <= 0 && inst == nil {
		return nil
	}
	// With a BackgroundRate, a call needs an account so that it is not a
	// background run; with a MaxGoroutines, so that a callback inside it is
	// not counted as a goroutine of native code (nativeCallback).
	a := &account{limited: m.Budget > 0, burst: m.Budget, inst: inst}
	a.left.Store(m.Budget)
	if a.limited && m.BudgetRate > 0 {
		a.rate, a.start = m.BudgetRate, time.Now()
	}
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

// take returns up to n steps from the account (0: none left). With a
// BudgetRate, it first adds the steps of the time that passed.
func (a *account) take(n int64) int64 {
	if a.rate > 0 {
		a.addRate()
	}
	for {
		l := a.left.Load()
		if l <= 0 {
			return 0
		}
		g := min(n, l)
		if a.left.CompareAndSwap(l, l-g) {
			return g
		}
	}
}

// put adds n steps to the account, at most up to its burst.
func (a *account) put(n int64) {
	for {
		l := a.left.Load()
		nl := min(l+n, a.burst)
		if nl <= l || a.left.CompareAndSwap(l, nl) {
			return
		}
	}
}

// addRate adds to the account the steps of BudgetRate for the time since
// the last add (a token bucket of size Budget). The slow path of the step
// count calls it at most once per chunk (at most maxChunk steps), so the
// clock read costs nothing on the hot path.
func (a *account) addRate() {
	now := int64(time.Since(a.start))
	last := a.last.Load()
	d := now - last
	// float64: d * rate overflows an int64 after about 90 s at 1e8 steps
	// per second.
	n := float64(d) * float64(a.rate) / 1e9
	if n < 1 || !a.last.CompareAndSwap(last, now) {
		return // less than one step, or another goroutine adds it
	}
	a.put(int64(min(n, float64(a.burst))))
}

// giveBack returns the unused steps of a goroutine that ends.
func (cx *ctx) giveBack() {
	if a := cx.acct; a != nil && a.limited && cx.rateEnd > cx.steps && a.state.Load() == acctOpen {
		a.put(cx.rateEnd - cx.steps)
		cx.m.stats.steps.Add(cx.steps - cx.rateEnd)
		cx.limit, cx.rateEnd = cx.steps, cx.steps
	}
}

// useAcct makes cx count its steps against a (nil: the background rule, or
// no limit without a budget).
func (cx *ctx) useAcct(a *account) {
	if a != nil {
		cx.inst = a.inst // also for a closed account: a callback of native code (withOrigin)
	}
	if a == nil || a.state.Load() == acctClosed {
		cx.background()
		return
	}
	cx.acct, cx.limit, cx.rateEnd = a, cx.steps, cx.steps // the next step refills
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
		// The steps past the end of the chunk (charge) are paid too.
		n := b.chunk + cx.steps - cx.rateEnd
		if d := b.reserve(n); d > 0 {
			m.stats.bgWait.Add(int64(d))
			time.Sleep(d)
		}
		m.stats.steps.Add(n)
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
	if gs := cx.gs; gs != nil && gs.fault != nil {
		cx.faultGrace() // nest.go
		return
	}
	if cx.stopped() {
		cx.closeStop(false) // close.go, instance.go
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
	if a.rate > 0 {
		cx.creditSlice(a)
		if cx.steps >= cx.slice+cx.m.Budget {
			// Budget steps without a blocking point: a loop that never
			// waits, whatever the rate (a heavy loop can run fewer steps
			// per second than BudgetRate).
			a.halt(acctSpent)
			cx.outOfBudget()
		}
		if cx.steps < cx.rateEnd {
			cx.rateLimit() // only the slice ended: the chunk goes on
			return
		}
	}
	if cx.chunk < minChunk {
		cx.chunk = minChunk
	}
	n := cx.chunk
	if a.limited {
		// owe: the steps past the end of the chunk, counted at once by
		// charge (a native call, a select).
		owe := max(cx.steps-cx.rateEnd, 0)
		got := a.take(n + owe)
		if got <= owe {
			a.halt(acctSpent)
			cx.outOfBudget()
		}
		n = got - owe
		cx.m.stats.steps.Add(got)
	} else {
		cx.m.stats.steps.Add(n + max(cx.steps-cx.rateEnd, 0))
	}
	cx.rateEnd = cx.steps + n // the end of the chunk
	cx.limit = cx.rateEnd
	if a.rate > 0 {
		cx.rateLimit()
	}
	if cx.chunk < maxChunk {
		cx.chunk *= 2
	}
}

// budgetClock is the origin of ctx.sliceAt.
var budgetClock = time.Now()

// budgetNow is the time since budgetClock in ns.
func budgetNow() int64 {
	if f := budgetNowTest.Load(); f != nil {
		return (*f)()
	}
	return int64(time.Since(budgetClock))
}

// budgetNowTest replaces the clock of budgetNow in a test (a slow clock:
// TestBudgetRateSlowClock).
var budgetNowTest atomic.Pointer[func() int64]

// sliceRateDiv: with a BudgetRate, the steps since the last blocking point
// of a goroutine also go down by BudgetRate/sliceRateDiv per second (1e6
// with the host defaults). A loop that runs few steps per second but never
// reaches a blocking point is not a runaway loop: a plugin that copies a
// stream by itself (`for { n, _ := body.Read(buf); w.Write(buf[:n]) }`,
// about 50 steps per round with the weights) waits in native code, which
// is not a blocking point. Without the credit it stopped after Budget
// steps (2e6 rounds: about 30 minutes of an SSE stream at 1000 messages
// per second). A loop of the plugin that runs more steps per second than
// that (a map write per round: 1e7) still stops, after about
// Budget/(speed-credit) seconds.
const sliceRateDiv = 100

// creditSlice moves the last blocking point of cx (slice) forward by the
// credit of the time since the last refill of cx. It runs on the slow path
// (once per chunk), and the credit never passes cx.steps.
func (cx *ctx) creditSlice(a *account) {
	now := budgetNow()
	if cx.sliceAt != 0 && cx.slice < cx.steps {
		c := float64(now-cx.sliceAt) * float64(a.rate) / sliceRateDiv / 1e9
		cx.slice = min(cx.slice+int64(min(c, float64(cx.m.Budget))), cx.steps)
	}
	cx.sliceAt = now
}

// rateLimit sets the limit of cx, which uses an account with a
// BudgetRate: the end of its chunk, or Budget steps after its last
// blocking point if that comes first.
//
// The second limit is at least sliceStep steps away (so a loop can pass
// Budget by at most sliceStep). Without it, near the end of the slice the
// limit came a few steps after cx.steps: each refill read the clock, and
// the credit of the time of that read (creditSlice) moved the limit by
// about the steps run since the last refill. Where the clock is slow
// (linux/386), a `for {}` never reached the limit and ran for ever.
func (cx *ctx) rateLimit() {
	b := cx.m.Budget
	cx.limit = min(cx.rateEnd, max(cx.slice+b, cx.steps+min(sliceStep, max(b/8, 1))))
}

// sliceStep is the least distance of the slice limit of rateLimit.
const sliceStep = minChunk

// charge counts n steps at once: the work of a native call or of a select,
// which costs far more CPU than one step (see the weights below).
func (cx *ctx) charge(n int64) {
	cx.steps += n
	if cx.steps >= cx.limit {
		cx.refill()
	}
}

// The weights of the work that the VM does not see step by step. A step
// (a call or a backward jump) is about 2 ns in `for {}`, but a native
// call or a select costs far more, and the budget and BackgroundRate
// count steps, not CPU: 8 background goroutines in `for { select {
// default: } }` used 3.7 cores of 4 for ever at a BackgroundRate of 5e7,
// a loop that calls bytes.Count on 1 MiB used 3.95, and a regexp on 200
// MiB ran 12.9 s in one step. Measured on main (4 cores, Go 1.26, Budget
// 1e8): `for {}` 4.3e8 steps per second, strings.Repeat("x", 100) in a
// loop 7.5e6 calls per second (130 ns, 57 steps of `for {}`), a select
// with a default 4.1e6 per second (240 ns, 100 steps), bytes.Count on 1
// MiB 17 us (7300 steps, 1 per 143 bytes), a regexp on 1 MiB 40 ms (16
// steps per byte). The weights are below these costs, so that a plugin
// that worked within its budget still does.
const (
	// nativeCallSteps: each call of a native function or method.
	nativeCallSteps = 16
	// selectSteps: each select (reflect.Select with its cases).
	selectSteps = 64
	// byteSteps: steps per 64 bytes of the []byte and string arguments of
	// a native function (bytes.Count, strings.Index, a Write).
	byteSteps = 1
	// regexpByteSteps: steps per 64 bytes for package regexp (2 per
	// byte), which reads its input far more slowly.
	regexpByteSteps = 128
)

// memRangeFuncs are the functions of golang.org/x/sys/unix and syscall whose
// []byte argument only names a range of memory for the kernel: the call
// does not read the bytes, so their length does not count. badger syncs and
// advises its mappings of hundreds of MiB (filemap.go): at one step per 64
// bytes, each Msync cost millions of steps, and the BackgroundRate slowed
// every write down to about a second.
var memRangeFuncs = map[string]bool{
	"Msync": true, "Madvise": true, "Munmap": true, "Mlock": true, "Munlock": true, "Mprotect": true,
}

// weigh sets the weight of the native call site s of f: the arguments
// whose length counts (nativeSteps).
func (s *nativeSite) weigh(f *ssa.Function) {
	s.byteMul = byteSteps
	if f.Pkg != nil && f.Pkg.Pkg.Path() == "regexp" || f.Pkg == nil && strings.Contains(f.String(), "regexp.") {
		s.byteMul = regexpByteSteps
	}
	if f.Pkg != nil && (f.Pkg.Pkg.Path() == xsysUnixPath || f.Pkg.Pkg.Path() == "syscall") && memRangeFuncs[f.Name()] {
		return // the []byte is a range of memory for the kernel: no byte is read
	}
	for i, t := range s.params {
		if i >= len(s.args) {
			break
		}
		switch u := t.Underlying().(type) {
		case *types.Basic:
			if u.Info()&types.IsString == 0 {
				continue
			}
		case *types.Slice:
			if b, ok := u.Elem().Underlying().(*types.Basic); !ok || b.Kind() != types.Byte {
				continue
			}
		default:
			continue
		}
		s.byteArgs = append(s.byteArgs, s.args[i])
	}
}

// nativeSteps returns the steps that one call of s costs: nativeCallSteps,
// and byteMul steps per 64 bytes of its []byte and string arguments.
func (s *nativeSite) nativeSteps(regs []Value) int64 {
	n := int64(nativeCallSteps)
	for _, a := range s.byteArgs {
		n += int64(seqLen(regs[a].R)) * s.byteMul >> 6
	}
	return n
}

// halted stops cx when its machine was closed, or its account was spent or
// canceled, while cx waited.
func (cx *ctx) halted() {
	if cx.stopped() {
		cx.closeStop(true) // close.go, instance.go
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
		cx.limit = cx.steps // the call that passed the limit does not run
		cx.grace(errCanceledHard)
		panic(ErrCanceled)
	}
	cx.grace(errCanceledHard)
	if wait {
		panic(ErrCanceled)
	}
}

// grace gives cx the next steps of the grace of its goroutine, or stops it
// with hard (errCanceledHard, errClosedHard) when the grace is spent. The
// steps past the limit (the weight of a native call or of a select, which
// charge counts at once) are taken from the grace too: a deferred regexp
// on 64 MiB (1.3e8 steps) stops before it starts, instead of running 1.8 s
// after ErrCanceled.
func (cx *ctx) grace(hard error) {
	gs := cx.gs
	gs.graceLeft -= max(cx.steps-cx.limit, 0)
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
	if a := cx.acct; a == nil {
		if cx.m.Budget > 0 || cx.m.BackgroundRate > 0 {
			cx.bgLimit()
		}
	} else if a.rate > 0 && a.state.Load() == acctOpen {
		cx.rateLimit()
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

// goReserve counts one more live goroutine of the program in the instance
// inst (nil: none), or panics with ErrTooManyGoroutines past the cap, or
// with ErrClosed when the machine or inst is closed.
func (m *Machine) goReserve(inst *Instance) {
	if m.closed.Load() || inst != nil && inst.closed.Load() {
		panic(ErrClosed) // close.go, instance.go
	}
	if !m.goCount(inst) {
		panic(ErrTooManyGoroutines)
	}
}

// goCount counts one more live goroutine in inst (nil: none) and reports
// whether it is under the cap: MaxGoroutines bounds the goroutines of each
// instance, and the goroutines of no instance together (instance.go).
func (m *Machine) goCount(inst *Instance) bool {
	c := &m.nFree
	if inst != nil {
		c = &inst.n
	}
	if n := c.Add(1); m.MaxGoroutines > 0 && n > int64(m.MaxGoroutines) {
		c.Add(-1)
		return false
	}
	m.nGo.Add(1)
	return true
}

// goRelease counts one goroutine of the program in inst less.
func (m *Machine) goRelease(inst *Instance) {
	if inst != nil {
		inst.n.Add(-1)
	} else {
		m.nFree.Add(-1)
	}
	m.nGo.Add(-1)
}

// Goroutines returns the number of live goroutines that the program started
// (go statements and iter.Pull sequences), the ones of the instances
// included. MaxGoroutines bounds the ones of each instance, and the ones of
// no instance (instance.go).
func (m *Machine) Goroutines() int { return int(m.nGo.Load()) }

// goCtx returns the ctx of a goroutine that parent (nil: native code)
// starts: it uses the account of parent while the call of parent runs. Its
// register stack is made by its first run (grow), as for Call.
func (m *Machine) goCtx(parent *ctx) *ctx {
	sub := &ctx{m: m, maxFrames: m.MaxDepth, kind: ctxGo}
	if parent != nil && parent.m == m {
		sub.inst = parent.inst
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
// code (the call then comes from inside that code, through host code), in
// the Instance inst (nil: none, instance.go).
func (m *Machine) top(ctx context.Context, inst *Instance, f func()) {
	// Counted before the check, so that CloseContext waits for every call
	// that passed it (close.go).
	m.inflight.Add(1)
	defer m.inflight.Add(-1)
	if m.refused() {
		panic(ErrClosed)
	}
	a := m.newAccountIn(ctx, inst)
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
	// f is a new call: it starts without the stop of the code below (a
	// deferred call of a plugin that cleans up after ErrBudget calls the
	// next handler, another plugin). Else f ran on the grace of that stop
	// and then stopped with its error, without its own deferred calls.
	fault, left, cleanup, hard, exit := gs.fault, gs.faultLeft, gs.cleanup, gs.hard, gs.goexit
	gacct, gleft := gs.graceAcct, gs.graceLeft
	top, pend, outer := gs.top, gs.pending, gs.topOuter
	gs.fault, gs.faultLeft, gs.cleanup, gs.hard, gs.goexit = nil, 0, 0, false, false
	gs.graceAcct, gs.graceLeft = nil, 0
	gs.top, gs.pending, gs.topOuter = true, a, gs.cur
	defer func() {
		// A fault of f is a fault of f only: the panic goes on, but the
		// code below does not record it.
		gs.fault, gs.faultLeft, gs.cleanup, gs.hard, gs.goexit = fault, left, cleanup, hard, exit
		gs.graceAcct, gs.graceLeft = gacct, gleft
		gs.top, gs.pending, gs.topOuter = top, pend, outer
	}()
	f()
}

// originAcct returns the account of the run of m on this goroutine, or nil
// (no budget and no Cancel, a background goroutine of no instance, or no VM
// code on this goroutine). A func or a value of the plugin that native code
// gets keeps it (withOrigin). A background goroutine of an instance has no
// account: it gives the closed account of its instance (Instance.bg), so
// that a callback of native code that it starts (time.AfterFunc in a cache
// janitor that New started) still belongs to the instance.
func (m *Machine) originAcct() *account {
	if m.Budget <= 0 && !m.Cancel && m.BackgroundRate <= 0 && !m.instUsed.Load() {
		return nil // there are no accounts, or none with an instance
	}
	gs := lookupG(getg())
	if gs == nil || gs.cur == nil || gs.cur.m != m {
		return nil
	}
	if gs.fault != nil {
		// A goroutine that stops gives native code no callback that runs,
		// as it starts no goroutine (spawn), also when its account is
		// still open (ErrStackOverflow) or it is a background run.
		return stoppedOrigin(gs.fault)
	}
	if a := gs.cur.acct; a != nil {
		return a
	}
	if i := gs.cur.inst; i != nil {
		return i.bg
	}
	return nil
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
	inst := cx.inst
	if !m.goCount(inst) {
		// The run does not start: forget it as callIn would.
		leaveG(g, cx.gs)
		cx.owned.close()
		panic(ErrTooManyGoroutines)
	}
	return func() { m.goRelease(inst) }
}

// callTop is call (hostGoexit false) as a call from the host with the
// context ctx, in the Instance that ctx carries (see top).
func (m *Machine) callTop(ctx context.Context, fn *Func, args []Value, env []Value) (rv Value) {
	m.top(ctx, m.instanceOf(ctx), func() { rv = m.call(fn, args, env, false) })
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
	if t.m.restricted {
		r = hideServer(r)
	}
	if t.m.recoverHandler {
		defer t.recoverServe(w)
	}
	t.m.stats.requests.Add(1)
	defer func() {
		if rec := recover(); rec != nil {
			t.m.countFault(rec)
			if isStopped(rec) {
				// the client is gone: net/http aborts the response, and
				// does not log it as a panic of the handler
				// (or the plugin was closed while the request ran)
				panic(http.ErrAbortHandler)
			}
			if isBudget(rec) && !t.m.recoverHandler {
				t.m.budgetStops.Add(1)
				if t.m.quietBudget {
					// net/http aborts the response without a log and
					// a stack per request. The first stop of the
					// machine goes to GoPanic.
					t.m.reportBudget()
					panic(http.ErrAbortHandler)
				}
			}
			panic(rec)
		}
	}()
	if !t.m.Cancel {
		// Background is never done: the account is only for the budget.
		t.m.top(context.Background(), nil, func() { t.h.ServeHTTP(w, r) })
		return
	}
	t.m.top(r.Context(), nil, func() { t.h.ServeHTTP(w, r) }) // a request is in no instance
}

// hideServer returns r without the *http.Server of the host in its context.
// net/http puts the server that serves the request under
// http.ServerContextKey: a restricted plugin would write its fields
// (Handler, TLSConfig, ConnContext), which the host memory check does not
// see (hostmem.go knows the objects of the standard library variables
// only). The next handler of the host gets the server back
// (serverRestorer). Native code that the plugin calls itself does not: for
// example, a directory read error of http.FileServer logs to the standard
// logger, not to the ErrorLog of the server.
func hideServer(r *http.Request) *http.Request {
	if r == nil || r.Context().Value(http.ServerContextKey) == nil {
		return r
	}
	return r.WithContext(hideServerCtx(r.Context()))
}

// hideServerCtx is hideServer for a context.
func hideServerCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	srv := ctx.Value(http.ServerContextKey)
	if srv == nil {
		return ctx
	}
	return &noServerCtx{ctx, srv}
}

// noServerCtx is a context where http.ServerContextKey gives nil. It does
// not use context.WithValue: a valueCtx keeps its parent in the exported
// field Context, and the reflect of the plugin would read the server
// there. Here the parent and the server are in unexported fields, which
// reflect does not give to the plugin.
type noServerCtx struct {
	parent context.Context
	srv    any
}

// hostServerKey finds the noServerCtx of a request for restoreServer.
// Value gives the *noServerCtx, never the server: a context of the plugin
// in the chain sees the key and the result of Value, and the fields of
// a *noServerCtx are not exported.
type hostServerKey struct{}

func (c *noServerCtx) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c *noServerCtx) Done() <-chan struct{}       { return c.parent.Done() }
func (c *noServerCtx) Err() error                  { return c.parent.Err() }

func (c *noServerCtx) Value(key any) any {
	switch key {
	case http.ServerContextKey:
		return nil
	case hostServerKey{}:
		return c
	}
	return c.parent.Value(key)
}

// restoreServer puts the server that hideServer hid back in the context of
// r, for the next handler of the host. httputil.ReverseProxy reads it: it
// panics with http.ErrAbortHandler when the body of the backend fails, so
// that the client does not take a cut response for a complete one.
func restoreServer(r *http.Request) *http.Request {
	if r == nil {
		return r
	}
	ctx := r.Context()
	if ctx.Value(http.ServerContextKey) != nil {
		return r
	}
	// The plugin cannot make a *noServerCtx: its own context in the chain
	// can only give back one that the vm made.
	nsc, ok := ctx.Value(hostServerKey{}).(*noServerCtx)
	if !ok {
		return r
	}
	return r.WithContext(context.WithValue(ctx, http.ServerContextKey, nsc.srv))
}

// serverRestorer is a handler of the host that Plugin.Call gives to a
// restricted plugin (its next): the host gets back its server. Only the
// handlers of the host get it: a handler that the plugin gives to native
// code is not wrapped.
type serverRestorer struct{ h http.Handler }

func (s serverRestorer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.h.ServeHTTP(w, restoreServer(r))
}

// recoverServe is the deferred call of ServeHTTP with
// Options.RecoverHandler: a panic goes to GoPanic and the response gets the
// status 500; an aborted or canceled request ends quietly.
func (t topHandler) recoverServe(w http.ResponseWriter) {
	r := recover()
	if r == nil || quietStop(r) {
		return
	}
	if isBudget(r) {
		t.m.budgetStops.Add(1)
		if t.m.quietBudget {
			t.m.reportBudget() // only the first stop
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	t.m.GoPanic(r)
	w.WriteHeader(http.StatusInternalServerError)
}

// isBudget reports whether r is the end of the budget of a call.
func isBudget(r any) bool {
	err, ok := r.(error)
	return ok && errors.Is(err, ErrBudget)
}

// ErrBudgetStopped is what GoPanic gets, with Options.QuietBudget, for the
// first ServeHTTP of a machine that the budget stops (errors.Is(err,
// ErrBudget) is true). The ServeHTTP panics with http.ErrAbortHandler:
// net/http aborts the response without a log, so a plugin that the budget
// stops at each request does not log a stack per request.
// Machine.BudgetStops counts them all, and GoPanic gets a *BudgetStopsError
// at most once per BudgetReportInterval while the stops go on.
var ErrBudgetStopped = fmt.Errorf("%w: a request of the plugin was stopped (the next ones are counted by Machine.Stats, and reported at most once per minute)", ErrBudget)

// reportBudget gives the first budget stop of the machine to GoPanic
// (QuietBudget), then at most one *BudgetStopsError per
// BudgetReportInterval while the stops go on. The Machine of a plugin lives
// as long as the host (one Plugin per process, docs/RELOAD.md): with the
// first stop only, a plugin that starts to loop weeks after its first stop
// was not seen in the logs, unless the host polled Machine.Stats.
func (m *Machine) reportBudget() {
	if m.GoPanic == nil {
		return
	}
	now := budgetNow()
	if m.budgetReported.CompareAndSwap(false, true) {
		m.budgetReportN.Store(m.budgetStops.Load())
		m.budgetReportAt.Store(now)
		m.GoPanic(ErrBudgetStopped)
		return
	}
	last := m.budgetReportAt.Load()
	if now-last < int64(BudgetReportInterval) || !m.budgetReportAt.CompareAndSwap(last, now) {
		return
	}
	total := m.budgetStops.Load()
	n := total - m.budgetReportN.Swap(total)
	if n <= 0 {
		return
	}
	m.GoPanic(&BudgetStopsError{Stops: n, Total: total, Since: time.Duration(now - last)})
}

// BudgetReportInterval is the least time between two reports of the budget
// stops of a machine to GoPanic, with Options.QuietBudget: after the first
// stop (ErrBudgetStopped), GoPanic gets at most one *BudgetStopsError per
// interval, and only when there were new stops. So a plugin that the budget
// stops at each request logs one line per minute, not one per request.
const BudgetReportInterval = time.Minute

// BudgetStopsError is what GoPanic gets, with Options.QuietBudget, when the
// budget stopped more ServeHTTP calls of a machine since its last report
// (reportBudget). errors.Is(err, ErrBudget) is true.
type BudgetStopsError struct {
	Stops int64         // ServeHTTP calls stopped since the last report
	Total int64         // all of them since the load (Machine.BudgetStops)
	Since time.Duration // time since the last report
}

func (e *BudgetStopsError) Error() string {
	return fmt.Sprintf("%v: %d more requests of the plugin were stopped in the last %v (%d since the load; Machine.Stats)", ErrBudget, e.Stops, e.Since.Round(time.Second), e.Total)
}

// Unwrap returns ErrBudget.
func (e *BudgetStopsError) Unwrap() error { return ErrBudget }

// BudgetStops returns the number of ServeHTTP calls that the budget
// stopped (ErrBudget), for the metrics of the host.
func (m *Machine) BudgetStops() int64 { return m.budgetStops.Load() }

// cancelCtx returns ctx when a call from the host stops when its context is
// done (Options.Cancel), and otherwise a context that is never done: ctx
// without its cancellation when it carries an Instance (instance.go),
// context.Background() else.
func (m *Machine) cancelCtx(ctx context.Context) context.Context {
	if m.Cancel {
		return ctx
	}
	if m.instanceOf(ctx) != nil {
		return context.WithoutCancel(ctx)
	}
	return context.Background()
}
