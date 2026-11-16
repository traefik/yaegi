package vm

// Nested runs of the interpreter on one goroutine.
//
// Interpreted calls do not use the Go stack: they push a frame on the
// explicit frame stack of the ctx. But some calls start a new ctx on the
// same goroutine, under Go frames:
//   - native code that calls back into the VM (strings.Map, sort.Slice, a
//     String method called by fmt, an iterator, an http.Handler...), through
//     Machine.Call;
//   - a deferred interpreted call (runDefers).
// Each of these nested runs uses real Go stack (a few KB, see
// DefaultMaxNest). If the new ctx started from zero, a recursion through a
// callback would never reach MaxDepth, and gc would kill the process when
// the goroutine stack reaches 1 GB ("fatal error: stack overflow", which
// recover cannot catch).
//
// So the VM keeps, per goroutine, the innermost running ctx. A nested ctx
// starts with the depth of the ctx below it (its frames count against
// MaxDepth, its registers against MaxStack), counts one more nested run (limit MaxNest), and shares its
// budget (steps and limit), when both run for the same Machine (see
// budget.go for the calls from the host, which get their own budget).
//
// AOT code that recurses on the Go stack (the copy of errors on a cyclic
// error tree) counts each recursive call as a nested run too (NestAOT).

import (
	"sync"
	"sync/atomic"
)

// DefaultMaxNest is the default Machine.MaxNest. Measured with
// TestNestStackCost on linux/amd64, 10000 nested runs use at most 64 MB of
// Go stack through strings.Map or a String method called by fmt (3.4 to
// 6.7 KB per run), 32 MB through sort.Slice or deferred calls, 32 MB on
// linux/386, and twice as much with -race. This is far below the stack
// limit of gc (1 GB, 250 MB on 32-bit platforms): the native code between
// two runs may use up to 25 KB of stack before the limit is at risk. A
// recursion that goes this deep through native callbacks is very rare in
// real code.
const DefaultMaxNest = 10000

// gstate is the interpreter state of one goroutine. It lives in the ctx of
// the outermost run (ctx.g), so registering a goroutine does not allocate.
type gstate struct {
	cur *ctx // innermost ctx running on the goroutine
	// ctxAsker is the machine whose ctxValue runs a native Value call on
	// the goroutine (context.go).
	ctxAsker *Machine
	// fault is set when a run of the goroutine stops with ErrBudget or
	// ErrStackOverflow (or with errCanceledHard, errClosedHard). Native
	// code may recover the panic (fmt does it for String methods): the VM
	// raises it again when the native call returns and when each nested
	// run returns, so the outermost caller always gets it (reraise).
	fault error
	// The deferred calls of the frames that ErrBudget or ErrStackOverflow
	// stops run, as for a cancellation: faultLeft is the grace of steps
	// that they share (cancelGrace at the first fault), cleanup counts the
	// deferred calls that run for the fault (the fault is not raised again
	// inside them), and hard is set when the grace is spent: the fault is
	// then fatal (ctx.isFatal) and no deferred call runs any more.
	faultLeft int64
	cleanup   int
	hard      bool
	// hostEntry: the outermost run is a call that turns a Goexit into
	// the panic errGoexit (Plugin.Call, init), and goexit is set when a
	// Goexit of a callback of native code goes on as that panic
	// (runRecv). Native code may recover it: it is raised again as a
	// fault is.
	hostEntry bool
	goexit    bool
	// top is set by Machine.top while it runs: the runs nested directly on
	// topOuter are calls from the host, with the account pending
	// (budget.go).
	top      bool
	pending  *account
	topOuter *ctx
	// graceAcct is the canceled account whose grace the deferred calls of
	// the goroutine use, and graceLeft the steps left in that grace
	// (budget.go).
	graceAcct *account
	graceLeft int64
	slot      int // index in gkeys, or -1 in gspill
	// shadows are the AOT shadows made on the goroutine (twin_value.go,
	// shadowRef). Only the goroutine reads and writes them.
	shadows []*shadowRef
	// attaching are the shadows whose attach runs on the goroutine: a cell
	// that holds a pointer to itself (var a any; a = &a) meets its own
	// shadow before addShadow records it.
	attaching []*shadowRef
	// chanReaders counts the native calls running on the goroutine that
	// read the channels of their data (chanstandin.go, readsChans).
	chanReaders int
	// uregs are the rings of the regions that the goroutine converted to
	// uintptr, one per Machine (unsafe.go, goRing). Only the goroutine
	// reads and writes them.
	uregs []*goRegions
	// plain is the last panic of a callback that native code got as the
	// value of the program (nativePanic, panics.go).
	plain plainPanic
}

// The goroutines that run interpreted code are found by their key (getg)
// in gkeys/gvals: buckets of gWays slots (one cache line of keys), without
// a lock. Only goroutine g writes or reads the slot that holds key g, so a
// lookup is a few atomic loads. When the bucket of g is full, g goes to
// gspill, a map under a lock.
const (
	gWays    = 8
	gBuckets = 512
)

var (
	gkeys  [gBuckets * gWays]atomic.Uintptr
	gvals  [gBuckets * gWays]*gstate
	gspill struct {
		n  atomic.Int32
		mu sync.Mutex
		m  map[uintptr]*gstate
	}
)

func gbucket(g uintptr) int { return int(uint64(g>>6)*0x9E3779B97F4A7C15>>55) * gWays }

// lookupG returns the gstate of goroutine g, or nil if it runs no
// interpreted code.
func lookupG(g uintptr) *gstate {
	b := gbucket(g)
	for i := b; i < b+gWays; i++ {
		if gkeys[i].Load() == g {
			return gvals[i]
		}
	}
	if gspill.n.Load() > 0 {
		gspill.mu.Lock()
		defer gspill.mu.Unlock()
		return gspill.m[g]
	}
	return nil
}

// enterG returns the gstate of goroutine g. If the goroutine runs no
// interpreted code yet, cx becomes its outermost ctx and fresh is true.
func enterG(g uintptr, cx *ctx) (gs *gstate, fresh bool) {
	if gs = lookupG(g); gs != nil {
		return gs, false
	}
	b := gbucket(g)
	cx.g = gstate{slot: -1}
	gs = &cx.g
	for i := b; i < b+gWays; i++ {
		if gkeys[i].Load() == 0 && gkeys[i].CompareAndSwap(0, g) {
			gvals[i], gs.slot = gs, i
			return gs, true
		}
	}
	gspill.mu.Lock()
	if gspill.m == nil {
		gspill.m = map[uintptr]*gstate{}
	}
	gspill.m[g] = gs
	gspill.n.Add(1)
	gspill.mu.Unlock()
	return gs, true
}

// leaveG forgets goroutine g: its outermost run (gs) is over.
func leaveG(g uintptr, gs *gstate) {
	if len(gs.shadows) > 0 {
		gs.dropShadows()
	}
	if gs.slot >= 0 {
		gvals[gs.slot] = nil
		gkeys[gs.slot].Store(0)
		return
	}
	gspill.mu.Lock()
	delete(gspill.m, g)
	gspill.n.Add(-1)
	gspill.mu.Unlock()
}

// nestIn starts cx on top of outer, the innermost ctx of the same
// goroutine: cx continues its depth and, for the same Machine, its budget
// (but not for a call from the host, see Machine.top).
func (cx *ctx) nestIn(outer *ctx) {
	cx.gs = outer.gs
	cx.depth = outer.depth + len(outer.frames) + 1
	cx.nest = outer.nest + outer.aotNest + 1
	cx.maxFrames = cx.m.MaxDepth - cx.depth
	// outer.fn is nil for a goroutine that runs a native function (spawn).
	cx.regsBelow = outer.belowVals()
	cx.fitStack()
	top := cx.gs.top && outer == cx.gs.topOuter
	cx.ownBudget = outer.m != cx.m || top
	var pend *account
	if top {
		pend = cx.gs.pending
	}
	if !cx.ownBudget {
		// The callback counts one step, as a call opcode does: native
		// code that calls back in a loop (AOT code that walks a cyclic
		// error chain) must still reach the end of the budget, even when
		// the callback itself has no call and no loop.
		cx.steps, cx.limit = outer.steps+1, outer.limit
		cx.acct, cx.slice, cx.chunk, cx.rateEnd, cx.sliceAt = outer.acct, outer.slice, outer.chunk, outer.rateEnd, outer.sliceAt
		cx.inst = outer.inst
		if cx.steps >= cx.limit {
			cx.refill()
		}
	} else {
		// A call from the host (the account of Machine.top), or into
		// another machine (plugin): an account of its own.
		if pend == nil {
			//lint:ignore SA1012 nil: no context to cancel the call, and newAccount skips the Done check (hot callback path)
			pend = cx.m.newAccount(nil)
			cx.owned = pend
		}
		cx.useAcct(pend)
	}
	if cx.nest > cx.m.MaxNest || cx.maxFrames <= 0 ||
		cx.m.MaxStack > 0 && int64(cx.regsBelow)*valueSize >= cx.m.MaxStack {
		cx.overflow()
	}
}

// nestOut gives back to outer the budget used by cx, and closes the account
// that cx opened.
func (cx *ctx) nestOut(outer *ctx) {
	if !cx.ownBudget {
		outer.steps, outer.limit = cx.steps, cx.limit
		outer.acct, outer.slice, outer.chunk, outer.rateEnd, outer.sliceAt = cx.acct, cx.slice, cx.chunk, cx.rateEnd, cx.sliceAt
		if a := outer.acct; a != nil && a.rate > 0 {
			// With a BudgetRate, the end of a callback is a blocking
			// point: native code that calls a method of the plugin for
			// each part of a stream (the Write of a wrapper of the
			// ResponseWriter) waits between two calls (budget.go).
			outer.blocked()
		}
	} else {
		cx.giveBack() // the account may be shared with the next runs (Machine.top)
	}
	cx.owned.close()
}

// AOTNest is one recursive call of AOT code, counted by NestAOT.
type AOTNest struct{ cx *ctx }

// NestAOT counts one recursive call of AOT code (generated by cmd/aotgen) on
// the current goroutine. AOT code is compiled Go: a recursion that never
// ends (errors.Is on an error whose Unwrap() []error contains itself) grows
// the Go stack until gc kills the process. Like a nested run, each such
// call counts against MaxNest, and the goroutine stops with
// ErrStackOverflow when the limit is reached. The caller must call Done
// when the call returns:
//
//	defer vm.NestAOT().Done()
//
// On a goroutine that runs no interpreted code, NestAOT does nothing.
func NestAOT() AOTNest {
	gs := lookupG(getg())
	if gs == nil || gs.cur == nil {
		return AOTNest{}
	}
	cx := gs.cur
	if cx.nest+cx.aotNest >= cx.m.MaxNest {
		cx.overflow()
	}
	cx.aotNest++
	return AOTNest{cx}
}

// Done ends the call counted by NestAOT.
func (n AOTNest) Done() {
	if n.cx != nil {
		n.cx.aotNest--
	}
}

// overflow stops the goroutine with ErrStackOverflow.
func (cx *ctx) overflow() { cx.fail(ErrStackOverflow) }

// outOfBudget stops the goroutine with ErrBudget.
func (cx *ctx) outOfBudget() { cx.fail(ErrBudget) }

// fail stops the goroutine with err. errCanceledHard and errClosedHard
// stop it at once (budget.go, close.go). ErrBudget and ErrStackOverflow
// unwind it as ErrCanceled does: the deferred calls of the stopped frames
// run, so a deferred Unlock or wg.Done still runs, and recover cannot stop
// the panic (recoverable). They share a grace of cancelGrace steps
// (faultGrace): a deferred call that does not end stops once the grace is
// spent, without running the deferred calls that are left. A new
// ErrBudget or ErrStackOverflow during the deferred calls (a deferred
// call that overflows the stack again) panics again, and the deferred
// calls below it still run. Before, both errors skipped every deferred
// call: a plugin that ran over its budget under a lock wedged every later
// call that took the lock.
func (cx *ctx) fail(err error) {
	gs := cx.gs
	if gs == nil {
		panic(err)
	}
	if err == errCanceledHard || err == errClosedHard {
		gs.fault, gs.hard = err, true
		panic(err)
	}
	if gs.fault == nil {
		gs.fault, gs.faultLeft = err, cancelGrace
		panic(err)
	}
	// A new fault during the deferred calls of a fault: it costs
	// faultCost steps of the grace, because it may come before any step
	// (a deferred call that overflows the stack at once): the deferred
	// calls of a million pending defers each panicked for free.
	gs.faultLeft -= faultCost
	if gs.hard || gs.faultLeft <= 0 {
		gs.hard = true
		panic(gs.fault)
	}
	panic(err)
}

// faultCost is the cost, in steps of the grace, of a fault raised again
// during the deferred calls of a fault (fail).
const faultCost = 64

// faultGrace gives cx the next steps of the grace of its goroutine, which
// stops with a fault, or stops it for good when the grace is spent. The
// steps past the limit (the weight of a native call, which charge counts at
// once) are taken from the grace too, as for a cancel (grace, budget.go): a
// deferred regexp on a large input stops before it starts.
func (cx *ctx) faultGrace() {
	gs := cx.gs
	gs.faultLeft -= max(cx.steps-cx.limit, 0)
	n := min(gs.faultLeft, minChunk)
	if gs.hard || n <= 0 {
		gs.hard = true
		panic(gs.fault)
	}
	gs.faultLeft -= n
	cx.limit = cx.steps + n
}

// reraise raises again the fault (or the Goexit, hostEntry) of the
// goroutine: always once the grace of the fault is spent, and else outside
// the deferred calls that run for it.
func (gs *gstate) reraise() {
	if gs.fault != nil && (gs.hard || gs.cleanup == 0) {
		panic(gs.fault)
	}
	if gs.goexit && gs.cleanup == 0 {
		panic(errGoexit)
	}
}

// checkFault raises again a fault that native code recovered during the
// native call that just returned. On a 32-bit platform it also sign extends
// the int variables that flag may have written (flagvars.go).
func (cx *ctx) checkFault() {
	if gs := cx.gs; gs.fault != nil || gs.goexit {
		gs.reraise()
	}
	if intIs32 {
		cx.m.flagInts.fix() // flagvars.go
	}
}

// callIn runs fn on cx for Machine.Call: cx is the outermost run of
// goroutine g (outer is nil) or nested on outer.
func (cx *ctx) callIn(g uintptr, outer *ctx, fn *Func, args, env []Value) Value {
	gs := cx.gs
	gs.cur = cx
	defer func() {
		gs.cur = outer
		if outer == nil {
			leaveG(g, gs)
			cx.giveBack() // the account may be shared with the next runs (Machine.top)
			cx.owned.close()
		} else {
			cx.nestOut(outer)
		}
	}()
	rv := cx.run(fn, args, env)
	gs.reraise()
	return rv
}
