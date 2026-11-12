package vm

// Dead registers and runtime.GC.
//
// A frame keeps every register until it returns, and the register stack keeps
// the registers of returned frames until another call writes over them. In a
// Go program, a value is not reachable after its last use. Programs that wait
// for a finalizer after runtime.GC() (GOROOT tests fixedbugs/issue24491b.go,
// typeparam/chans.go) wait forever if the VM still holds the object.
//
// Clearing registers on each return or after each last use would slow down
// every call. Instead, an interpreted call of runtime.GC (OpGC) clears, in the
// frames of the calling goroutine, the registers whose SSA values are dead at
// the current point of each frame, and the registers above the top frame.
// The liveness of a function is computed from its SSA the first time a GC
// happens while one of its frames is on the stack. Only registers of SSA
// values are cleared: constants, globals, functions and compiler temporaries
// are kept.

import (
	"go/ast"
	"go/types"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// gcInfo is the liveness data of a function, built on the first GC.
type gcInfo struct {
	ins  []ssa.Instruction // SSA instructions in block order (insStart order)
	blk  []int             // index of the block of ins[i]
	pos  []int             // index of ins[i] in its block
	reg  map[ssa.Value][2]int32
	out  []map[ssa.Value]bool       // live values at the end of each block
	keep map[ssa.Value]bool         // live in the Recover block: live everywhere
	agg  map[*ssa.Alloc][]ssa.Value // Func.aggVals: the fields of the literals built in a register
}

// execGC clears the dead registers of the frames of cx and runs runtime.GC.
// fn, pc and base describe the current frame. When cx runs a deferred call,
// the frames of the ctx that runs the defers are cleared too (parent): its
// current frame waits at parent.at (OpRunDefers), or is panicking. The walk
// stops at a callback of native code: the instruction of the frame that
// called the native code may not be known (ctx.at is exact only when the
// machine tracks it, callers.go).
func (cx *ctx) execGC(fn *Func, pc, base int) {
	cx.clearFrames(fn, pc, base)
	for c := cx; c.parent != nil && !c.native; c = c.parent {
		p := c.parent
		if p.fn != nil {
			ppc := p.at + 1
			if c.unwinding {
				ppc = -1
			}
			p.clearFrames(p.fn, ppc, p.base)
		}
	}
	runtime.GC()
}

// clearFrames clears the dead registers of the frames of cx, of its top
// frame (fn at pc, registers from base), and the stack above the top frame.
func (cx *ctx) clearFrames(fn *Func, pc, base int) {
	for _, f := range cx.frames {
		f.fn.clearDead(cx.stack[f.base:f.base+f.fn.nregs], f.pc)
	}
	fn.clearDead(cx.stack[base:base+fn.nregs], pc)
	if top := base + fn.nregs; top < cx.hw && top < len(cx.stack) {
		clear(cx.stack[top:min(cx.hw, len(cx.stack))])
	}
}

// clearDead clears the registers of the SSA values that are dead after the
// instruction that ends just before pc (the call that the frame waits for).
// pc -1 means that the frame is panicking: it can only go on in its Recover
// block, so only the values of that block stay live (GOROOT test
// fixedbugs/issue32477.go).
func (f *Func) clearDead(regs []Value, pc int) {
	if f.ssa == nil || len(f.insStart) == 0 || pc == 0 {
		return
	}
	g := f.liveness()
	var live map[ssa.Value]bool
	if pc < 0 {
		live = g.keep
	} else {
		// The instruction whose code holds pc-1: the last one starting at
		// or before it.
		i := sort.Search(len(f.insStart), func(i int) bool { return int(f.insStart[i]) > pc-1 }) - 1
		if i < 0 || i >= len(g.ins) {
			return
		}
		// A frame that waits for an interpreted call gave the arguments
		// to the callee, which clears them once they are dead there.
		var waits bool
		switch f.code[pc-1].Op {
		case OpCall, OpCallClosure, OpInvoke:
			waits = true
		}
		live = g.liveAfter(f.ssa, i, waits)
	}
	keep := make(map[int32]bool)
	for v := range live {
		if r, ok := g.reg[v]; ok {
			for k := r[0]; k < r[0]+r[1]; k++ {
				keep[k] = true
			}
		}
	}
	for v, r := range g.reg {
		if live[v] {
			continue
		}
		for k := r[0]; k < r[0]+r[1]; k++ {
			if !keep[k] && int(k) < len(regs) {
				regs[k] = Value{}
				// The cell of a frame-local Alloc (escape.go) is
				// dead with its Alloc.
				if cell, ok := f.cells[k]; ok && int(cell) < len(regs) {
					regs[cell] = Value{}
				}
			}
		}
	}
}

// liveness returns the liveness data of f, computed once.
func (f *Func) liveness() *gcInfo {
	f.gcOnce.Do(func() {
		g := &gcInfo{reg: map[ssa.Value][2]int32{}, agg: f.aggVals}
		fn := f.ssa
		for bi, b := range fn.Blocks {
			for j, ins := range b.Instrs {
				g.ins = append(g.ins, ins)
				g.blk = append(g.blk, bi)
				g.pos = append(g.pos, j)
			}
		}
		for i, r := range f.insReg {
			if r[1] > 0 && i < len(g.ins) {
				g.reg[g.ins[i].(ssa.Value)] = r
			}
		}
		for i, p := range fn.Params {
			g.reg[p] = [2]int32{int32(i), 1}
		}
		for i, fv := range fn.FreeVars {
			g.reg[fv] = [2]int32{int32(len(fn.Params) + i), 1}
		}
		g.solve(fn)
		f.gc = g
	})
	return f.gc
}

// tracked reports whether v is a value whose register may be cleared.
func (g *gcInfo) tracked(v ssa.Value) bool {
	_, ok := g.reg[v]
	return ok
}

// uses adds to set the tracked values that ins reads. A value without a
// register (an address fused into its load or store) stands for its own
// operands; the Alloc of a composite literal built in a register (aggv.go)
// stands for the values of its fields, which its Load reads.
func (g *gcInfo) uses(ins ssa.Instruction, set map[ssa.Value]bool, seen map[ssa.Value]bool) {
	for _, op := range ins.Operands(nil) {
		if op == nil || *op == nil {
			continue
		}
		v := *op
		if g.tracked(v) {
			set[v] = true
			continue
		}
		if a, ok := v.(*ssa.Alloc); ok {
			for _, x := range g.agg[a] {
				if g.tracked(x) {
					set[x] = true
				}
			}
		}
		if in, ok := v.(ssa.Instruction); ok && !seen[v] {
			seen[v] = true
			g.uses(in, set, seen)
		}
	}
}

// solve computes the live values at the end of each block.
func (g *gcInfo) solve(fn *ssa.Function) {
	n := len(fn.Blocks)
	in := make([]map[ssa.Value]bool, n)
	g.out = make([]map[ssa.Value]bool, n)
	for i := range in {
		in[i], g.out[i] = map[ssa.Value]bool{}, map[ssa.Value]bool{}
	}
	for changed := true; changed; {
		changed = false
		for bi := n - 1; bi >= 0; bi-- {
			b := fn.Blocks[bi]
			out := g.out[bi]
			for _, s := range b.Succs {
				for v := range in[s.Index] {
					if !out[v] {
						out[v], changed = true, true
					}
				}
				// Phi operands are read on the edge, at the end of b.
				for _, ins := range s.Instrs {
					phi, ok := ins.(*ssa.Phi)
					if !ok {
						break
					}
					for pi, pred := range s.Preds {
						if pred == b && g.tracked(phi.Edges[pi]) && !out[phi.Edges[pi]] {
							out[phi.Edges[pi]], changed = true, true
						}
					}
				}
			}
			live := g.backward(b, len(b.Instrs)-1, -1, out)
			for v := range live {
				if !in[bi][v] {
					in[bi][v], changed = true, true
				}
			}
		}
	}
	g.keep = map[ssa.Value]bool{}
	if fn.Recover != nil {
		for v := range in[fn.Recover.Index] {
			g.keep[v] = true
		}
	}
	// The objects of uintptr(p) in the arguments of a deferred (or go)
	// call of a //go:uintptrescapes function stay alive until that call
	// runs: keep them live in the whole function (GOROOT test
	// fixedbugs/issue24491a.go).
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			var cc *ssa.CallCommon
			switch ins := ins.(type) {
			case *ssa.Defer:
				cc = ins.Common()
			case *ssa.Go:
				cc = ins.Common()
			default:
				continue
			}
			if !uintptrEscapes(declOf(cc.StaticCallee())) {
				continue
			}
			for _, a := range cc.Args {
				for _, x := range uintptrArgs(a) {
					if g.tracked(x) {
						g.keep[x] = true
					}
				}
			}
		}
	}
}

// declOf returns the declared function behind f: for a method wrapper (a
// call of a method on a pointer, for example), the method itself.
func declOf(f *ssa.Function) *ssa.Function {
	if f == nil || f.Synthetic == "" {
		return f
	}
	if obj, ok := f.Object().(*types.Func); ok {
		if d := f.Prog.FuncValue(obj); d != nil {
			return d
		}
	}
	return f
}

// backward walks b from instruction hi down to lo+1 starting from the live
// set out, and returns the live set before instruction lo+1. Phis define
// their value but read nothing here.
func (g *gcInfo) backward(b *ssa.BasicBlock, hi, lo int, out map[ssa.Value]bool) map[ssa.Value]bool {
	live := make(map[ssa.Value]bool, len(out))
	for v := range out {
		live[v] = true
	}
	for j := hi; j > lo; j-- {
		ins := b.Instrs[j]
		if v, ok := ins.(ssa.Value); ok {
			delete(live, v)
		}
		if _, ok := ins.(*ssa.Phi); ok {
			continue
		}
		g.uses(ins, live, map[ssa.Value]bool{})
	}
	return live
}

// uintptrEscapes reports whether f has the //go:uintptrescapes directive.
func uintptrEscapes(f *ssa.Function) bool {
	if f == nil {
		return false
	}
	fd, ok := f.Syntax().(*ast.FuncDecl)
	if !ok || fd.Doc == nil {
		return false
	}
	for _, c := range fd.Doc.List {
		if strings.HasPrefix(c.Text, "//go:uintptrescapes") {
			return true
		}
	}
	return false
}

// uintptrArgs returns the pointers p of the uintptr(p) conversions passed
// by the call argument a: a itself, or the elements of the slice built for
// a variadic parameter.
func uintptrArgs(a ssa.Value) []ssa.Value {
	if cv, ok := a.(*ssa.Convert); ok && isUnsafePointer(cv.X.Type()) {
		return []ssa.Value{cv.X}
	}
	sl, ok := a.(*ssa.Slice)
	if !ok {
		return nil
	}
	arr, ok := sl.X.(*ssa.Alloc)
	if !ok {
		return nil
	}
	var out []ssa.Value
	for _, r := range *arr.Referrers() {
		ia, ok := r.(*ssa.IndexAddr)
		if !ok {
			continue
		}
		for _, r := range *ia.Referrers() {
			if st, ok := r.(*ssa.Store); ok && st.Addr == ia {
				if cv, ok := st.Val.(*ssa.Convert); ok && isUnsafePointer(cv.X.Type()) {
					out = append(out, cv.X)
				}
			}
		}
	}
	return out
}

func isUnsafePointer(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.UnsafePointer
}

// liveAfter returns the values live after instruction i (flat index) and the
// values the Recover block needs. Unless waits (the frame waits for the
// interpreted callee of instruction i), the operands of instruction i are
// live too: its code may still read them after the call it waits for (the
// copy back of a native call, for example).
func (g *gcInfo) liveAfter(fn *ssa.Function, i int, waits bool) map[ssa.Value]bool {
	b := fn.Blocks[g.blk[i]]
	live := g.backward(b, len(b.Instrs)-1, g.pos[i], g.out[g.blk[i]])
	if !waits {
		g.uses(g.ins[i], live, map[ssa.Value]bool{})
	}
	if call, ok := g.ins[i].(ssa.CallInstruction); ok && uintptrEscapes(call.Common().StaticCallee()) {
		// As gc does for a //go:uintptrescapes function, uintptr(p) in the
		// arguments of the call keeps the object of p alive until the call
		// returns.
		for _, a := range call.Common().Args {
			for _, x := range uintptrArgs(a) {
				if g.tracked(x) {
					live[x] = true
				}
			}
		}
	}
	for v := range g.keep {
		live[v] = true
	}
	return live
}
