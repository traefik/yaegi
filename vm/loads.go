package vm

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Loads of struct and array values that need no copy.
//
// go/ssa loads a struct or an array from memory with a Load (UnOp MUL).
// The VM copies the value (deepCopy, two allocations per struct or
// array level) so that the register holds what the memory held at that
// point, whatever is written later. Two kinds of loads do not need it:
//
//   - a dead load, one that nothing uses: the range expression of
//     `for i := range p.arr` (go/ssa loads the array; its length is a
//     constant). Go does not evaluate such a range expression at all
//     (unevaluatedRange, panics.go): the compiler emits neither the load
//     nor the address it would read, so a nil p does not panic, as in gc.
//     Another dead load is left out when its address cannot be nil (a
//     local, a global, or a field of one); else it stays for its nil
//     check;
//   - a shallow load, one whose value only goes to instructions that copy
//     it (a Store into a cell, a composite literal built in a register, an
//     append), in the same block, with nothing in between that can write
//     interpreted memory or run other code. The register then shares the
//     cells it was loaded from, and the consumer copies them.
//
// VM_OPT=-deadload and VM_OPT=-shallow keep the copies.

// markLoads fills c.shallow and marks the dead loads as skipped. It runs
// after skipAggregates: the stores of the literals and of the varargs
// arrays are skipped, and the spills are known.
func (c *compiler) markLoads() {
	fn := c.f.ssa
	pos := map[ssa.Instruction]int{}
	for _, b := range fn.Blocks {
		for i, ins := range b.Instrs {
			pos[ins] = i
		}
	}
	for _, b := range fn.Blocks {
		for i, ins := range b.Instrs {
			ld, ok := ins.(*ssa.UnOp)
			if !ok || ld.Op != token.MUL || c.skip[ld] || c.aggs[ld] != nil {
				continue
			}
			if _, ok := c.aliasReg(ld.X); ok {
				continue // a move, not a load (aggv.go)
			}
			refs := ld.Referrers()
			if refs == nil {
				continue
			}
			if deadValue(*refs) {
				if c.unevaluatedRange(ld) {
					c.skipDead(ld)
					c.skipAddrChain(ld.X)
				} else if optDeadLoad && nonNilAddr(ld.X) {
					c.skipDead(ld)
				}
				continue
			}
			if optShallow && needsCopy(ld.Type()) && c.shallowLoad(ld, b, i, *refs, pos) {
				if c.shallow == nil {
					c.shallow = map[*ssa.UnOp]bool{}
				}
				c.shallow[ld] = true
			}
		}
	}
}

// deadValue reports whether nothing reads a value with the referrers
// refs (a DebugRef is not a read).
func deadValue(refs []ssa.Instruction) bool {
	for _, r := range refs {
		if _, ok := r.(*ssa.DebugRef); !ok {
			return false
		}
	}
	return true
}

// skipDead marks ins as skipped.
func (c *compiler) skipDead(ins ssa.Instruction) {
	if c.skip == nil {
		c.skip = map[ssa.Instruction]bool{}
	}
	c.skip[ins] = true
}

// skipAddrChain marks as skipped the field and element addresses that
// only lead to a skipped load: the address of an unevaluated range
// expression is not computed either (no nil or bounds check, as in gc).
func (c *compiler) skipAddrChain(x ssa.Value) {
	for {
		switch a := x.(type) {
		case *ssa.FieldAddr:
			if a.Referrers() == nil || len(*a.Referrers()) != 1 {
				return
			}
			c.skipDead(a)
			x = a.X
		case *ssa.IndexAddr:
			if a.Referrers() == nil || len(*a.Referrers()) != 1 {
				return
			}
			c.skipDead(a)
			x = a.X
		default:
			return
		}
	}
}

// nonNilAddr reports whether the address x is known not to be nil: a
// local or a global, or a field of one.
func nonNilAddr(x ssa.Value) bool {
	switch x := x.(type) {
	case *ssa.Alloc, *ssa.Global:
		return true
	case *ssa.FieldAddr:
		return nonNilAddr(x.X)
	}
	return false
}

// shallowLoad reports whether the load ld (at index i of block b) may
// share the cells it loads: every referrer is a copying consumer in b,
// and the instructions between ld and each of them cannot write
// interpreted memory or run other code.
func (c *compiler) shallowLoad(ld *ssa.UnOp, b *ssa.BasicBlock, i int, refs []ssa.Instruction, pos map[ssa.Instruction]int) bool {
	last := i
	for _, u := range refs {
		if u.Block() != b || pos[u] <= i || !c.copies(u, ld) {
			return false
		}
		last = max(last, pos[u])
	}
	for _, ins := range b.Instrs[i+1 : last] {
		if !c.harmless(ins) {
			return false
		}
	}
	return true
}

// copies reports whether the instruction u copies the value v that it
// consumes: a Store of v into memory (assign, or a conversion to native
// memory), including the stores of a literal built in a register or of a
// varargs array of an append, which copy their struct and array elements.
// A store into a read-only local is not a copy (the local is an alias of
// the stored register), and a map update keeps the value as it is.
func (c *compiler) copies(u ssa.Instruction, v ssa.Value) bool {
	st, ok := u.(*ssa.Store)
	if !ok || st.Val != v || st.Addr == v {
		return false
	}
	if a, ok := st.Addr.(*ssa.Alloc); ok && c.spills[a] {
		return false
	}
	return true
}

// harmless reports whether ins, between a shallow load and its consumer,
// can neither write interpreted memory nor run other code. A skipped
// instruction runs nothing. An Alloc can write: OpAllocLocal (escape.go)
// sets its cell to zero in place. It is still harmless here, because the
// Alloc dominates every load of its cell and a shallow load and its
// consumer are in the same block, so the reset never runs between them.
func (c *compiler) harmless(ins ssa.Instruction) bool {
	if c.skip[ins] {
		return true
	}
	switch ins := ins.(type) {
	case *ssa.Alloc, *ssa.FieldAddr, *ssa.IndexAddr, *ssa.BinOp, *ssa.Convert, *ssa.ChangeType,
		*ssa.ChangeInterface, *ssa.MakeInterface, *ssa.MakeClosure, *ssa.Slice, *ssa.SliceToArrayPointer,
		*ssa.Field, *ssa.Index, *ssa.Extract, *ssa.Lookup, *ssa.MakeSlice, *ssa.MakeMap, *ssa.MakeChan,
		*ssa.TypeAssert, *ssa.MultiConvert, *ssa.DebugRef:
		return true
	case *ssa.UnOp:
		return ins.Op != token.ARROW
	case *ssa.Call:
		// A builtin that reads its arguments; append writes into the
		// spare capacity of its slice, which the load may share.
		if bi, ok := ins.Call.Value.(*ssa.Builtin); ok {
			switch bi.Name() {
			case "len", "cap", "min", "max", "real", "imag", "complex":
				return true
			}
		}
		return false
	}
	return false
}

// shallowSite returns the memory site of the load v: a shallow load keeps
// the cells it loads.
func (c *compiler) shallowSite(v *ssa.UnOp, t types.Type) *memSite {
	s := c.mem(t)
	s.shallow = c.shallow[v]
	return s
}

// copyLoad reports whether a load through s copies the value: a struct or
// an array, unless the load is shallow and no AOT shadow exists (deepCopy
// reads the shadows of the cells it copies; a shared cell would keep
// them).
func (s *memSite) copyLoad() bool {
	return s.deep && (!s.shallow || ownedShadows.Load() > 0)
}
