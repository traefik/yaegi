package vm

import "unsafe"

// The stack size of a goroutine (Options.MaxStack, Machine.MaxStack).
//
// gc stops a goroutine whose stack passes 1 GB with a fatal error. The VM
// has no Go stack per interpreted frame: a frame is its registers on the
// register stack of the ctx, the cells of its frame-local Allocs (a local
// array that does not escape, escape.go), and its deferred calls. Without a
// bound in bytes, a recursion with big frames (many locals, a local
// [2000]int) or a defer in a long loop grows these until the host runs out
// of memory, long before MaxDepth frames. MaxStack bounds their sum, for the
// ctx and the ctxs below it on the goroutine (regsBelow); past it, the
// goroutine stops with ErrStackOverflow, as a Go program past its stack
// limit (no recover can stop it).
//
// The count is cheap on the hot paths:
//   - the registers: the check runs only when the register stack grows
//     (grow);
//   - the cells: OpAllocLocal adds the Values of a new cell to cx.cells, a
//     normal return gives them back (freeLocals). A frame that a panic
//     unwinds gives nothing back, so cx.cells can only be too large: when
//     it seems to pass the bound, stackUse counts the cells of the live
//     frames again;
//   - the deferred calls: they are counted when the slice of cx.defers is
//     full (once for each doubling), in cx.deferVals.

// DefaultMaxStack is the MaxStack of a plugin whose Options.MaxStack is 0:
// 1 GiB for each goroutine, as gc. A register, and each element of a
// frame-local array (even a byte), is a 24-byte Value: the same frames take
// 3 to 24 times more room than under gc, so a lower bound stopped valid
// programs (3000 frames with a local [4096]byte, 12 MB under gc, passed
// 256 MiB). MaxMemory bounds all the goroutines together.
const DefaultMaxStack = 1 << 30

// deferVals is the cost in Values of one deferred call without its
// arguments: the deferred record and the closure that prepare may make.
const deferVals = int((unsafe.Sizeof(deferred{}) + 2*uintptr(valueSize) - 1) / uintptr(valueSize))

// stackVals is the fast count of the Values of the goroutine below and in
// cx, with top registers in use on cx. It can be too large (cx.cells after
// a panic, cx.deferVals after the deferred calls ran), never too small.
func (cx *ctx) stackVals(top int) int64 {
	return int64(cx.regsBelow) + int64(top) + cx.cells + int64(cx.deferVals)
}

// stackUse is the exact count: it counts the cells of the live frames and
// the deferred calls again, and keeps the new counts.
func (cx *ctx) stackUse(top int) int64 {
	var cells int64
	count := func(fn *Func, base int) {
		if fn == nil || fn.locals == nil || base+fn.nregs > len(cx.stack) {
			return
		}
		regs := cx.stack[base : base+fn.nregs]
		for _, l := range fn.locals {
			if _, ok := regs[l.reg].R.(*Value); ok {
				cells += l.site.cellVals
			}
		}
	}
	for _, f := range cx.frames {
		count(f.fn, f.base)
	}
	count(cx.fn, cx.base)
	cx.cells = cells
	cx.deferVals = defersVals(cx.defers)
	return cx.stackVals(top)
}

// defersVals is the cost in Values of the deferred calls ds. The records
// count twice: an append to a full slice makes a new array of about twice
// its length, and copies the old one into it. The environment of a
// deferred closure counts twice: a captured variable is a Value and the
// cell it points to (a variable of a loop is a new cell at each turn).
func defersVals(ds []deferred) int {
	n := 2 * len(ds) * deferVals
	for i := range ds {
		n += len(ds[i].args) + 2*len(ds[i].env)
	}
	return n
}

// checkDefers counts the deferred calls of cx and one more with nargs
// arguments, when the slice of cx.defers is full: against MaxStack, and in
// the memory quota.
func (cx *ctx) checkDefers(nargs int) {
	m := cx.m
	if m.MaxStack <= 0 && m.mem == nil {
		return
	}
	cx.deferVals = defersVals(cx.defers) + deferVals + nargs
	if m.MaxStack > 0 {
		cx.checkStack(cx.topRegs())
	}
	if m.mem != nil {
		cx.chargeMem()
	}
}

// topRegs is the register count in use on cx: up to the end of the frame
// that runs.
func (cx *ctx) topRegs() int {
	if cx.fn == nil {
		return cx.base
	}
	return cx.base + cx.fn.nregs
}

// checkStack stops the goroutine with ErrStackOverflow when its stack, with
// top registers in use on cx, passes MaxStack.
func (cx *ctx) checkStack(top int) {
	max := cx.m.MaxStack
	if max <= 0 || cx.stackVals(top)*valueSize <= max {
		return
	}
	if cx.stackUse(top)*valueSize > max {
		cx.overflow()
	}
}

// fitStack shortens the register stack of cx, which runs no frame yet, to
// what MaxStack allows: the check of the registers runs only when the stack
// grows (grow), and a ctx of the pool can come with a longer stack than
// the bound.
func (cx *ctx) fitStack() {
	if limit := cx.m.MaxStack; limit > 0 {
		if n := limit/valueSize - int64(cx.regsBelow); n < int64(len(cx.stack)) {
			cx.stack = cx.stack[:max(n, 0)]
		}
	}
}

// chargeMem counts the cells, deferred calls and free cells of cx in the
// memory quota (Options.MaxMemory), as grow counts a large register
// stack: MaxStack bounds one goroutine, the quota bounds them all. The
// charge is held by cx.memCharge, which cx gives back when it ends
// (releaseMem: back in the pool, or the end of its goroutine). Until
// then, it stays at the largest stack of the ctx, as a Go stack does not
// shrink at once. It grows by the missing bytes and an eighth more (at
// most 1 MiB), so a recursion pays a few reservations, not one per frame.
// Past the quota, the plugin panics with ErrMemory.
//
// Before, the charge was held by weak tokens, in steps of at least 1 MiB,
// that only a garbage collection gave back: each call from the host with a
// large frame-local array (a pooled ctx, its cell taken from the free
// list) reserved 1 MiB more, and about 1000 calls filled a quota of 1 GiB
// with dead tokens.
func (cx *ctx) chargeMem() {
	need := cx.keptVals() * valueSize
	if need <= cx.charged || need < memMin {
		// Below memMin, as the other counts: the first small local array
		// of each call from the host takes no lock of the quota.
		return
	}
	if cx.reserveMem(need, true) {
		return
	}
	// The count of the cells can be too large after a panic: count them
	// again. There is no smaller retry: near the quota, each one would
	// force a garbage collection and wait for it, frame after frame.
	if cx.stackUse(cx.topRegs()); cx.keptVals()*valueSize <= cx.charged {
		return
	}
	panic(ErrMemory)
}

// reserveMem grows the stack charge of cx to need bytes and an eighth more
// (at most 1 MiB), or reports false when they do not fit in the quota.
// With gc false, it does not force a garbage collection to make room.
func (cx *ctx) reserveMem(need int64, gc bool) bool {
	q := cx.m.mem
	sc := cx.memCharge
	if sc == nil || sc.q != q {
		if sc != nil {
			sc.release(true) // a pooled ctx of another Machine
		}
		if cx.memTok == nil {
			cx.memTok = new([16]byte)
		}
		sc = q.newStackCharge(cx.memTok)
		cx.memCharge, cx.charged = sc, 0
	}
	step := need - cx.charged + min(need/8, 1<<20)
	var ok bool
	if gc {
		ok = q.reserveFor(step, nil, sc)
	} else {
		ok = q.tryReserveFor(step, sc)
	}
	if ok {
		cx.charged += step
	}
	return ok
}

// chargeFree counts in the quota the free cells that a pooled ctx brings
// to a new call from the host (getCtx). They stay with the ctx in the
// pool, but putCtx gave back their charge: without this, they were
// counted again only at the next OpAllocLocal, and a call that runs none
// held them outside MaxMemory for all its life (1000 requests in flight
// kept 96 MB with no charge, round 3 review of PR 323). The free cells
// are only a cache: when they do not fit in the quota, they are dropped
// instead, with no garbage collection and no ErrMemory.
func (cx *ctx) chargeFree() {
	if cx.m.mem == nil || int64(cx.freeSize)*valueSize < memMin {
		return
	}
	if !cx.reserveMem(int64(cx.freeSize)*valueSize, false) {
		cx.dropFree()
	}
}

// keptVals is the count in Values of what chargeMem counts: the cells of
// the live frames, the deferred calls, and the free cells (escape.go).
func (cx *ctx) keptVals() int64 {
	return cx.cells + int64(cx.deferVals) + int64(cx.freeSize)
}

// releaseMem gives back the charge of cx in the memory quota (chargeMem),
// when cx ends.
func (cx *ctx) releaseMem() {
	if cx.charged == 0 {
		return
	}
	cx.memCharge.release(false)
	cx.charged = 0
}

// belowVals is the regsBelow of a ctx that starts on top of cx (nestIn,
// iter.Pull): the registers, cells and deferred calls of cx and below.
// The count of cx is made exact when it seems to pass MaxStack, so that a
// count left too large by a panic does not stop the new ctx.
func (cx *ctx) belowVals() int {
	top := cx.topRegs()
	n := cx.stackVals(top)
	if max := cx.m.MaxStack; max > 0 && n*valueSize >= max {
		if gs := cx.gs; gs != nil && gs.fault != nil {
			// The deferred calls of a fault (nest.go, fail) pay the
			// recount from their grace: it walks every pending
			// deferred call, and a loop of a million defers stopped
			// by MaxStack made each of its deferred calls walk all
			// the others.
			gs.faultLeft -= int64(len(cx.defers) >> 10)
			if gs.faultLeft <= 0 {
				gs.hard = true
				panic(gs.fault)
			}
		}
		n = cx.stackUse(top)
	}
	return int(n)
}
