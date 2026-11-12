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

// chargeMem counts the cells and deferred calls of cx in the memory quota
// (Options.MaxMemory), as grow counts a large register stack: MaxStack
// bounds one goroutine, the quota bounds them all. The quota counts live
// objects by weak pointers: the charge is held by small tokens that cx
// keeps, in steps of a quarter of the charge (at least 1 MiB), so a
// recursion pays a few reservations, not one per frame; the quota can stop
// a stack that grows a step before it is full. The tokens go when the ctx ends (a
// goroutine, or a ctx back in the pool): until then, the charge stays at
// the largest stack of the ctx, as a Go stack does not shrink at once.
// Past the quota, the plugin panics with ErrMemory.
func (cx *ctx) chargeMem() {
	need := (cx.cells + int64(cx.deferVals)) * valueSize
	if need <= cx.charged || need < memMin {
		// Below memMin, as the other counts: else the first small local
		// array of each call from the host reserved 1 MiB, held until the
		// next garbage collection.
		return
	}
	tok := new([16]byte)
	step := max(need-cx.charged, cx.charged/4, 1<<20)
	if cx.m.mem.reserve(step, unsafe.Pointer(tok)) {
		cx.chargeTok = append(cx.chargeTok, tok)
		cx.charged += step
		return
	}
	// The count of the cells can be too large after a panic: count them
	// again. There is no smaller retry: near the quota, each one would
	// force a garbage collection and wait for it, frame after frame.
	if cx.stackUse(cx.topRegs()); (cx.cells+int64(cx.deferVals))*valueSize <= cx.charged {
		return
	}
	panic(ErrMemory)
}

// belowVals is the regsBelow of a ctx that starts on top of cx (nestIn,
// iter.Pull): the registers, cells and deferred calls of cx and below.
// The count of cx is made exact when it seems to pass MaxStack, so that a
// count left too large by a panic does not stop the new ctx.
func (cx *ctx) belowVals() int {
	top := cx.topRegs()
	n := cx.stackVals(top)
	if max := cx.m.MaxStack; max > 0 && n*valueSize >= max {
		n = cx.stackUse(top)
	}
	return int(n)
}
