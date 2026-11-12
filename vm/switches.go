package vm

// Switch tables.
//
// go/ssa compiles a Go switch on an integer to a chain of blocks that each
// test x == constant and go on to the next block when the test fails. A
// value that matches the last case runs every test before it: one compare
// and branch per case. A chain of at least minSwitch such tests on the
// same x becomes one OpSwitch, with a jump table when the constants are
// dense and a binary search otherwise, as gc compiles a switch.
//
// The tests of the chain compare integers: they have no effect and cannot
// panic, so jumping straight to the case does not change what the program
// does. The first test of a key wins, as in the chain. The blocks of the
// chain after the first one are still compiled (another block may jump
// into them), so their own code is the same as before.
//
// VM_OPT=-switch keeps the chains.

import (
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/ssa"
)

// minSwitch is the shortest chain that becomes an OpSwitch.
const minSwitch = 4

// switchSite is the table of an OpSwitch: a jump table tab[x-lo] when the
// keys are dense, else the sorted keys and their pcs. Every pc is the start
// of a block or of the phi moves of an edge.
type switchSite struct {
	lo   int64
	tab  []int32
	keys []int64
	pcs  []int32
	def  int32
}

// target is the pc for the value v.
func (s *switchSite) target(v int64) int {
	if s.tab != nil {
		if u := uint64(v - s.lo); u < uint64(len(s.tab)) {
			return int(s.tab[u])
		}
		return int(s.def)
	}
	lo, hi := 0, len(s.keys)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		if s.keys[m] < v {
			lo = m + 1
		} else {
			hi = m
		}
	}
	if lo < len(s.keys) && s.keys[lo] == v {
		return int(s.pcs[lo])
	}
	return int(s.def)
}

// switchChain is a chain of blocks that each test x == key and go on to the
// next one when the test fails.
type switchChain struct {
	x     ssa.Value
	cases []switchCase
	def   *ssa.BasicBlock // where the last test goes when it fails
	last  *ssa.BasicBlock
	pc    int   // the OpSwitch
	tpc   []int // the phi moves of the edge of each case, or -1
	dpc   int   // the phi moves of the default edge, or -1
}

type switchCase struct {
	key      int64
	from, to *ssa.BasicBlock
}

// chainTest returns x and the key when the If of b tests x == constant on
// an integer, and the comparison is used by this If only.
func (c *compiler) chainTest(b *ssa.BasicBlock) (ssa.Value, int64, bool) {
	if len(b.Instrs) == 0 {
		return nil, 0, false
	}
	iff, ok := b.Instrs[len(b.Instrs)-1].(*ssa.If)
	if !ok {
		return nil, 0, false
	}
	cmp, ok := iff.Cond.(*ssa.BinOp)
	if !ok || cmp.Op != token.EQL || cmp.Block() != b {
		return nil, 0, false
	}
	for _, r := range *cmp.Referrers() {
		if _, d := r.(*ssa.DebugRef); !d && r != ssa.Instruction(iff) {
			return nil, 0, false
		}
	}
	bt := basic(cmp.X.Type())
	if bt == nil || bt.Info()&types.IsInteger == 0 {
		return nil, 0, false
	}
	x, y := cmp.X, cmp.Y
	if _, ok := x.(*ssa.Const); ok {
		x, y = y, x
	}
	if _, ok := x.(*ssa.Const); ok {
		return nil, 0, false
	}
	k, ok := y.(*ssa.Const)
	if !ok {
		return nil, 0, false
	}
	// The key is the register value of the constant: what OpEqI compares.
	// c.reg(k) allocates the constant register even when the chain is
	// shorter than minSwitch. That is not wasted: the compare of the If
	// uses the same register when it is compiled.
	return x, c.template[c.reg(k)].N, true
}

// onlyTest reports whether b has nothing but its comparison and its If.
func onlyTest(b *ssa.BasicBlock) bool {
	n := 0
	for _, ins := range b.Instrs {
		if _, d := ins.(*ssa.DebugRef); !d {
			n++
		}
	}
	return n == 2
}

// switchChain returns the chain that starts with the If of b, or nil.
func (c *compiler) switchChain(b *ssa.BasicBlock) *switchChain {
	if !optSwitch || c.inChain[b] {
		return nil // a later test of a chain: only the OpSwitch jumps to it
	}
	x, k, ok := c.chainTest(b)
	if !ok {
		return nil
	}
	sw := &switchChain{x: x}
	seen := map[*ssa.BasicBlock]bool{b: true}
	cur := b
	for {
		sw.cases = append(sw.cases, switchCase{k, cur, cur.Succs[0]})
		next := cur.Succs[1]
		sw.def, sw.last = next, cur
		if seen[next] || len(next.Preds) != 1 || hasPhi(next) || !onlyTest(next) {
			break
		}
		x2, k2, ok := c.chainTest(next)
		if !ok || x2 != x {
			break
		}
		seen[next] = true
		cur, k = next, k2
	}
	if len(sw.cases) < minSwitch {
		return nil
	}
	if c.inChain == nil {
		c.inChain = map[*ssa.BasicBlock]bool{}
	}
	for _, cs := range sw.cases[1:] {
		c.inChain[cs.from] = true
	}
	return sw
}

// emitSwitch emits the OpSwitch of sw and the phi moves of its edges. The
// table is built by finish, when the blocks have their pcs.
func (c *compiler) emitSwitch(sw *switchChain) {
	sw.pc = c.emit(Instr{Op: OpSwitch, A: c.reg(sw.x)})
	sw.tpc = make([]int, len(sw.cases))
	for i, cs := range sw.cases {
		sw.tpc[i] = -1
		if hasPhi(cs.to) {
			sw.tpc[i] = len(c.code)
			c.edge(cs.from, cs.to, false)
		}
	}
	sw.dpc = -1
	if hasPhi(sw.def) {
		sw.dpc = len(c.code)
		c.edge(sw.last, sw.def, false)
	}
	c.switches = append(c.switches, sw)
}

// finish builds the switchSite of sw: a jump table when the keys are
// dense, else the sorted keys.
func (sw *switchChain) finish(c *compiler) {
	target := func(pc int, b *ssa.BasicBlock) int32 {
		if pc >= 0 {
			return int32(pc)
		}
		return int32(c.blockPC[b])
	}
	s := &switchSite{def: target(sw.dpc, sw.def)}
	seen := map[int64]bool{}
	type kp struct {
		k  int64
		pc int32
	}
	var ks []kp
	for i, cs := range sw.cases {
		if !seen[cs.key] {
			seen[cs.key] = true
			ks = append(ks, kp{cs.key, target(sw.tpc[i], cs.to)})
		}
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i].k < ks[j].k })
	lo, hi := ks[0].k, ks[len(ks)-1].k
	// hi >= lo after the sort, so span is right even when the keys cross
	// the int64 sign boundary: MinInt64 and MaxInt64 give 2^64-1.
	if span := uint64(hi) - uint64(lo); span < 1<<16 && span <= uint64(4*len(ks)+8) {
		s.lo = lo
		s.tab = make([]int32, span+1)
		for i := range s.tab {
			s.tab[i] = s.def
		}
		for _, e := range ks {
			s.tab[e.k-lo] = e.pc
		}
	} else {
		for _, e := range ks {
			s.keys = append(s.keys, e.k)
			s.pcs = append(s.pcs, e.pc)
		}
	}
	c.code[sw.pc].X = s
}
