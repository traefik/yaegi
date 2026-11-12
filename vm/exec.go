package vm

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"math"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"sync"
	"unsafe"
)

type frame struct {
	fn   *Func
	pc   int
	base int
	dst  int32
}

// ctx is one thread of interpreted execution: an explicit register stack and
// an explicit frame stack (no Go recursion for interpreted calls).
type ctx struct {
	m      *Machine
	stack  []Value
	frames []frame
	steps  int64
	limit  int64
	fn     *Func // current frame (kept up to date on call/return for unwinding)
	base   int
	defers []deferred
	panic  *panicState // set when this ctx runs a deferred call during a panic
	hw     int         // high-water mark of the register stack
	// freePage is the last page (address >> hostPageShift) without host
	// memory that a byte store of a restricted machine checked (hostByte).
	freePage uintptr
	// at is the index in fn.code of the instruction being executed: the
	// position of the innermost frame for the stack walks (callers.go).
	at int
	// parent is the context below this one on the goroutine, for the
	// same Machine: the context whose deferred call this one runs, or the
	// one that called the native code that calls back into the VM (nest.go
	// keeps the innermost context of a goroutine). nil: the context starts
	// a goroutine or a call from the host; kind tells which one.
	parent *ctx
	kind   ctxKind
	// native: parent called native code that called back into the VM
	// (else this context runs a deferred call of parent). unwinding: the
	// deferred call runs during a panic or a Goexit of parent. The GC walk
	// of gclive.go uses them: it clears the registers of parent at
	// parent.at+1, or with the liveness of a panicking frame.
	native    bool
	unwinding bool
	// ptrace holds the stack of the panic in flight, for the traceback
	// of an unrecovered panic (only in RunMain and goroutine contexts).
	ptrace []uintptr
	esc    escape // the last panic that left a nested context (callers.go)
	// unwound holds the frames that a panic has left, innermost first,
	// while it runs the deferred calls of the frames below them: gc keeps
	// these frames on the stack until a recover.
	unwound []frameAt
	// hostGoexit: the ctx was entered from native code (Machine.Call), so
	// a runtime.Goexit that reaches its first frame calls the real
	// runtime.Goexit (see run).
	hostGoexit bool

	// Nested runs on one goroutine (nest.go).
	gs        *gstate // state of the goroutine
	g         gstate  // the state itself, when this ctx is the outermost run
	depth     int     // frames of the ctxs below this one on the goroutine
	nest      int     // number of ctxs below this one on the goroutine
	maxFrames int     // MaxDepth - depth: frames allowed on this ctx
	regsBelow int     // Values used by the ctxs below: registers, cells, defers (MaxStack)
	// cells and deferVals are the Values of the frame-local cells of the
	// live frames and of the deferred calls, counted in MaxStack
	// (stacklimit.go). Both can be too large, never too small.
	cells     int64
	deferVals int
	// charged is the part of cells and deferVals, in bytes, counted in the
	// memory quota (Options.MaxMemory), held by the tokens of chargeTok
	// (stacklimit.go).
	charged   int64
	chargeTok []*[16]byte
	ownBudget bool // does not share the budget of the ctx below (nestIn)

	// The cells of the frame-local Allocs of the frames that returned,
	// set to zero, per site: the next OpAllocLocal of the site takes one
	// (escape.go). They stay with the ctx in the pool.
	freeCells []freeList // by memSite.local
	freeSize  int        // Values held by freeCells

	// The budget (budget.go).
	acct    *account // account of the call from the host (nil: background, or no budget)
	owned   *account // account that this ctx opened, closed when its run ends
	slice   int64    // steps at the last blocking point (background rule)
	chunk   int64    // size of the next chunk taken from acct
	rateEnd int64    // background: steps where the chunk of BackgroundRate ends
	aotNest int      // recursive AOT calls running on top of this ctx (NestAOT)
}

var traceVM = os.Getenv("VM_TRACE") != ""

var ErrBudget = errors.New("vm: execution budget exhausted")

// ErrStackOverflow stops a goroutine whose stack passes one of its limits:
// MaxDepth frames, MaxStack bytes or MaxNest nested runs.
var ErrStackOverflow = errors.New("vm: stack overflow (MaxDepth, MaxStack or MaxNest)")

// VMPanic is an interpreted panic(v).
type VMPanic struct{ V Value }

func (p *VMPanic) Error() string { return "panic: " + displayIface(p.V) }

func displayIface(v Value) string {
	iv, _ := v.R.(*Iface)
	if iv == nil {
		return "nil"
	}
	if iv.T.Native != nil {
		return fmt.Sprint(iv.V.R)
	}
	s := formatBasic(iv.V, iv.T.T)
	if _, isNamed := iv.T.T.(*types.Named); isNamed {
		return iv.T.T.String() + "(" + s + ")"
	}
	if b := basic(iv.T.T); b != nil && b.Info()&types.IsString != 0 {
		return s
	}
	return s
}

func formatBasic(v Value, t types.Type) string {
	b := basic(t)
	if b == nil {
		return fmt.Sprintf("%v", v.R)
	}
	switch {
	case b.Info()&types.IsString != 0:
		s, _ := v.R.(string)
		return s
	case b.Info()&types.IsBoolean != 0:
		return fmt.Sprint(v.N != 0)
	case b.Info()&types.IsComplex != 0:
		return fmt.Sprint(cplx(v))
	case b.Kind() == types.UnsafePointer:
		return fmt.Sprintf("%#x", addrOf(v))
	case b.Info()&types.IsFloat != 0:
		return fmt.Sprint(math.Float64frombits(uint64(v.N)))
	case b.Info()&types.IsUnsigned != 0:
		return fmt.Sprint(uint64(v.N))
	}
	return fmt.Sprint(v.N)
}

// Call runs fn with args (and closure env) on a fresh execution context.
// It is safe to call concurrently (e.g. from native callbacks). When the
// goroutine already runs interpreted code (a callback of native code), the
// new context continues its depth and its budget (nest.go). If fn calls
// runtime.Goexit, Call runs the deferred calls and then calls the real
// runtime.Goexit: the goroutine of the caller ends, as with gc.
func (m *Machine) Call(fn *Func, args []Value, env []Value) Value {
	return m.callCtx(fn, args, env, nil, true)
}

// call is Call. With hostGoexit false, a runtime.Goexit of fn ends the
// call with a Go panic of errGoexit instead (for the callers that handle it:
// RunMain, the Plugin methods).
func (m *Machine) call(fn *Func, args []Value, env []Value, hostGoexit bool) Value {
	return m.callCtx(fn, args, env, nil, hostGoexit)
}

// callP is Call for a function called directly by a deferred call while
// the panic p runs (nil: no panic): recover in fn stops p.
func (m *Machine) callP(fn *Func, args []Value, env []Value, p *panicState) Value {
	return m.callCtx(fn, args, env, p, true)
}

// callCtx runs fn on a fresh ctx that sees the panic p (see callP) and
// handles a runtime.Goexit as hostGoexit says (see call).
func (m *Machine) callCtx(fn *Func, args []Value, env []Value, p *panicState, hostGoexit bool) Value {
	if intIs32 {
		// a callback of native code (flag.Parse calls a Func flag) sees
		// the int variables that flag wrote before it (flagvars.go)
		m.flagInts.fix()
	}
	var cx *ctx
	if optCtxPool {
		cx = getCtx(m, p)
	} else {
		cx = &ctx{m: m, limit: math.MaxInt64, panic: p, maxFrames: m.MaxDepth}
	}
	g := getg()
	gs, fresh := enterG(g, cx)
	var outer *ctx
	if fresh {
		cx.gs = gs
		// A call from the host: the account of top (shared by all the
		// runs of the call), or an account of its own (a callback on a
		// goroutine of native code).
		a := peekPending(g)
		if a == nil {
			// Not under top: the host calls a func value or a method of the
			// plugin, or native code calls it on a goroutine of its own. It
			// counts for the drain of CloseContext, as a call of top does.
			m.inflight.Add(1)
			defer m.inflight.Add(-1)
			//lint:ignore SA1012 nil: no context to cancel the call, and newAccount skips the Done check (hot callback path)
			a = m.newAccount(nil)
			cx.owned = a
		}
		cx.useAcct(a)
	} else {
		if gs.fault != nil {
			// Native code recovered a fault and calls back again: the
			// execution is over (budget.go).
			panic(gs.fault)
		}
		outer = gs.cur
		cx.nestIn(outer)
		if outer.m == m {
			cx.parent, cx.native = outer, true // the stack walks go on below (callers.go)
		}
	}
	cx.hostGoexit = hostGoexit
	var rv Value
	if fresh && m.GoPanic != nil {
		// A callback on a goroutine that native code started (nativego.go).
		var caught bool
		if rv, caught = cx.callOutermost(g, fn, args, env); caught {
			return rv // the ctx stopped in the middle: do not reuse it
		}
	} else {
		if fresh {
			cx.refuseClosed(g)
		}
		rv = cx.callIn(g, outer, fn, args, env)
	}
	if optCtxPool {
		putCtx(cx)
	}
	return rv
}

// mainCtx returns the context of the main goroutine of a program
// (RunMain): its stack ends with runtime.main, as in gc.
func (m *Machine) mainCtx() *ctx {
	cx := &ctx{m: m, limit: math.MaxInt64, maxFrames: m.MaxDepth, kind: ctxMain}
	//lint:ignore SA1012 nil: the main goroutine has no context to cancel it
	cx.owned = m.newAccount(nil)
	cx.useAcct(cx.owned)
	return cx
}

// runMain runs fn on cx, the context of mainCtx, as the outermost run of
// the goroutine (nest.go).
func (cx *ctx) runMain(fn *Func) {
	noteMainPC()
	g := getg()
	cx.gs, _ = enterG(g, cx)
	cx.callIn(g, nil, fn, nil, nil)
}

// Optimization 1: execution contexts (register stack + frame stack) are
// pooled instead of allocated per native->VM call.
//
// A new ctx has no register stack: the first run gives it one sized from
// the frame of the entry function (grow), and calls double it when they need
// more. A ctx lives as long as its call: a fixed stack of 4096 Values (96
// KiB) per in-flight request, or per nested callback, cost hundreds of MB
// with thousands of slow requests.
var ctxPool = sync.Pool{New: func() any { return &ctx{} }}

func getCtx(m *Machine, p *panicState) *ctx {
	cx := ctxPool.Get().(*ctx)
	cx.m, cx.limit, cx.steps, cx.panic, cx.hostGoexit = m, math.MaxInt64, 0, p, false
	cx.gs, cx.depth, cx.nest, cx.maxFrames, cx.regsBelow = nil, 0, 0, m.MaxDepth, 0
	cx.cells, cx.deferVals, cx.charged, cx.chargeTok = 0, 0, 0, nil
	cx.fitStack()
	cx.frames, cx.defers = cx.frames[:0], cx.defers[:0]
	cx.parent, cx.kind, cx.ptrace, cx.unwound, cx.esc = nil, ctxNative, nil, cx.unwound[:0], escape{}
	cx.native, cx.unwinding, cx.aotNest = false, false, 0
	cx.acct, cx.owned, cx.slice, cx.chunk, cx.rateEnd = nil, nil, 0, 0, 0
	return cx
}

func putCtx(cx *ctx) {
	if len(cx.stack) > pooledRegs || cap(cx.frames) > pooledStack || cap(cx.defers) > pooledStack {
		return // a deep recursion grew the stacks: let the GC free them
	}
	clear(cx.stack[:min(cx.hw, len(cx.stack))]) // drop references (GC hygiene)
	cx.hw = 0
	clear(cx.defers[:cap(cx.defers)])
	cx.m, cx.fn, cx.panic, cx.gs, cx.parent = nil, nil, nil, nil, nil
	cx.acct, cx.owned = nil, nil
	cx.charged, cx.chargeTok = 0, nil
	ctxPool.Put(cx)
}

// pooledStack is the largest frame stack kept by the ctx pool.
const pooledStack = 1 << 16

// pooledRegs is the largest register stack kept by the ctx pool (96 KiB):
// a ctx that a deep recursion grew past it goes to the GC.
const pooledRegs = 4096

// minStack is the smallest register stack that grow makes (1.5 KiB): the
// first calls of a small handler then fit without a second growth.
const minStack = 64

// valueSize is the size in bytes of a Value.
const valueSize = int64(unsafe.Sizeof(Value{}))

// grow makes the register stack hold at least n Values. It doubles the
// size, so a deep call chain grows it a few times only (amortized). The
// frames keep offsets into cx.stack, never pointers: the callers re-slice
// their registers after a call that may grow (exec).
func (cx *ctx) grow(n int) {
	size := max(2*n, 2*len(cx.stack), minStack)
	if limit := cx.m.MaxStack; limit > 0 {
		// The stacks of the ctxs below on the goroutine count too
		// (nest.go), and the cells and deferred calls (stacklimit.go).
		cx.checkStack(n)
		size = int(max(int64(n), min(int64(size), limit/valueSize-cx.stackVals(0))))
	}
	if m := cx.m; m.mem != nil && int64(size)*valueSize >= memMin {
		// A large register stack counts in the memory quota too: each
		// goroutine has one (Options.MaxMemory).
		m.memCheck(int64(size) * valueSize)
		ns := make([]Value, size)
		copy(ns, cx.stack)
		cx.stack = ns
		m.memSlice(ns)
		return
	}
	ns := make([]Value, size)
	copy(ns, cx.stack)
	cx.stack = ns
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func f64(v Value) float64 { return math.Float64frombits(uint64(v.N)) }
func fv(f float64) int64  { return int64(math.Float64bits(f)) }

// convNum converts the number n between an integer and a float type. kinds
// holds the source basic kind in its low byte and the target kind in the next
// byte (see numKinds). Each case uses the Go conversion of the same types, so
// rounding and out-of-range results are the ones of compiled code on the
// host (for example int32(+Inf) on amd64).
func convNum(n int64, kinds int32) int64 {
	from, to := types.BasicKind(kinds&0xff), types.BasicKind(kinds>>8)
	switch from {
	case types.Float32:
		return f2i(float32(math.Float64frombits(uint64(n))), to)
	case types.Float64, types.UntypedFloat:
		return f2i(math.Float64frombits(uint64(n)), to)
	case types.Uint, types.Uint64, types.Uintptr:
		// The top bit is a value bit, not a sign.
		if to == types.Float32 {
			return fv(float64(float32(uint64(n))))
		}
		return fv(float64(uint64(n)))
	default:
		// Other integers are stored sign- or zero-extended: n is the value.
		if to == types.Float32 {
			return fv(float64(float32(n)))
		}
		return fv(float64(n))
	}
}

// f2i converts the float f to the integer type to. It is generic so that
// float32 and float64 sources share one list of targets, and each case still
// uses the Go conversion from the source float type.
func f2i[F float32 | float64](f F, to types.BasicKind) int64 {
	switch to {
	case types.Int8:
		return int64(int8(f))
	case types.Int16:
		return int64(int16(f))
	case types.Int32:
		return int64(int32(f))
	case types.Int:
		return int64(int(f))
	case types.Int64:
		return int64(f)
	case types.Uint8:
		return int64(uint8(f))
	case types.Uint16:
		return int64(uint16(f))
	case types.Uint32:
		return int64(uint32(f))
	case types.Uint:
		return int64(uint(f))
	case types.Uintptr:
		return int64(uintptr(f))
	case types.Uint64:
		return int64(uint64(f))
	}
	panic(fmt.Sprintf("vm: bad float to integer conversion to %v", to))
}

func (cx *ctx) run(fn *Func, args []Value, env []Value) Value {
	return cx.runRecv(fn, nil, args, env)
}

// runRecv is run with a receiver (non-nil recv) before the arguments: a
// deferred method call, with no copy of the receiver and the arguments
// into one slice.
func (cx *ctx) runRecv(fn *Func, recv *Value, args []Value, env []Value) Value {
	fn.ensure()
	if fn.nregs > len(cx.stack) {
		cx.grow(fn.nregs)
	}
	regs := cx.stack[0:fn.nregs]
	cx.hw = max(cx.hw, fn.nregs)
	copy(regs, fn.template)
	n := 0
	if recv != nil {
		regs[0] = *recv
		n = 1
	}
	n += copy(regs[n:], args)
	copy(regs[n:], env)
	cx.fn, cx.base = fn, 0
	pc := 0
	for {
		rv, p := cx.protected(pc)
		if p == nil {
			if cx.parent == nil {
				cx.ptrace, cx.esc = nil, escape{} // the goroutine's run is over
			}
			return rv
		}
		cx.notePanic(p.val)
		if p.fatal {
			cx.escape(p.val)
			panic(p.val)
		}
		// Unwind interpreted frames, running their defers (explicit frames make
		// this a plain loop: no Go stack involved).
		for {
			if cx.panicDefers(p) {
				if !p.goexit {
					pc = cx.fn.recoverPC
					cx.recovered()
					break
				}
				// The recovered panic started during a Goexit: as with
				// gc, the Goexit goes on (the function does not
				// return to its caller).
				p.val, p.recovered = errGoexit, false
			}
			if len(cx.frames) == 0 {
				if p.val == errGoexit && cx.hostGoexit {
					// The deferred calls ran: end the goroutine like
					// gc. Native code must not see errGoexit as a
					// normal panic (it could recover or print it).
					runtime.Goexit()
				}
				cx.unwound = cx.unwound[:0]
				// The panic leaves the VM: a native sentinel goes on as
				// itself, for host code that compares it (hostPanic).
				val := hostPanic(p.val)
				if cx.native {
					// native code called back into the VM: it
					// recovers the value of the program
					val = cx.m.nativePanic(val)
				}
				cx.escape(val)
				panic(val)
			}
			cx.unwound = append(cx.unwound, frameAt{cx.fn, cx.at})
			f := cx.frames[len(cx.frames)-1]
			cx.frames = cx.frames[:len(cx.frames)-1]
			cx.fn, cx.base, cx.at = f.fn, f.base, f.pc-1
		}
	}
}

type panicState struct {
	val       any // VMPanic or native panic value
	recovered bool
	goexit    bool // a runtime.Goexit is pending under a newer panic
	fatal     bool // ErrBudget or ErrStackOverflow: no interpreted recover
	// exited: a real runtime.Goexit left a deferred call that this panic
	// runs (a native callback of the deferred call called it).
	exited bool
}

type deferred struct {
	depth int
	fn    *Func
	env   []Value
	recv  *Value
	args  []Value
	nat   func()
	// recover marks a deferred call of the recover builtin.
	recover bool
	// natP is a Go call that gets the panic that runs it (Closure.GoP).
	natP func(p *panicState)
}

// isFatal reports whether r is a VM error that no recover() may stop: the
// sandbox relies on them to stop a plugin (Options.Budget, MaxDepth).
func isFatal(r any) bool {
	return r == ErrBudget || r == ErrStackOverflow || r == errCanceledHard || r == errClosedHard
}

// recoverable reports whether recover() may stop the panic value r.
// runtime.Goexit is not a panic for recover, ErrCanceled and ErrClosed stop
// the call like a Goexit (budget.go, close.go), and isFatal values must
// reach the host (runDefers never stores them; this is a second guard).
func recoverable(r any) bool {
	return r != errGoexit && r != ErrCanceled && r != ErrClosed && !isFatal(r)
}

func (cx *ctx) protected(pc int) (rv Value, p *panicState) {
	returned := false
	defer func() {
		r := recover()
		if r == nil && !returned {
			// A real runtime.Goexit, called by native code (or by a VM
			// callback that native code called): recover() does not see
			// it. Run the deferred calls of the interpreted frames now;
			// the Goexit goes on when this function returns.
			cx.goexitUnwind()
			return
		}
		if r != nil {
			if isFatal(r) {
				// Raised again by run, once this recover is complete: a
				// panic raised here would start on top of the frames that
				// are not unwound yet, and the Go runtime would walk them
				// again at each nested run (quadratic time, see nest.go).
				p = &panicState{val: r, fatal: true}
				return
			}
			if traceVM {
				fmt.Fprintf(os.Stderr, "vm panic %v in %s\n%s\n", r, cx.fn.ssa, debug.Stack())
			}
			p = &panicState{val: restorePanic(r)}
		}
	}()
	rv = cx.exec(pc)
	returned = true
	return rv, nil
}

// panicDefers is runDefers for the frame on top of cx while the panic p
// runs. run calls it outside protected, so a real runtime.Goexit called in a
// deferred call (by native code, or by a VM callback that native code runs)
// must be handled here: as in gc, the Goexit ends the panic and runs the
// deferred calls of all the frames, then the goroutine ends. Without this,
// the deferred calls of the frames below were lost (a deferred wg.Done never
// ran: deadlock).
func (cx *ctx) panicDefers(p *panicState) (recovered bool) {
	defer func() {
		if p.exited {
			p.exited = false
			cx.goexitUnwind()
		}
	}()
	return cx.runDefers(len(cx.frames), p, true)
}

// goexitUnwind runs the deferred calls of all the frames of cx during a real
// runtime.Goexit. recover() does not see the Goexit. A new panic of a
// deferred call that nobody recovers goes on as a panic.
func (cx *ctx) goexitUnwind() {
	p := &panicState{val: errGoexit}
	defer func() {
		if p.exited {
			// One more real Goexit in a deferred call: the deferred calls
			// of the frames that are left still run.
			p.exited = false
			cx.goexitUnwind()
		}
	}()
	for {
		if cx.runDefers(len(cx.frames), p, true) {
			p.val, p.recovered = errGoexit, false // the Goexit goes on
		}
		if len(cx.frames) == 0 {
			break
		}
		cx.unwound = append(cx.unwound, frameAt{cx.fn, cx.at})
		f := cx.frames[len(cx.frames)-1]
		cx.frames = cx.frames[:len(cx.frames)-1]
		cx.fn, cx.base, cx.at = f.fn, f.base, f.pc-1
	}
	cx.unwound = cx.unwound[:0]
	if p.val != errGoexit {
		panic(p.val)
	}
}

// runDefers runs, LIFO, the defers registered by the frame at depth.
// unwinding: the frame is panicking (or in a Goexit); else it returns.
// It reports whether the current panic (if any) was recovered.
func (cx *ctx) runDefers(depth int, p *panicState, unwinding bool) (recovered bool) {
	for len(cx.defers) > 0 && cx.defers[len(cx.defers)-1].depth == depth {
		d := cx.defers[len(cx.defers)-1]
		cx.defers[len(cx.defers)-1] = deferred{} // drop references
		cx.defers = cx.defers[:len(cx.defers)-1]
		if d.recover {
			// defer recover() acts like a recover call in the function that
			// deferred it, at its normal return: it stops the panic when that
			// function was called directly by the panic (recover.go test4).
			// The function also returns normally when a newer panic of the
			// same frame was recovered by an earlier deferred call
			// (recover1.go test6). While that newer panic is still active,
			// it does nothing (recover1.go test7).
			// It never stops a Goexit, ErrBudget or ErrStackOverflow.
			if (p == nil || p.recovered) && depth == 0 && cx.panic != nil && !cx.panic.recovered && recoverable(cx.panic.val) {
				cx.panic.recovered = true
			}
			continue
		}
		var again any // panic to raise again once recovered (see protected)
		done := false
		func() {
			defer func() {
				r := recover()
				if r == nil && !done && p != nil {
					// A real runtime.Goexit left the deferred call: it
					// goes on when this function returns (panicDefers).
					p.exited = true
					return
				}
				if r != nil {
					if p == nil || isFatal(r) {
						// ErrBudget and ErrStackOverflow never become
						// the current panic: recover() could stop them.
						again = r
						return
					}
					if p.val == errGoexit {
						p.goexit = true
					}
					p.val, p.recovered = r, false // a new panic replaces the old one
				}
			}()
			if d.natP != nil {
				d.natP(p)
				done = true
				cx.checkFault()
				return
			}
			if d.nat != nil {
				d.nat()
				done = true
				cx.checkFault()
				return
			}
			var sub *ctx
			if optCtxPool {
				sub = getCtx(cx.m, p)
				defer putCtx(sub)
			} else {
				sub = &ctx{m: cx.m, stack: make([]Value, 256), panic: p}
			}
			sub.parent, sub.unwinding = cx, unwinding
			// The deferred call runs under the Go frames of this ctx: it
			// continues its depth and its budget.
			sub.nestIn(cx)
			gs := cx.gs
			gs.cur = sub
			defer func() { gs.cur = cx; sub.nestOut(cx) }()
			sub.runRecv(d.fn, d.recv, d.args, d.env)
			done = true
		}()
		if again != nil {
			panic(again)
		}
	}
	return p != nil && p.recovered
}

// directlyDeferred reports whether the current function was called directly
// by the deferred call that runs it: the frames below it in this context, if
// any, are method wrappers (defer x.M(), defer f() with f := x.M, a method
// called through reflect). recover only stops a panic in such a function.
func (cx *ctx) directlyDeferred() bool {
	for _, f := range cx.frames {
		if !f.fn.wrapper {
			return false
		}
	}
	return true
}

func (cx *ctx) exec(pc int) Value {
	// track: keep cx.at up to date at each instruction, so that the stack
	// of a panic knows the instruction that panicked (callers.go). A
	// machine that does not need it pays a predicted branch per step.
	track := cx.m.track
	m := cx.m
	fn, base := cx.fn, cx.base
	regs := cx.stack[base : base+fn.nregs]
	code := fn.code
	for {
		if track {
			cx.at = pc
		}
		in := &code[pc]
		pc++
		if profOps {
			OpCount[in.Op]++
			fn.icount++
		}
		switch in.Op {
		case OpNop:
		case OpMov:
			regs[in.A] = regs[in.B]
		case OpJmp:
			if int(in.A) < pc {
				cx.steps++
				if cx.steps >= cx.limit {
					cx.refill()
				}
			}
			pc = int(in.A)
		case OpMovJmp:
			regs[in.A] = regs[in.B]
			if int(in.C) < pc {
				cx.steps++
				if cx.steps >= cx.limit {
					cx.refill()
				}
			}
			pc = int(in.C)
		case OpSwitch:
			t := in.X.(*switchSite).target(regs[in.A].N)
			if t < pc {
				cx.step()
			}
			pc = t
		case OpIf:
			if regs[in.A].N != 0 {
				if int(in.B) < pc {
					cx.step()
				}
				pc = int(in.B)
			} else {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpAddI:
			regs[in.A].N = regs[in.B].N + regs[in.C].N
		case OpSubI:
			regs[in.A].N = regs[in.B].N - regs[in.C].N
		case OpMulI:
			regs[in.A].N = regs[in.B].N * regs[in.C].N
		case OpDivI:
			regs[in.A].N = regs[in.B].N / regs[in.C].N
		case OpRemI:
			regs[in.A].N = regs[in.B].N % regs[in.C].N
		case OpDivU:
			regs[in.A].N = int64(uint64(regs[in.B].N) / uint64(regs[in.C].N))
		case OpRemU:
			regs[in.A].N = int64(uint64(regs[in.B].N) % uint64(regs[in.C].N))
		case OpAndI:
			regs[in.A].N = regs[in.B].N & regs[in.C].N
		case OpOrI:
			regs[in.A].N = regs[in.B].N | regs[in.C].N
		case OpXorI:
			regs[in.A].N = regs[in.B].N ^ regs[in.C].N
		case OpAndNotI:
			regs[in.A].N = regs[in.B].N &^ regs[in.C].N
		case OpShlI:
			if in.F != 0 && regs[in.C].N < 0 {
				panic(errNegShift)
			}
			regs[in.A].N = regs[in.B].N << uint64(regs[in.C].N)
		case OpShrI:
			if in.F != 0 && regs[in.C].N < 0 {
				panic(errNegShift)
			}
			regs[in.A].N = regs[in.B].N >> uint64(regs[in.C].N)
		case OpShrU:
			if in.F != 0 && regs[in.C].N < 0 {
				panic(errNegShift)
			}
			regs[in.A].N = int64(uint64(regs[in.B].N) >> uint64(regs[in.C].N))
		case OpEqI:
			regs[in.A].N = b2i(regs[in.B].N == regs[in.C].N)
		case OpNeI:
			regs[in.A].N = b2i(regs[in.B].N != regs[in.C].N)
		case OpLtI:
			regs[in.A].N = b2i(regs[in.B].N < regs[in.C].N)
		case OpLeI:
			regs[in.A].N = b2i(regs[in.B].N <= regs[in.C].N)
		case OpGtI:
			regs[in.A].N = b2i(regs[in.B].N > regs[in.C].N)
		case OpGeI:
			regs[in.A].N = b2i(regs[in.B].N >= regs[in.C].N)
		case OpLtU:
			regs[in.A].N = b2i(uint64(regs[in.B].N) < uint64(regs[in.C].N))
		case OpLeU:
			regs[in.A].N = b2i(uint64(regs[in.B].N) <= uint64(regs[in.C].N))
		case OpGtU:
			regs[in.A].N = b2i(uint64(regs[in.B].N) > uint64(regs[in.C].N))
		case OpGeU:
			regs[in.A].N = b2i(uint64(regs[in.B].N) >= uint64(regs[in.C].N))
		case OpAddF:
			regs[in.A].N = fv(f64(regs[in.B]) + f64(regs[in.C]))
		case OpSubF:
			regs[in.A].N = fv(f64(regs[in.B]) - f64(regs[in.C]))
		case OpMulF:
			regs[in.A].N = fv(f64(regs[in.B]) * f64(regs[in.C]))
		case OpDivF:
			regs[in.A].N = fv(f64(regs[in.B]) / f64(regs[in.C]))
		case OpLtF:
			regs[in.A].N = b2i(f64(regs[in.B]) < f64(regs[in.C]))
		case OpLeF:
			regs[in.A].N = b2i(f64(regs[in.B]) <= f64(regs[in.C]))
		case OpGtF:
			regs[in.A].N = b2i(f64(regs[in.B]) > f64(regs[in.C]))
		case OpGeF:
			regs[in.A].N = b2i(f64(regs[in.B]) >= f64(regs[in.C]))
		case OpEqF:
			regs[in.A].N = b2i(f64(regs[in.B]) == f64(regs[in.C]))
		case OpAddS:
			x, _ := regs[in.B].R.(string)
			y, _ := regs[in.C].R.(string)
			if m.mem != nil && len(x)+len(y) >= memMin {
				m.memCheck(int64(len(x) + len(y)))
				xy := x + y
				m.memString(xy)
				regs[in.A] = Value{R: xy}
				break
			}
			regs[in.A] = Value{R: x + y}
		case OpEqS, OpNeS, OpLtS:
			x, _ := regs[in.B].R.(string)
			y, _ := regs[in.C].R.(string)
			var r bool
			switch in.Op {
			case OpEqS:
				r = x == y
			case OpNeS:
				r = x != y
			default:
				r = x < y
			}
			regs[in.A] = Value{N: b2i(r)}
		case OpEqV:
			if pc, ok := in.X.(*ptrCmp); ok {
				regs[in.A] = Value{N: b2i(pc.equal(regs[in.B], regs[in.C]))}
				break
			}
			regs[in.A] = Value{N: b2i(m.equal(regs[in.B], regs[in.C], in.X.(types.Type)))}
		case OpNeV:
			if pc, ok := in.X.(*ptrCmp); ok {
				regs[in.A] = Value{N: b2i(!pc.equal(regs[in.B], regs[in.C]))}
				break
			}
			regs[in.A] = Value{N: b2i(!m.equal(regs[in.B], regs[in.C], in.X.(types.Type)))}
		case OpNegI:
			regs[in.A].N = -regs[in.B].N
		case OpNegF:
			regs[in.A].N = fv(-f64(regs[in.B]))
		case OpNot:
			regs[in.A].N = regs[in.B].N ^ 1
		case OpCompl:
			regs[in.A].N = ^regs[in.B].N
		case OpTrunc:
			n := regs[in.B].N
			switch types.BasicKind(in.C) {
			case types.Int8:
				n = int64(int8(n))
			case types.Int16:
				n = int64(int16(n))
			case types.Int32:
				n = int64(int32(n))
			case types.Uint8:
				n = int64(uint8(n))
			case types.Uint16:
				n = int64(uint16(n))
			case types.Uint32:
				n = int64(uint32(n))
			case types.Float32:
				n = fv(float64(float32(f64(regs[in.B]))))
			}
			regs[in.A].N = n
		case OpI2F:
			// C == 0: the common case, a signed integer to float64.
			if in.C == 0 {
				regs[in.A].N = fv(float64(regs[in.B].N))
			} else {
				regs[in.A].N = convNum(regs[in.B].N, in.C)
			}
		case OpF2I:
			// C == 0: the common case, float64 to int64 (or a 64-bit int).
			if in.C == 0 {
				regs[in.A].N = int64(f64(regs[in.B]))
			} else {
				regs[in.A].N = convNum(regs[in.B].N, in.C)
			}
		case OpStr2Bytes:
			s, _ := regs[in.B].R.(string)
			if optBytes {
				if in.C != 0 {
					// a constant string: cap == len, as with gc
					b := make([]byte, len(s))
					copy(b, s)
					regs[in.A] = Value{R: b}
					break
				}
				regs[in.A] = Value{R: []byte(s)}
				break
			}
			out := make([]Value, len(s))
			for i := 0; i < len(s); i++ {
				out[i].N = int64(s[i])
			}
			regs[in.A] = Value{R: out}
		case OpBytes2Str:
			if b, ok := regs[in.B].R.([]byte); ok {
				regs[in.A] = Value{R: string(b)}
				break
			}
			bs, _ := regs[in.B].R.([]Value)
			b := make([]byte, len(bs))
			for i := range bs {
				b[i] = byte(bs[i].N)
			}
			regs[in.A] = Value{R: string(b)}
		case OpRune2Str:
			regs[in.A] = Value{R: intToStr(regs[in.B].N)}
		case OpStr2Runes:
			regs[in.A] = strToRunes(regs[in.B])
		case OpRunes2Str:
			regs[in.A] = runesToStr(regs[in.B])

		case OpCall, OpCallClosure, OpInvoke:
			var callee *Func
			var env []Value
			var recv *Value
			var argRegs []int32
			switch in.Op {
			case OpCall:
				cs := in.X.(*callSite)
				callee, argRegs = cs.fn, cs.args
			case OpCallClosure:
				cs := in.X.(*callSite)
				argRegs = cs.args
				switch cl := regs[in.B].R.(type) {
				case *Closure:
					if cl.Go != nil {
						av := make([]Value, len(argRegs))
						for i, r := range argRegs {
							av[i] = regs[r]
						}
						put(regs, in, cl.Go(av))
						cx.checkFault()
						continue
					}
					callee, env = cl.Fn, cl.Env
				case reflect.Value:
					put(regs, in, m.callNativeValue(cl, cs.sig, regs, argRegs))
					cx.checkFault()
					continue
				default:
					panic(errNilDeref)
				}
			case OpInvoke:
				s := in.X.(*invokeSite)
				argRegs = s.args
				iv, _ := regs[in.B].R.(*Iface)
				if iv == nil {
					panic(errNilDeref)
				}
				if m.vr != nil && iv.T == m.vr.typeRT {
					put(regs, in, m.vr.invoke(iv.V.R.(*vtype), s.name, regs, argRegs))
					cx.checkFault()
					continue
				}
				if iv.T.Native != nil {
					if s.memCheck {
						m.nativeMemInvoke(nil, iv.V, s.name, regs, argRegs)
					}
					if s.name == "Value" && len(argRegs) == 1 {
						if ctx, ok := natAny(regs[in.B]).(context.Context); ok {
							put(regs, in, m.ctxValue(ctx, regs[argRegs[0]]))
							cx.checkFault()
							continue
						}
					}
					if optFastNative {
						s.once.Do(func() { s.fast = m.fastInvoke(s.name) })
						if s.fast != nil {
							if r, ok := s.fast(natAny(regs[in.B]), regs, argRegs); ok {
								put(regs, in, r)
								cx.checkFault()
								continue
							}
						}
						if r, ok := m.genNativeInvoke(s, iv.V, regs, argRegs); ok {
							put(regs, in, r)
							cx.checkFault()
							continue
						}
					}
					put(regs, in, m.callNativeMethod(iv.V.R.(reflect.Value), s.name, s.sig, regs, argRegs))
					cx.checkFault()
					continue
				}
				if h := s.hit.Load(); h != nil && h.rt == iv.T {
					callee = h.fn
				} else {
					callee = iv.T.Methods[s.name]
					if h == nil {
						s.hit.Store(&invokeHit{rt: iv.T, fn: callee})
					}
				}
				if !m.isInterp(callee.ssa) {
					if m.progVR && isVRPointer(callee.ssa) {
						// Value.Pointer of the program's reflect through an
						// interface: the VM reflect, as for a static call,
						// so that the uintptr keeps its object (vrPointer).
						// The twin would give a bare address.
						put(regs, in, m.vrPointer(decV(iv.V)))
						continue
					}
					if s.memCheck {
						m.nativeMemInvoke(callee.ssa, iv.V, s.name, regs, argRegs)
					}
					put(regs, in, m.callBodyless(callee.ssa, iv.V, regs, argRegs))
					cx.checkFault()
					continue
				}
				recv = &iv.V
			}
			callee.ensure()
			cx.steps++
			if cx.steps >= cx.limit {
				cx.refill()
			}
			if len(cx.frames) >= cx.maxFrames {
				cx.overflow()
			}
			nb := base + fn.nregs
			if nb+callee.nregs > cx.hw {
				cx.hw = nb + callee.nregs
			}
			if nb+callee.nregs > len(cx.stack) {
				cx.grow(nb + callee.nregs)
				regs = cx.stack[base : base+fn.nregs]
			}
			nr := cx.stack[nb : nb+callee.nregs]
			copy(nr, callee.template)
			off := 0
			if recv != nil {
				nr[0] = *recv
				off = 1
			}
			for i, a := range argRegs {
				nr[off+i] = regs[a]
			}
			if env != nil {
				copy(nr[off+len(argRegs):], env)
			}
			cx.frames = append(cx.frames, frame{fn: fn, pc: pc, base: base, dst: in.A})
			fn, code, base, regs, pc = callee, callee.code, nb, nr, 0
			cx.fn, cx.base = fn, base

		case OpRet:
			if fn.locals != nil {
				cx.freeLocals(fn, regs)
			}
			var rv Value
			switch in.A {
			case -1:
			case -2:
				rs := in.X.([]int32)
				if optTuple && len(cx.frames) > 0 {
					// Optimization 2: multiple results are written directly
					// into the caller's consecutive result registers.
					f := cx.frames[len(cx.frames)-1]
					cx.frames = cx.frames[:len(cx.frames)-1]
					cr := cx.stack[f.base : f.base+f.fn.nregs]
					for i, r := range rs {
						cr[int(f.dst)+i] = regs[r]
					}
					fn, code, base, pc, regs = f.fn, f.fn.code, f.base, f.pc, cr
					cx.fn, cx.base = fn, base
					continue
				}
				t := make([]Value, len(rs))
				for i, r := range rs {
					t[i] = regs[r]
				}
				rv = Value{R: t}
			default:
				rv = regs[in.A]
			}
			if len(cx.frames) == 0 {
				return rv
			}
			f := cx.frames[len(cx.frames)-1]
			cx.frames = cx.frames[:len(cx.frames)-1]
			fn, code, base, pc = f.fn, f.fn.code, f.base, f.pc
			cx.fn, cx.base = fn, base
			regs = cx.stack[base : base+fn.nregs]
			regs[f.dst] = rv

		case OpCallNative:
			put(regs, in, m.callNativeSite(in.X.(*nativeSite), regs))
			cx.checkFault()

		case OpMakeClosure:
			s := in.X.(*closureSite)
			cl := newClosure(s.fn, len(s.binds))
			for i, r := range s.binds {
				cl.Env[i] = regs[r]
			}
			regs[in.A] = Value{R: cl}

		case OpAlloc:
			s := in.X.(*memSite)
			cell := new(Value)
			if s.flat > 0 {
				// A zero array of plain values: a fresh slice is already zero.
				if m.mem != nil && int64(s.flat)*valueSize >= memMin {
					m.memCheck(int64(s.flat) * valueSize)
					a := make([]Value, s.flat)
					m.memSlice(a)
					cell.R = a
				} else {
					cell.R = make([]Value, s.flat)
				}
			} else if s.deep {
				*cell = deepCopy(s.zero, s.t)
			} else {
				*cell = s.zero
			}
			regs[in.A] = Value{R: cell}
		case OpAllocLocal:
			cx.allocLocal(regs, in)
		case OpLoad:
			s := in.X.(*memSite)
			switch p := regs[in.B].R.(type) {
			case *Value:
				if s.copyLoad() {
					regs[in.A] = deepCopy(*p, s.t)
				} else if s.word {
					regs[in.A] = loadWord(p)
				} else {
					regs[in.A] = *p
				}
			case *byte:
				regs[in.A] = Value{N: int64(*p)}
			case roByte:
				regs[in.A] = Value{N: int64(*p)}
			case reflect.Value:
				regs[in.A] = m.loadNative(p, s)
			case *hdrView:
				regs[in.A] = p.load(s.t)
			case *hdrWord:
				regs[in.A] = p.load(s.t)
			case *byteView:
				regs[in.A] = p.load(s.t)
			case badAddr:
				p.refuse()
			default:
				panic(errNilDeref)
			}
		case OpStore:
			s := in.X.(*memSite)
			if p, ok := regs[in.A].R.(*Value); ok {
				if s.deep {
					assign(p, regs[in.B], s.t)
				} else if s.word {
					storeWord(p, regs[in.B])
				} else {
					*p = regs[in.B]
				}
				break
			}
			cx.store(regs[in.A], regs[in.B], s)
		case OpFieldAddr:
			switch p := regs[in.B].R.(type) {
			case *Value:
				switch s := p.R.(type) {
				case []Value:
					regs[in.A] = Value{R: &s[in.C]}
				case reflect.Value:
					regs[in.A] = Value{R: fieldAddr(s, int(in.C))}
				}
			case reflect.Value:
				if p.IsNil() {
					panic(errNilDeref)
				}
				regs[in.A] = Value{R: fieldAddr(p.Elem(), int(in.C))}
			case *hdrView:
				regs[in.A] = Value{R: &hdrWord{v: p, word: int(in.C)}}
			case *byteView:
				regs[in.A] = p.field(int(in.C))
			case badAddr:
				p.refuse()
			default:
				panic(errNilDeref)
			}
		case OpField:
			switch s := regs[in.B].R.(type) {
			case []Value:
				regs[in.A] = s[in.C]
			case reflect.Value:
				regs[in.A] = m.fromNative(s.Field(int(in.C)), in.X.(types.Type))
			}
		case OpIndexAddr:
			i := regs[in.C].N
			switch s := regs[in.B].R.(type) {
			case []Value:
				checkIndex(i, len(s), in.F)
				regs[in.A] = Value{R: &s[i]}
			case []byte:
				checkIndex(i, len(s), in.F)
				regs[in.A] = Value{R: &s[i]}
			default:
				regs[in.A] = m.indexAddr(regs[in.B], i, in.F)
			}
		case OpIndex:
			i := regs[in.C].N
			switch s := regs[in.B].R.(type) {
			case []Value:
				checkIndex(i, len(s), in.F)
				regs[in.A] = s[i]
			case string:
				checkIndex(i, len(s), in.F)
				regs[in.A] = Value{N: int64(s[i])}
			case *zslice:
				checkIndex(i, len(s.s), in.F)
				regs[in.A] = s.elem()
			default:
				panic(fmt.Sprintf("vm: index on %T", s))
			}
		case OpLookup:
			s := in.X.(*lookupSite)
			if s.str {
				str, _ := regs[in.B].R.(string)
				checkIndex(regs[in.C].N, len(str), s.idx)
				regs[in.A] = Value{N: int64(str[regs[in.C].N])}
				break
			}
			var v Value
			ok := false
			switch mp := regs[in.B].R.(type) {
			case *MapV:
				if m.panicOnFatal {
					v, ok = mp.guardedGet(s.ms.keyOf(regs[in.C]))
					break
				}
				v, ok = mp.M[s.ms.keyOf(regs[in.C])]
			case reflect.Value:
				if r := mp.MapIndex(m.toNative(regs[in.C], s.ms.key)); r.IsValid() {
					v, ok = m.fromNative(r, s.ms.elem), true
				}
			}
			if !ok {
				v = s.zero
			}
			if s.commaOk {
				putOk(regs, in, v, ok)
			} else {
				regs[in.A] = v
			}
		case OpMakeMap:
			regs[in.A] = Value{R: newMapV(0)}
		case OpMapUpdate:
			ms := in.X.(*mapSite)
			switch mp := regs[in.A].R.(type) {
			case *MapV:
				k := ms.keyOf(regs[in.B])
				if ms.nan {
					k = storeKey(k)
				}
				if m.panicOnFatal {
					mp.guardedSet(k, regs[in.C], regs[in.B])
					break
				}
				mp.M[k] = regs[in.C]
				mp.setKey(k, regs[in.B])
			case reflect.Value:
				m.hostWriteIn(mp)
				mp.SetMapIndex(m.toNative(regs[in.B], ms.key), m.toNative(regs[in.C], ms.elem))
			default:
				panic(errNilMapAssign)
			}
		case OpSlice:
			s := in.X.(*sliceSite)
			v := regs[in.B].R
			if p, ok := v.(*Value); ok {
				v = p.R
			}
			if s.kind == sliceOfString {
				str, _ := v.(string)
				lo, hi, _ := s.bounds(regs, len(str), len(str))
				if lo == hi {
					regs[in.A] = emptySubstring(regs[in.B], str)
					break
				}
				regs[in.A] = Value{R: str[lo:hi]}
				break
			}
			if b, ok := v.([]byte); ok {
				lo, hi, mx := s.bounds(regs, len(b), cap(b))
				regs[in.A] = Value{R: b[lo:hi:mx]}
				break
			}
			if bv, ok := v.(*byteView); ok {
				regs[in.A] = bv.slice(s, regs)
				break
			}
			if v == nil && s.kind == sliceOfArray {
				panic(errNilDeref)
			}
			if z, ok := v.(*zslice); ok {
				lo, hi, mx := s.bounds(regs, len(z.s), cap(z.s))
				regs[in.A] = Value{R: z.slice(lo, hi, mx, true)}
				break
			}
			if rv, ok := v.(reflect.Value); ok && rv.IsValid() {
				// Native memory: a native slice view (natslice.go), or an
				// array pointer made from a []byte. A byte slice stays
				// []byte and shares the native bytes; a slice of scalars
				// stays a view of the same array (fromNative). Other
				// element types are copied into a []Value: writes through
				// the new slice do not reach the native array.
				if rv.Kind() == reflect.Pointer {
					if rv.IsNil() {
						panic(errNilDeref)
					}
					rv = rv.Elem()
				}
				lo, hi, mx := s.bounds(regs, rv.Len(), rv.Cap()) // gc errors, not reflect panics
				rv = rv.Slice3(lo, hi, mx)
				if rv.Type().Elem().Kind() == reflect.Uint8 {
					regs[in.A] = Value{R: rv.Bytes()}
				} else {
					regs[in.A] = m.fromNative(rv, s.t)
				}
				break
			}
			sl, _ := v.([]Value)
			lo, hi, mx := s.bounds(regs, len(sl), cap(sl))
			regs[in.A] = Value{R: sl[lo:hi:mx]}
		case OpMakeSlice:
			s := in.X.(*memSite)
			makeSliceCheck(regs[in.B].N, regs[in.C].N, s.size)
			n, c := int(regs[in.B].N), int(regs[in.C].N)
			if c >= zsliceMin && s.zeroSized {
				regs[in.A] = Value{R: &zslice{s: make([]struct{}, n, c), zero: s.zero, t: s.t}}
				break
			}
			if s.bytes {
				if m.mem != nil && c >= memMin {
					regs[in.A] = Value{R: m.memBytes(c)[:n]}
					break
				}
				regs[in.A] = Value{R: make([]byte, n, c)}
				break
			}
			checkMakeSlice(int64(n), s.entries, s.t) // the elements, not a fatal OOM
			var memN int64
			if m.mem != nil {
				memN = m.memMakeSlice(int64(c), s.entries)
			}
			sl := make([]Value, n, c)
			// The whole capacity gets zero values: s[:cap(s)] shows them
			// (protobuf's filedesc allocates from make([]T, 0, n)).
			if full := sl[:c]; s.flat > 0 {
				for i := range full {
					full[i] = Value{R: make([]Value, s.flat)}
				}
			} else if s.deep || s.zero.R != nil {
				for i := range full {
					full[i] = deepCopy(s.zero, s.t)
				}
			}
			regs[in.A] = Value{R: sl}
			if memN > 0 {
				m.memAdd(unsafe.Pointer(unsafe.SliceData(sl[:c])), memN)
			}
		case OpLen:
			var n int
			switch s := regs[in.B].R.(type) {
			case []Value:
				n = len(s)
			case []byte:
				n = len(s)
			case string:
				n = len(s)
			case *MapV:
				n = len(s.M)
			case *zslice:
				n = len(s.s)
			case chan Value:
				n = len(s)
			case reflect.Value:
				n = s.Len()
			}
			regs[in.A] = Value{N: int64(n)}
		case OpCap:
			if b, ok := regs[in.B].R.([]byte); ok {
				regs[in.A] = Value{N: int64(cap(b))}
				break
			}
			var n int
			switch s := regs[in.B].R.(type) {
			case []Value:
				n = cap(s)
			case *zslice:
				n = cap(s.s)
			case chan Value:
				n = cap(s)
			case reflect.Value:
				n = s.Cap()
			}
			regs[in.A] = Value{N: int64(n)}
		case OpAppend:
			et, _ := in.X.(types.Type)
			old := regs[in.B].R
			regs[in.A] = m.appendAny(old, regs[in.C].R, et)
			if m.mem != nil {
				m.memAppend(old, regs[in.A].R, et)
			}
		case OpAppendV:
			old := regs[in.B].R
			site := in.X.(*appendSite)
			regs[in.A] = m.appendV(regs, old, site)
			if m.mem != nil {
				m.memAppend(old, regs[in.A].R, site.et)
			}
		case OpMakeAgg:
			regs[in.A] = makeAgg(regs, in.X.(*aggSite))
		case OpStoreAgg:
			s := in.X.(*aggStoreSite)
			if !s.storeInPlace(regs, regs[in.A]) {
				cx.store(regs[in.A], makeAgg(regs, s.agg), s.mem)
			}
		case OpCopy:
			et, _ := in.X.(types.Type)
			regs[in.A] = Value{N: int64(m.execCopy(regs[in.B].R, regs[in.C].R, et))}
		case OpMakeInterface:
			regs[in.A] = Value{R: &Iface{T: in.X.(*RType), V: regs[in.B]}}
		case OpMakeIfaceBox:
			regs[in.A] = Value{R: in.X.(*boxSite).box(regs[in.B])}
		case OpTypeAssert:
			s := in.X.(*assertSite)
			iv, _ := regs[in.B].R.(*Iface)
			ok := false
			var out Value
			if iv != nil {
				if s.concrete != nil {
					ok = iv.T == s.concrete
					out = iv.V
					if !ok && iv.T.Native != nil {
						s.once.Do(func() { s.nat = m.counterpart(s.t) })
						if ok = s.nat != nil && iv.T.Native == s.nat; ok {
							out = m.fromNative(iv.V.R.(reflect.Value), s.t)
						}
					}
				} else {
					if h := s.hit.Load(); h != nil && h.rt == iv.T {
						ok = h.ok
					} else {
						ok = m.implements(iv.T, s.iface)
						if h == nil {
							s.hit.Store(&assertHit{rt: iv.T, ok: ok})
						}
					}
					out = regs[in.B]
				}
			}
			if s.commaOk {
				if !ok {
					out = s.zero
				}
				putOk(regs, in, out, ok)
			} else if !ok {
				panic(m.assertError(s, iv))
			} else {
				regs[in.A] = out
			}
		case OpExtract:
			regs[in.A] = regs[in.B].R.([]Value)[in.C]
		case OpPrintln:
			s := in.X.(*printSite)
			writePrint(cx.m.printOut(), regs, s.args, s.types, s.ln)
		case OpPanic:
			panic(cx.m.newPanic(regs[in.B]))
		case OpFunc:
			put(regs, in, in.X.(func([]Value) Value)(regs))
			// AOT and intrinsic code can call back into the VM.
			cx.checkFault()
		case OpCtxFunc:
			cx.at = pc - 1
			put(regs, in, in.X.(func(*ctx, []Value) Value)(cx, regs))
		case OpGC:
			cx.execGC(fn, pc, base)
		case OpDefer:
			d := in.X.(*callDesc)
			if d.stack >= 0 {
				cx.deferTo(regs[d.stack], cx.prepare(d, regs))
				break
			}
			if len(cx.defers) == cap(cx.defers) {
				cx.checkDefers(len(d.args))
			}
			cx.defers = append(cx.defers, cx.prepare(d, regs))
		case OpDeferStack:
			regs[in.A] = Value{N: int64(len(cx.frames)), R: cx}
		case OpRunDefers:
			cx.at = pc - 1
			cx.runDefers(len(cx.frames), nil, false)
		case OpWrapNilChk:
			if isNilPtr(regs[in.B].R) {
				panic(in.X.(error))
			}
			regs[in.A] = regs[in.B]
		case OpRecover:
			if p := cx.panic; p != nil && !p.recovered && cx.directlyDeferred() && recoverable(p.val) {
				p.recovered = true
				regs[in.A] = m.panicValue(p.val)
			} else {
				regs[in.A] = Value{}
			}
		case OpGo:
			cx.spawn(cx.prepare(in.X.(*callDesc), regs))
		case OpMakeChan:
			if rt, ok := in.X.(reflect.Type); ok {
				regs[in.A] = Value{R: makeNativeChan(rt, regs[in.B].N)}
				break
			}
			regs[in.A] = Value{R: make(chan Value, regs[in.B].N)}
		case OpSend:
			cx.chanSend(regs[in.A].R, regs[in.B], in.X.(types.Type))
		case OpRecv:
			var v Value
			var ok bool
			if rv, isNat := regs[in.B].R.(reflect.Value); isNat { // native channel
				var x reflect.Value
				var fast bool
				if v, ok, fast = cx.fastRecv(rv); fast {
					// nativechan.go
				} else if x, ok = cx.natRecv(rv); ok {
					v = m.fromNative(x, in.X.(types.Type))
				} else {
					v = deepCopy(m.zero(in.X.(types.Type)), in.X.(types.Type))
				}
			} else {
				ch, _ := regs[in.B].R.(chan Value)
				if v, ok = cx.chanRecv(ch); !ok {
					// closed: the zero value of the element type, as in
					// execSelect (a struct is a []Value, not Value{})
					v = deepCopy(m.zero(in.X.(types.Type)), in.X.(types.Type))
				}
			}
			if in.C != 0 {
				putOk(regs, in, v, ok)
			} else {
				regs[in.A] = v
			}
		case OpClose:
			closeChan(regs[in.B].R)
		case OpSelect:
			put(regs, in, cx.execSelect(in.X.(*selectSite), regs))
		case OpRange:
			ms, _ := in.X.(*mapSite)
			regs[in.A] = m.execRange(regs[in.B].R, ms)
		case OpNext:
			put(regs, in, m.execNext(regs[in.B].R))
		case OpDelete:
			ms := in.X.(*mapSite)
			m.execDelete(regs[in.A], regs[in.B], ms.key)
		case OpFieldLoad:
			s := in.X.(*memSite)
			if p, ok := regs[in.B].R.(*Value); ok {
				if fs, ok := p.R.([]Value); ok {
					if s.copyLoad() {
						regs[in.A] = deepCopy(fs[in.C], s.t)
					} else if s.word {
						regs[in.A] = loadWord(&fs[in.C])
					} else {
						regs[in.A] = fs[in.C]
					}
					break
				}
				if rv, ok := p.R.(reflect.Value); ok && rv.Kind() == reflect.Struct {
					// a field of a native struct in native memory
					// (new(runtime.MemStats).Mallocs): read without making
					// its address, which a reflect.Value boxed in Value.R
					// would allocate
					if f := rv.Field(int(in.C)); f.CanInterface() {
						regs[in.A] = m.loadNativeValue(f, s)
						break
					}
				}
			} else if p, ok := regs[in.B].R.(reflect.Value); ok && p.Kind() == reflect.Pointer && !p.IsNil() {
				// a field through a native pointer (req.Header): the same,
				// without the boxed address of the field
				if f := p.Elem().Field(int(in.C)); f.CanInterface() {
					regs[in.A] = m.loadNativeValue(f, s)
					break
				}
			}
			regs[in.A] = m.load(m.fieldAddr(regs[in.B], in.C), s)
		case OpFieldStore:
			s := in.X.(*memSite)
			if p, ok := regs[in.A].R.(*Value); ok {
				if fs, ok := p.R.([]Value); ok {
					if s.deep {
						assign(&fs[in.C], regs[in.B], s.t)
					} else if s.word {
						storeWord(&fs[in.C], regs[in.B])
					} else {
						fs[in.C] = regs[in.B]
					}
					break
				}
			}
			m.store(m.fieldAddr(regs[in.A], in.C), regs[in.B], s)
		case OpIndexLoad:
			s := in.X.(*memSite)
			i := regs[in.C].N
			switch sl := regs[in.B].R.(type) {
			case []Value:
				checkIndex(i, len(sl), in.F)
				if s.copyLoad() {
					regs[in.A] = deepCopy(sl[i], s.t)
				} else if s.word {
					regs[in.A] = loadWord(&sl[i])
				} else {
					regs[in.A] = sl[i]
				}
			case []byte:
				checkIndex(i, len(sl), in.F)
				regs[in.A] = Value{N: int64(sl[i])}
			case reflect.Value:
				if !isNatView(sl) {
					regs[in.A] = m.load(m.indexAddr(regs[in.B], i, in.F), s)
					break
				}
				checkIndex(i, sl.Len(), in.F)
				regs[in.A] = natScalar(sl.Index(int(i)))
			default:
				regs[in.A] = m.load(m.indexAddr(regs[in.B], i, in.F), s)
			}
		case OpIndexStore:
			s := in.X.(*memSite)
			i := regs[in.C].N
			switch sl := regs[in.A].R.(type) {
			case []Value:
				checkIndex(i, len(sl), in.F)
				if s.deep {
					assign(&sl[i], regs[in.B], s.t)
				} else if s.word {
					storeWord(&sl[i], regs[in.B])
				} else {
					sl[i] = regs[in.B]
				}
			case []byte:
				checkIndex(i, len(sl), in.F)
				if m.restricted && uintptr(unsafe.Pointer(&sl[i]))>>hostPageShift != cx.freePage {
					cx.hostByte(unsafe.Pointer(&sl[i]))
				}
				sl[i] = byte(regs[in.B].N)
			case reflect.Value:
				if !isNatView(sl) {
					m.store(m.indexAddr(regs[in.A], i, in.F), regs[in.B], s)
					break
				}
				checkIndex(i, sl.Len(), in.F)
				m.hostWrite(sl.Index(int(i)))
				setNatScalar(sl.Index(int(i)), regs[in.B])
			default:
				m.store(m.indexAddr(regs[in.A], i, in.F), regs[in.B], s)
			}
		case OpJEqI:
			if regs[in.A].N == regs[in.B].N {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJNeI:
			if regs[in.A].N != regs[in.B].N {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJLtI:
			if regs[in.A].N < regs[in.B].N {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJLeI:
			if regs[in.A].N <= regs[in.B].N {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJGtI:
			if regs[in.A].N > regs[in.B].N {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJGeI:
			if regs[in.A].N >= regs[in.B].N {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJLtU:
			if uint64(regs[in.A].N) < uint64(regs[in.B].N) {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJLeU:
			if uint64(regs[in.A].N) <= uint64(regs[in.B].N) {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJGtU:
			if uint64(regs[in.A].N) > uint64(regs[in.B].N) {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJGeU:
			if uint64(regs[in.A].N) >= uint64(regs[in.B].N) {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		case OpJEqS, OpJNeS:
			x, _ := regs[in.A].R.(string)
			y, _ := regs[in.B].R.(string)
			if (x == y) == (in.Op == OpJEqS) {
				if int(in.C) < pc {
					cx.step()
				}
				pc = int(in.C)
			}
		default:
			panic(fmt.Sprintf("vm: bad opcode %d", in.Op))
		}
	}
}

// step counts a backward jump for the budget and the cancellation, as
// OpJmp does: a loop whose back edge is a conditional branch (a goto, a
// fused compare and branch) must stop like one with an unconditional
// jump.
func (cx *ctx) step() {
	cx.steps++
	if cx.steps >= cx.limit {
		cx.refill()
	}
}

// execCopy implements copy(dst, src) for []Value- or []byte-backed slices.
// et is the element type, or nil when it is not known.
func (m *Machine) execCopy(dst, src any, et types.Type) int {
	if b, ok := dst.([]byte); ok {
		m.hostBytes(b, min(len(b), seqLen(src)))
		return copyBytes(b, src)
	}
	if z, ok := dst.(*zslice); ok {
		return min(len(z.s), seqLen(src))
	}
	if n, ok := copyNat(dst, src); ok {
		return n
	}
	d, _ := dst.([]Value)
	var n int
	switch t := src.(type) {
	case *zslice:
		n = min(len(d), len(t.s)) // zero-size elements: nothing to copy
	case []byte:
		for n = 0; n < len(d) && n < len(t); n++ {
			d[n] = Value{N: int64(t[n])}
		}
	case []Value:
		if et != nil && needsCopy(et) {
			return copyValues(d, t, et)
		}
		if et != nil && wordKind(et) {
			return copyWords(d, t)
		}
		n = copy(d, t)
	case string:
		for n = 0; n < len(d) && n < len(t); n++ {
			d[n] = Value{N: int64(t[n])}
		}
	}
	return n
}

// copyValues copies the struct or array elements of src (of type et) into
// dst, like copy in Go. Each element of dst keeps its own memory (assign),
// so pointers into dst see the new values and dst never shares memory with
// src. dst and src can overlap.
func copyValues(dst, src []Value, et types.Type) int {
	n := min(len(dst), len(src))
	if n == 0 {
		return 0
	}
	dst, src = dst[:n], src[:n]
	if overlaps(dst, src) {
		tmp := make([]Value, n)
		for i := range tmp {
			tmp[i] = deepCopy(src[i], et)
		}
		src = tmp
	}
	for i := range dst {
		assign(&dst[i], src[i], et)
	}
	return n
}

// appendValues is append(s, t...) for struct or array elements of type et.
// When s has room, the new elements are written into its memory, as in Go.
// When append needs a new array, every element of the new array gets its
// own copy: the old and the new array must not share memory, and the spare
// capacity gets zero values, which s[:cap(s)] shows.
func appendValues(m *Machine, s, t []Value, et types.Type) []Value {
	old := len(s)
	if cap(s)-old >= len(t) {
		s = s[:old+len(t)]
		copyValues(s[old:], t, et)
		return s
	}
	s = append(s, t...)
	for i := range s {
		s[i] = deepCopy(s[i], et)
	}
	for i, full := len(s), s[:cap(s)]; i < len(full); i++ {
		full[i] = m.zeroValue(et)
	}
	return s
}

// growValues copies the elements of s (of type et) into the new, larger
// array ns, as when a slice grows: struct and array elements get their own
// copy, so the old and the new array do not share memory.
func growValues(ns, s []Value, et types.Type) {
	if !needsCopy(et) {
		copy(ns, s)
		return
	}
	for i := range s {
		ns[i] = deepCopy(s[i], et)
	}
}

// overlaps reports whether a and b share some elements.
func overlaps(a, b []Value) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	pa := uintptr(unsafe.Pointer(unsafe.SliceData(a)))
	pb := uintptr(unsafe.Pointer(unsafe.SliceData(b)))
	sz := unsafe.Sizeof(Value{})
	return pa < pb+uintptr(len(b))*sz && pb < pa+uintptr(len(a))*sz
}

func (m *Machine) execDelete(mv, key Value, kt types.Type) {
	switch mp := mv.R.(type) {
	case *MapV:
		k := keyOf(key, kt)
		if m.panicOnFatal {
			mp.guardedDelete(k)
			return
		}
		delete(mp.M, k)
		mp.deleteKey(k)
	case reflect.Value:
		m.hostWriteIn(mp)
		mp.SetMapIndex(m.toNative(key, kt), reflect.Value{})
	}
}

// execClear implements clear(x) for x of type t. The elements of a slice
// get the zero value of their type (a struct or an array is a []Value, not
// an empty Value).
func (m *Machine) execClear(x any, t types.Type) {
	switch x := x.(type) {
	case *MapV:
		if m.panicOnFatal {
			x.guardedClear()
			break
		}
		x.clear()
	case []Value:
		et := t.Underlying().(*types.Slice).Elem()
		z := m.zero(et)
		if z.R == nil {
			if wordKind(et) {
				for i := range x {
					storeWord(&x[i], Value{}) // a racy reader must not see half of it
				}
				break
			}
			clear(x)
			break
		}
		for i := range x {
			x[i] = deepCopy(z, et)
		}
	case []byte:
		m.hostBytes(x, len(x))
		clear(x)
	case reflect.Value:
		m.hostWriteIn(x)
		x.Clear()
	}
}

// put stores the result of an instruction: tuples (in.T > 1) are spread over
// consecutive registers.
func put(regs []Value, in *Instr, v Value) {
	if in.T > 1 {
		copy(regs[in.A:int(in.A)+int(in.T)], v.R.([]Value))
		return
	}
	regs[in.A] = v
}

func putOk(regs []Value, in *Instr, v Value, ok bool) {
	if in.T > 1 {
		regs[in.A], regs[in.A+1] = v, Value{N: b2i(ok)}
		return
	}
	regs[in.A] = Value{R: []Value{v, {N: b2i(ok)}}}
}

// fieldAddr returns the address of field i of the addressable native struct
// s. For an unexported field reflect returns a read-only value, whose
// methods cannot be called; the program may use them (methods promoted from
// an embedded unexported type, like net.conn's in net.UDPConn), so the
// pointer is rebuilt without the flag.
func fieldAddr(s reflect.Value, i int) reflect.Value {
	f := s.Field(i)
	if f.CanInterface() {
		return f.Addr()
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr()))
}

// equalStruct compares two structs in the order of gc
// (cmd/compile/internal/compare.EqStruct), which programs can observe: a
// comparison that panics (an interface that holds a slice) or that reads
// memory (the bytes of a string) must not happen when an earlier, cheaper
// one already gives false. The fields are cut in groups: a field whose
// comparison can panic is a group of its own. In a group, the bytes of the
// strings are compared last, after the other fields and the lengths of the
// strings.
func (m *Machine) equalStruct(xs, ys []Value, u *types.Struct) bool {
	group := func(lo, hi int) bool {
		for i := lo; i < hi; i++ {
			f := u.Field(i)
			switch {
			case f.Name() == "_": // blank fields are not compared
			case isString(f.Type()):
				if len(valStr(xs[i])) != len(valStr(ys[i])) {
					return false
				}
			case !m.equal(xs[i], ys[i], f.Type()):
				return false
			}
		}
		for i := lo; i < hi; i++ {
			if f := u.Field(i); f.Name() != "_" && isString(f.Type()) && valStr(xs[i]) != valStr(ys[i]) {
				return false
			}
		}
		return true
	}
	lo := 0
	for i := range xs {
		if f := u.Field(i); f.Name() != "_" && eqCanPanic(f.Type()) {
			if !group(lo, i) || !m.equal(xs[i], ys[i], f.Type()) {
				return false
			}
			lo = i + 1
		}
	}
	return group(lo, len(xs))
}

// eqCanPanic reports whether == on values of type t can panic: t holds an
// interface (cmd/compile/internal/compare.EqCanPanic).
func eqCanPanic(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Interface:
		return true
	case *types.Array:
		return eqCanPanic(u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if u.Field(i).Name() != "_" && eqCanPanic(u.Field(i).Type()) {
				return true
			}
		}
	}
	return false
}

// equalAcross compares two interface values whose dynamic types have two
// descriptors: a value that native code gave (the native type, T.Native) and
// a value of the program of the same named native type (its go/types type),
// as tok == json.Delim('[') for a tok of Decoder.Token. They are equal when
// the native counterpart of the second is the first, and their native
// values are equal, as the type assertion does.
func (m *Machine) equalAcross(x, y *Iface) bool {
	if x.T.Native == nil {
		x, y = y, x
	}
	if x.T.Native == nil || y.T.Native != nil || y.T.T == nil || m.counterpart(y.T.T) != x.T.Native {
		return false
	}
	if !x.T.Native.Comparable() {
		panic(runtimeError("comparing uncomparable type " + x.T.Native.String()))
	}
	return x.V.R.(reflect.Value).Interface() == m.toNative(y.V, y.T.T).Interface()
}

func (m *Machine) equal(a, b Value, t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsString != 0:
			x, _ := a.R.(string)
			y, _ := b.R.(string)
			return x == y
		case u.Info()&types.IsFloat != 0:
			return f64(a) == f64(b)
		case u.Info()&types.IsComplex != 0:
			return cplx(a) == cplx(b)
		case u.Kind() == types.UnsafePointer:
			return addrOf(a) == addrOf(b)
		}
		return a.N == b.N
	case *types.Interface:
		x, _ := a.R.(*Iface)
		y, _ := b.R.(*Iface)
		if x == nil || y == nil {
			return x == y
		}
		if x.T != y.T {
			return m.equalAcross(x, y)
		}
		if m.vr != nil && x.T == m.vr.typeRT {
			return x.V.R == y.V.R
		}
		if x.T.Native != nil {
			return x.V.R.(reflect.Value).Interface() == y.V.R.(reflect.Value).Interface()
		}
		if x.T.noCmp {
			panic(runtimeError("comparing uncomparable type " + typeString(x.T.T)))
		}
		return m.equal(x.V, y.V, x.T.T)
	case *types.Pointer:
		if ptrIsNil(a) || ptrIsNil(b) {
			return ptrIsNil(a) == ptrIsNil(b)
		}
		return ptrEqual(a, b, m.ptrInfo(u), u)
	case *types.Struct:
		if r, ok := m.nativeEqual(a, b, t); ok {
			return r
		}
		xs, _ := a.R.([]Value)
		ys, _ := b.R.([]Value)
		return m.equalStruct(xs, ys, u)
	case *types.Array:
		if r, ok := m.nativeEqual(a, b, t); ok {
			return r
		}
		xs, _ := a.R.([]Value)
		ys, _ := b.R.([]Value)
		if isString(u.Elem()) {
			// as gc: all the lengths first, then the contents
			for i := range xs {
				if len(valStr(xs[i])) != len(valStr(ys[i])) {
					return false
				}
			}
		}
		for i := range xs {
			if !m.equal(xs[i], ys[i], u.Elem()) {
				return false
			}
		}
		return true
	case *types.Slice:
		return isNilSlice(a.R) == isNilSlice(b.R) // only comparison with nil is legal
	}
	if ra, ok := a.R.(reflect.Value); ok {
		if rb, ok := b.R.(reflect.Value); ok {
			return ra.Pointer() == rb.Pointer()
		}
		return false
	}
	return a.R == b.R
}

// nativeEqual is == on a struct or an array of type t when a or b is a
// native value (a reflect.Value: a native named struct such as netip.Addr,
// time.Time or image.Point is kept as one, see zero). Such a value has no
// VM fields to walk, and its fields are often unexported, so Go itself
// compares it: reflect.Value.Equal is Go's == (it also reads unexported
// fields, and panics on an uncomparable value in an interface field, as Go
// does). A VM value on the other side is first converted to the native
// type. ok is false when neither a nor b is native.
func (m *Machine) nativeEqual(a, b Value, t types.Type) (r, ok bool) {
	ra, aok := a.R.(reflect.Value)
	rb, bok := b.R.(reflect.Value)
	if !aok && !bok {
		return false, false
	}
	if !aok {
		ra = m.toNative(a, t)
	}
	if !bok {
		rb = m.toNative(b, t)
	}
	return ra.Equal(rb), true
}

func (m *Machine) implements(rt *RType, it *types.Interface) bool {
	if r, ok := rt.impl.Load(it); ok {
		return r.(bool)
	}
	rt.implMu.Lock()
	defer rt.implMu.Unlock()
	if r, ok := rt.impl.Load(it); ok {
		return r.(bool)
	}
	var r bool
	if m.vr != nil && rt == m.vr.typeRT {
		r = true // reflect.Type / reflectlite.Type
	} else if rt.T != nil {
		r = m.typesImplements(rt.T, it)
	} else {
		r = true
		for i := 0; i < it.NumMethods(); i++ {
			fn := it.Method(i)
			nm, ok := rt.Native.MethodByName(fn.Name())
			if !ok || !m.nativeSigMatches(nm.Type, rt.Native.Kind() == reflect.Interface, fn.Type().(*types.Signature)) {
				r = false
			}
		}
	}
	rt.impl.Store(it, r)
	return r
}

// nativeSigMatches reports whether the native method type mt (of
// reflect.Type.MethodByName: the receiver is its first parameter, except
// for an interface type) has the signature sig of an interface method. A method of the
// same name and another signature does not implement it:
// err.(interface{ Unwrap() []error }) on a *fmt.wrapError, whose Unwrap
// returns error, is false (TestErrorf of package fmt). A type that only the
// program has (no native counterpart: a named type or a non-empty interface
// of the program) is never identical to a type of a native method.
func (m *Machine) nativeSigMatches(mt reflect.Type, isIface bool, sig *types.Signature) bool {
	recv := 1
	if isIface {
		recv = 0
	}
	if mt.NumIn()-recv != sig.Params().Len() || mt.NumOut() != sig.Results().Len() || mt.IsVariadic() != sig.Variadic() {
		return false
	}
	for i := recv; i < mt.NumIn(); i++ {
		if !m.nativeIdentical(mt.In(i), sig.Params().At(i-recv).Type()) {
			return false
		}
	}
	for i := 0; i < mt.NumOut(); i++ {
		if !m.nativeIdentical(mt.Out(i), sig.Results().At(i).Type()) {
			return false
		}
	}
	return true
}

// nativeIdentical reports whether the native type rt and the type t of the
// program are identical. counterpart does not build every unnamed type (not
// struct{}, so not <-chan struct{}, the result of Done of a native
// context.Context): an unnamed type is then compared by its structure.
func (m *Machine) nativeIdentical(rt reflect.Type, t types.Type) bool {
	if ct := m.counterpart(t); ct != nil {
		return ct == rt
	}
	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return rt.Kind() == reflect.Pointer && m.nativeIdentical(rt.Elem(), t.Elem())
	case *types.Slice:
		return rt.Kind() == reflect.Slice && m.nativeIdentical(rt.Elem(), t.Elem())
	case *types.Array:
		return rt.Kind() == reflect.Array && int64(rt.Len()) == t.Len() && m.nativeIdentical(rt.Elem(), t.Elem())
	case *types.Map:
		return rt.Kind() == reflect.Map && m.nativeIdentical(rt.Key(), t.Key()) && m.nativeIdentical(rt.Elem(), t.Elem())
	case *types.Chan:
		dir := map[types.ChanDir]reflect.ChanDir{types.SendRecv: reflect.BothDir, types.SendOnly: reflect.SendDir, types.RecvOnly: reflect.RecvDir}[t.Dir()]
		return rt.Kind() == reflect.Chan && rt.ChanDir() == dir && m.nativeIdentical(rt.Elem(), t.Elem())
	case *types.Signature:
		return rt.Kind() == reflect.Func && m.nativeSigMatches(rt, true, t)
	case *types.Struct:
		if rt.Kind() != reflect.Struct || rt.NumField() != t.NumFields() {
			return false
		}
		for i := 0; i < t.NumFields(); i++ {
			f, rf := t.Field(i), rt.Field(i)
			// An unexported field of the program is never in the package of
			// a native type.
			if !f.Exported() || f.Name() != rf.Name || f.Embedded() != rf.Anonymous ||
				t.Tag(i) != string(rf.Tag) || !m.nativeIdentical(rf.Type, f.Type()) {
				return false
			}
		}
		return true
	}
	return false
}

// store runs *p = v (OpStore) for the address p of any kind.
func (cx *ctx) store(p, v Value, s *memSite) {
	m := cx.m
	switch p := p.R.(type) {
	case *Value:
		if s.deep {
			assign(p, v, s.t)
		} else if s.word {
			storeWord(p, v)
		} else {
			*p = v
		}
	case *byte:
		if m.restricted && uintptr(unsafe.Pointer(p))>>hostPageShift != cx.freePage {
			cx.hostByte(unsafe.Pointer(p))
		}
		*p = byte(v.N)
	case roByte:
		refuseROWrite()
	case reflect.Value:
		m.hostWritePtr(p)
		setNative(p.Elem(), m.toNative(v, s.t))
	case *hdrView:
		p.store(v, s.t)
	case *hdrWord:
		p.store(v, s.t)
	case *byteView:
		p.store(v, s.t)
	case badAddr:
		p.refuse()
	default:
		panic(errNilDeref)
	}
}
