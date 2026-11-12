package vm

// Locals that stay in the frame.
//
// OpAlloc makes a new cell at each run of an Alloc: a struct local
// declared in a loop body costs a new cell and new []Value for its fields
// at each iteration, even when the program only reads it through a
// pointer it passes to a function. In
//
//	for _, r := range rules { ... evalOp(&r.Ops[j], v) }
//
// go/ssa puts r on the heap, because its address goes to a call, and the
// VM makes a new r at each iteration.
//
// For each parameter of an interpreted function, the VM computes whether
// a pointer passed in it can outlive the call (paramEscapes). An Alloc
// whose address only goes to loads, to stores into it, to comparisons, to
// the addresses of its fields and elements, and to such parameters of
// interpreted functions never outlives the run of its function: the
// frame keeps its cell in a register of its own (OpAllocLocal), and the
// next run of the Alloc in the same frame sets the same cell to zero again
// instead of making a new one. It is the same memory, as for a variable
// on the stack in gc: the value that the old instance held is dead,
// since nothing can still reach it.
//
// The rules are conservative. Anything not known escapes: a call of a
// native function, of a func value or of an interface method, a value
// stored somewhere, put in an interface, a phi, a closure, a defer, a go
// statement, a conversion (unsafe.Pointer), and any instruction that a
// newer x/tools adds (the default case). The register of the cell is set
// from the template of the function at each call, so a cell never goes
// from a running frame to another one; a frame that returned gives its
// cells to the next calls (see "Cells used again" below).
//
// An OpAllocLocal writes interpreted memory: it sets an existing cell to
// zero in place. loads.go (harmless) still lets a shallow load skip over
// an Alloc, because the Alloc dominates every load of its cell and a
// shallow load and its consumer are in the same block: the reset always
// runs before the load, never between the load and its consumer. A change
// to shallowLoad that breaks this must treat an Alloc as a write.
//
// Each function summary has its own depth budget (maxEscDepth bounds only
// the chain of derived pointers inside one function), so a long chain of
// helpers that only read the pointer keeps the local in the frame, and a
// summary does not depend on which function was compiled first.
//
// Recursive functions (f calls g calls f) get their summaries together, as
// a fixed point: the summaries that are not known yet start as "does not
// escape", and a function is computed again each time a summary that it
// read becomes "escapes", until nothing changes. A summary only goes from
// "does not escape" to "escapes", so this ends, and each function is
// computed at most once more per parameter of the functions it calls. The
// result is the smallest set of escaping parameters that agrees with the
// rules above: a pointer that a recursion only passes down and reads stays
// in the frame.
//
// VM_OPT=-escape turns it off.

import (
	"go/token"
	"sync"

	"golang.org/x/tools/go/ssa"
)

// escState holds the escape summaries of the interpreted functions of a
// machine: for each parameter, whether a pointer passed in it can outlive
// the call.
type escState struct {
	mu  sync.Mutex
	sum map[*ssa.Function][]bool
	// The fixed point being computed (paramEscapes): the summaries that
	// are not final yet, the functions that read each of them, the
	// functions to compute again, and the function being computed.
	pend  map[*ssa.Function][]bool
	deps  map[*ssa.Function][]*ssa.Function
	work  []*ssa.Function
	queue map[*ssa.Function]bool
	cur   *ssa.Function
}

// maxEscDepth bounds the walk through derived pointers (field and element
// addresses, slices) inside one function. A call does not use it: the
// summary of the callee starts with a fresh budget.
const maxEscDepth = 8

// frameLocal reports whether the address of the Alloc a never outlives
// the run of its function.
func (m *Machine) frameLocal(a *ssa.Alloc) bool {
	if !optEscape {
		return false
	}
	m.esc.mu.Lock()
	defer m.esc.mu.Unlock()
	if m.esc.sum == nil {
		m.esc.sum = map[*ssa.Function][]bool{}
	}
	return !m.escapes(a, 0)
}

// escapes reports whether the pointer (or slice) v, or a pointer derived
// from it, can outlive the current run of its function. m.esc.mu is held.
func (m *Machine) escapes(v ssa.Value, depth int) bool {
	if depth > maxEscDepth {
		return true
	}
	refs := v.Referrers()
	if refs == nil {
		return true
	}
	for _, r := range *refs {
		switch r := r.(type) {
		case *ssa.DebugRef:
		case *ssa.UnOp:
			if r.Op != token.MUL {
				return true
			}
		case *ssa.Store:
			if r.Val == v {
				return true // the pointer itself is stored somewhere
			}
		case *ssa.FieldAddr:
			if m.escapes(r, depth+1) {
				return true
			}
		case *ssa.IndexAddr:
			if m.escapes(r, depth+1) {
				return true
			}
		case *ssa.Slice:
			if m.escapes(r, depth+1) {
				return true
			}
		case *ssa.BinOp:
			if r.Op != token.EQL && r.Op != token.NEQ {
				return true
			}
		case *ssa.Call:
			if m.callEscapes(&r.Call, v) {
				return true
			}
		default:
			// Phi, MakeInterface, MakeClosure, Return, Defer, Go, Send,
			// MapUpdate, conversions, and the instructions of a newer
			// x/tools.
			return true
		}
	}
	return false
}

// callEscapes reports whether the call cc can keep the pointer v, which is
// one of its operands.
func (m *Machine) callEscapes(cc *ssa.CallCommon, v ssa.Value) bool {
	if cc.IsInvoke() || cc.Value == v {
		return true
	}
	switch fn := cc.Value.(type) {
	case *ssa.Builtin:
		switch fn.Name() {
		case "len", "cap", "copy":
			return false
		}
		return true
	case *ssa.Function:
		if !m.staticCallRunsBytecode(fn) {
			return true // native code, an intrinsic or a hook: it may keep it
		}
		for i, a := range cc.Args {
			if a == v && m.paramEscapes(fn, i) {
				return true
			}
		}
		return false
	}
	return true // a call through a func value
}

// staticCallRunsBytecode reports whether a static call of fn runs its SSA
// in the VM (OpCall in compiler.call), and not a native function, a
// generic intrinsic or the reflect of the plugin. compiler.call uses it to
// emit OpCall, and the escape summaries trust it: a new route in
// compiler.call that runs other code for an interpreted function must
// return false here, or a local could be reset while that code keeps its
// address.
func (m *Machine) staticCallRunsBytecode(fn *ssa.Function) bool {
	if !m.isInterp(fn) || isVMGeneric(fn) {
		return false
	}
	if m.progVR {
		o := fn
		if fn.Origin() != nil {
			o = fn.Origin()
		}
		if o.Pkg != nil && isReflectPkg(o.Pkg.Pkg) {
			return false
		}
	}
	return true
}

// paramEscapes reports whether parameter i of fn can keep a pointer passed
// in it. The summary does not depend on the caller: it starts with a fresh
// depth budget, so the cached result is the same whoever computes it.
// Called while a fixed point is computed, it gives the current summary of
// fn (see above).
func (m *Machine) paramEscapes(fn *ssa.Function, i int) bool {
	e := &m.esc
	if s, ok := e.sum[fn]; ok {
		return i >= len(s) || s[i]
	}
	if e.pend != nil {
		s, ok := e.pend[fn]
		if !ok {
			s = make([]bool, len(fn.Params))
			e.pend[fn] = s
			e.push(fn)
		}
		if d := e.deps[fn]; len(d) == 0 || d[len(d)-1] != e.cur {
			e.deps[fn] = append(d, e.cur)
		}
		return i >= len(s) || s[i]
	}
	e.pend = map[*ssa.Function][]bool{fn: make([]bool, len(fn.Params))}
	e.deps = map[*ssa.Function][]*ssa.Function{}
	e.queue = map[*ssa.Function]bool{}
	e.push(fn)
	for len(e.work) > 0 {
		g := e.work[len(e.work)-1]
		e.work = e.work[:len(e.work)-1]
		delete(e.queue, g)
		e.cur = g
		old := e.pend[g]
		changed := false
		for j, p := range g.Params {
			if !old[j] && m.escapes(p, 0) {
				old[j], changed = true, true
			}
		}
		if changed {
			for _, d := range e.deps[g] {
				e.push(d)
			}
		}
	}
	for g, s := range e.pend {
		e.sum[g] = s
	}
	e.pend, e.deps, e.queue, e.cur = nil, nil, nil, nil
	s := e.sum[fn]
	return i >= len(s) || s[i]
}

// push adds fn to the functions to compute again, once.
func (e *escState) push(fn *ssa.Function) {
	if !e.queue[fn] {
		e.queue[fn] = true
		e.work = append(e.work, fn)
	}
}

// allocLocal runs OpAllocLocal: the cell kept in register B, set to zero
// again, or a cell that a frame of the same function gave back when it
// returned (freeLocals, already zero), or a new one, which register B
// keeps.
//
//go:noinline
func (cx *ctx) allocLocal(regs []Value, in *Instr) {
	s := in.X.(*memSite)
	if cell, ok := regs[in.B].R.(*Value); ok {
		resetCell(cell, s)
		regs[in.A] = regs[in.B]
		return
	}
	if cell := cx.reuseCell(s); cell != nil {
		regs[in.A] = Value{R: cell}
	} else {
		regs[in.A] = Value{R: newCell(s)}
	}
	regs[in.B] = regs[in.A]
	// The new cell counts in MaxStack and MaxMemory (stacklimit.go). It is
	// in its register first: stackUse counts it there.
	if cx.cells += s.cellVals; cx.m.MaxStack > 0 {
		cx.checkStack(cx.topRegs())
	}
	if cx.m.mem != nil && cx.cells*valueSize > cx.charged {
		cx.chargeMem()
	}
}

// Cells used again from one call to the next.
//
// The cells of the frame-local Allocs of a frame are dead when the frame
// returns: nothing that the frame gave to other code can still reach them
// (the rules above). A normal return (OpRet) sets them to zero and puts
// them in the free list of their site, in the ctx, and the next
// OpAllocLocal of the same site on that ctx takes one back instead of
// making a new one. Set to zero, a free cell keeps nothing alive but its
// own memory (memSite.localSize, at most maxFreeCells for a ctx). A function called in a loop, or a recursion, then allocates no
// cell after its first calls, as gc keeps such locals on the stack. A
// frame that a panic or a Goexit unwinds gives nothing back.
//
// The free lists stay with the ctx when it goes back to the pool (a call
// from the host, such as a request of a plugin, takes a pooled ctx), so
// they also work from one call from the host to the next. The index of a
// site (memSite.local) is only unique in its Machine, and a pooled ctx can
// serve another Machine next: a free list remembers its site, and a list
// of another site is dropped. The pool itself is emptied by the GC.

// localCell is the register of the cell of a frame-local Alloc of a
// function, and its site.
type localCell struct {
	reg  int32
	site *memSite
}

// freeList holds the free cells of one site.
type freeList struct {
	site  *memSite
	cells []*Value
}

// maxFreeCells bounds the Values that the free cells of a ctx hold, as
// pooledStack bounds the register stack that a pooled ctx keeps: a deep
// recursion that is over keeps at most this much memory (1.5 MB).
const maxFreeCells = pooledStack

// reuseCell takes a free cell of the site s, or returns nil.
func (cx *ctx) reuseCell(s *memSite) *Value {
	if int(s.local) >= len(cx.freeCells) {
		return nil
	}
	f := &cx.freeCells[s.local]
	if f.site != s || len(f.cells) == 0 {
		return nil
	}
	cell := f.cells[len(f.cells)-1]
	f.cells[len(f.cells)-1] = nil
	f.cells = f.cells[:len(f.cells)-1]
	cx.freeSize -= int(s.localSize)
	return cell
}

// freeLocals runs at a normal return of fn: it sets the cells of its
// frame-local Allocs to zero and puts them in their free lists.
func (cx *ctx) freeLocals(fn *Func, regs []Value) {
	for _, l := range fn.locals {
		cell, ok := regs[l.reg].R.(*Value)
		if !ok {
			continue
		}
		// The register would keep the cell too: the next frame that
		// uses this part of the stack starts from its template anyway.
		regs[l.reg] = Value{}
		cx.cells -= l.site.cellVals
		n := int(l.site.localSize)
		if cx.freeSize+n > maxFreeCells {
			continue
		}
		i := int(l.site.local)
		if i >= len(cx.freeCells) {
			cx.freeCells = append(cx.freeCells, make([]freeList, i+1-len(cx.freeCells))...)
		}
		f := &cx.freeCells[i]
		if f.site != l.site {
			// A new list, or the list of a site of another Machine (a
			// pooled ctx).
			if f.site != nil {
				cx.freeSize -= len(f.cells) * int(f.site.localSize)
			}
			clear(f.cells)
			f.site, f.cells = l.site, f.cells[:0]
		}
		resetCell(cell, l.site)
		f.cells = append(f.cells, cell)
		cx.freeSize += n
	}
}

// resetCell sets the cell of a frame-local Alloc (OpAllocLocal) to the
// zero value of its site again, in place.
func resetCell(cell *Value, s *memSite) {
	switch {
	case s.flat > 0:
		if r, ok := cell.R.([]Value); ok && len(r) == s.flat && cell.N == 0 {
			clear(r)
			return
		}
		*cell = Value{R: make([]Value, s.flat)}
	case s.deep:
		// assign writes the zero into the cells the local already has,
		// and makes new ones where the shape differs.
		assign(cell, s.zero, s.t)
	default:
		*cell = s.zero
	}
}

// newCell makes the cell of an Alloc, as OpAlloc does.
func newCell(s *memSite) *Value {
	cell := new(Value)
	if s.flat > 0 {
		// A zero array of plain values: a fresh slice is already zero.
		cell.R = make([]Value, s.flat)
	} else if s.deep {
		*cell = deepCopy(s.zero, s.t)
	} else {
		*cell = s.zero
	}
	return cell
}
