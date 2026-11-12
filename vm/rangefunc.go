package vm

// The run-time errors of range-over-func misuse.
//
// go/ssa checks the state of a range-over-func loop (the jump variable) and
// panics with strings of its own: "yield function called after range loop
// exit" and "iterator call did not preserve panic". gc panics with a
// runtime.Error whose message depends on the state, and an iterator that
// recovers such a panic and returns normally makes the loop panic again.
// The VM replaces the two panics of go/ssa with the errors of gc.

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// The states of the jump variable of go/ssa (builder.go): READY, BUSY (the
// body runs), DONE (the iterator returned); a positive value is the exit
// that the body took (break, return, goto).
const (
	jumpReady = 0
	jumpBusy  = -1
	jumpDone  = -2
)

// The errors of gc, taken from real panics of the host.
var (
	errRangeDone = hostRuntimePanic(func() {
		for range func(yield func() bool) { yield(); yield() } {
			break
		}
	})
	errRangeExhausted = hostRuntimePanic(func() {
		var keep func() bool
		for range func(yield func() bool) { keep = yield } {
		}
		keep()
	})
	errRangePanic = hostRuntimePanic(func() {
		for range func(yield func() bool) {
			defer func() { recover(); yield() }()
			yield()
		} {
			panic(0)
		}
	})
	errRangeMissingPanic = hostRuntimePanic(func() {
		for range func(yield func() bool) {
			defer func() { recover() }()
			yield()
		} {
			panic(0)
		}
	})
)

// rangeFuncPanic compiles the panics of go/ssa for range-over-func misuse
// (see above) and reports whether v is one of them.
func (c *compiler) rangeFuncPanic(v *ssa.Panic) bool {
	switch v.Block().Comment {
	case "yield-invalid":
		// if jump != READY { panic(...) } in the yield function
		jumpPtr, ok := rangeJumpPtr(v, "yield function called after range loop exit")
		if !ok {
			return false
		}
		p := c.reg(jumpPtr)
		c.emit(Instr{Op: OpFunc, A: c.newReg(), X: func(regs []Value) Value {
			cell, _ := regs[p].R.(*Value)
			if cell == nil {
				panic(errNilDeref)
			}
			state := cell.N
			// as gc: the loop now waits for a panic, and an iterator
			// that recovers it and returns makes the loop panic
			cell.N = jumpBusy
			switch state {
			case jumpBusy:
				panic(errRangePanic)
			case jumpDone:
				panic(errRangeExhausted)
			}
			panic(errRangeDone)
		}})
		return true
	case "rangefunc.resume.busy":
		// the iterator returned while jump is BUSY
		if _, ok := rangeJumpPtr(v, "iterator call did not preserve panic"); !ok {
			return false
		}
		c.emit(Instr{Op: OpFunc, A: c.newReg(), X: func([]Value) Value { panic(errRangeMissingPanic) }})
		return true
	}
	return false
}

// rangeResumeLoad compiles v := *jump after the call of the iterator
// (buildYieldResume of go/ssa), and reports whether v is that load. go/ssa
// sets jump to DONE only when the body ran to its end; gc marks the loop
// exhausted when the iterator returns, also after a break, return or goto
// of the body. The VM does it here, so a yield kept and called after the
// loop panics with "after whole loop exit".
func (c *compiler) rangeResumeLoad(v *ssa.UnOp) bool {
	if v.Op != token.MUL || v.Block().Comment == "yield-invalid" {
		return false
	}
	refs := v.Referrers()
	if refs == nil {
		return false
	}
	resume := false
	for _, r := range *refs {
		cmp, ok := r.(*ssa.BinOp)
		if !ok || cmp.X != v || cmp.Op != token.EQL {
			continue
		}
		for _, rr := range *cmp.Referrers() {
			if ifi, ok := rr.(*ssa.If); ok && ifi.Block().Succs[0].Comment == "rangefunc.resume.busy" {
				resume = true
			}
		}
	}
	if !resume {
		return false
	}
	dst, p := c.reg(v), c.reg(v.X)
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		cell, _ := regs[p].R.(*Value)
		if cell == nil {
			panic(errNilDeref)
		}
		state := cell.N
		if state != jumpBusy {
			cell.N = jumpDone
		}
		return Value{N: state}
	}})
	return true
}

// rangeJumpPtr checks that v panics with the string msg after a test of the
// jump variable, and returns the pointer to that variable.
func rangeJumpPtr(v *ssa.Panic, msg string) (ssa.Value, bool) {
	mi, ok := v.X.(*ssa.MakeInterface)
	if !ok {
		return nil, false
	}
	k, ok := mi.X.(*ssa.Const)
	if !ok || k.Value == nil || k.Value.Kind() != constant.String || constant.StringVal(k.Value) != msg {
		return nil, false
	}
	if len(v.Block().Preds) != 1 {
		return nil, false
	}
	pred := v.Block().Preds[0]
	ifi, ok := pred.Instrs[len(pred.Instrs)-1].(*ssa.If)
	if !ok {
		return nil, false
	}
	cmp, ok := ifi.Cond.(*ssa.BinOp)
	if !ok {
		return nil, false
	}
	load, ok := cmp.X.(*ssa.UnOp)
	if !ok {
		return nil, false
	}
	return load.X, true
}
