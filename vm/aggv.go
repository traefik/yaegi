package vm

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Composite literals in registers, and read-only locals as aliases.
//
// go/ssa builds a composite literal T{...} in memory: an Alloc of T
// (Comment "complit"), a Store per field or element through a FieldAddr
// or an IndexAddr, then a Load of the whole value. It also spills a
// parameter, or a receiver, whose fields are read into a local: an Alloc,
// a Store of the parameter, then a FieldAddr and a Load per field. The VM
// ran both as written: a heap cell, a copy of the zero value, and a copy
// at the Store or at the Load (deepCopy: two allocations per struct). A
// method with a value receiver paid three allocations per call for a
// receiver it only reads.
//
// When nothing else uses the memory, the compiler leaves it out:
//
//   - a literal becomes OpMakeAgg, which builds the value in a register
//     from the registers of its fields (aggSite);
//   - a local that is stored once and then only read is an alias of the
//     stored register: its FieldAddr and IndexAddr become OpField and
//     OpIndex on the register, and its Loads become moves. A register is
//     never changed in place (loads and stores copy structs and arrays),
//     so the alias reads what the memory would hold.
//
// Liveness for runtime.GC (gclive.go) sees the fields of a literal as
// read by its Load (Func.aggVals): a field register must not be cleared
// between its Store and the Load, which a call in a later field can
// separate.
//
// VM_OPT=-agglit and VM_OPT=-rospill keep the memory paths.

// maxAggCells bounds the literals built in a register. A bigger literal
// keeps the memory path, whose zero value is not a template kept alive
// (see compiler.mem).
const maxAggCells = 64

// aggLit is a composite literal built in a register.
type aggLit struct {
	t     types.Type // the struct or array type
	load  *ssa.UnOp  // the Load of the whole value, compiled as OpMakeAgg
	at    int        // the index of load in its block
	cells []int      // the cell of each written field or element
	vals  []ssa.Value
	types []types.Type
	elem  func(i int) types.Type // the type of cell i
}

// aggSite is the X of an OpMakeAgg.
type aggSite struct {
	zero   []Value      // the zero cells of the type: a template, never written
	cells  []int32      // the cell of each written field or element
	regs   []int32      // its register
	deep   []bool       // needsCopy of its type: the value is copied
	types  []types.Type // its type
	zdeep  []int32      // the unwritten cells whose zero is a struct or an array
	ztypes []types.Type // their types (the zero is deep copied)
}

// makeAgg runs OpMakeAgg.
func makeAgg(regs []Value, s *aggSite) Value {
	cells := make([]Value, len(s.zero))
	copy(cells, s.zero)
	for i, c := range s.zdeep {
		cells[c] = deepCopy(s.zero[c], s.ztypes[i])
	}
	for i, c := range s.cells {
		v := regs[s.regs[i]]
		if s.deep[i] {
			v = deepCopy(v, s.types[i])
		}
		cells[c] = v
	}
	return Value{R: cells}
}

// A literal whose only use is a Store right after its Load, *p = T{...},
// is OpStoreAgg: when p is a cell of the same shape and no field of the
// literal is a struct or an array, the fields are written into the cells
// p already has, as assign would copy them from the value of OpMakeAgg,
// without that value (a []Value and its box). Else it runs as OpMakeAgg
// then OpStore.

// aggStoreSite is the X of an OpStoreAgg.
type aggStoreSite struct {
	agg *aggSite
	mem *memSite // the site of the Store
	// flat: no cell of the literal is a struct or an array, written or
	// not: assign writes each cell as it is
	flat bool
	// words: the reg index (in agg.regs) of each cell, -1 when the cell
	// is not written; set only when a cell is a wordKind. Each cell is then
	// written once, with storeWord for a wordKind, so that a racy reader
	// of a field sees the old or the new value, never half of one or a
	// zero between them (wordcell.go).
	words []int32
	word  []bool
}

// storeInPlace writes the literal of s into the cell p when it can, as
// assign(p, makeAgg(...)) would. It reports whether it did.
func (s *aggStoreSite) storeInPlace(regs []Value, p Value) bool {
	if !s.flat {
		return false
	}
	cell, ok := p.R.(*Value)
	if !ok {
		return false
	}
	dst, ok := cell.R.([]Value)
	if !ok || len(dst) != len(s.agg.zero) {
		return false
	}
	if s.words != nil {
		for c, r := range s.words {
			v := s.agg.zero[c]
			if r >= 0 {
				v = regs[s.agg.regs[r]]
			}
			if s.word[c] {
				storeWord(&dst[c], v)
			} else {
				dst[c] = v
			}
		}
		return true
	}
	copy(dst, s.agg.zero)
	for i, c := range s.agg.cells {
		dst[c] = regs[s.agg.regs[i]]
	}
	return true
}

// aggStore returns the Store that takes the literal lit, when it is the
// only use of its Load and comes right after it (nothing runs between the
// two, so the fields are read when OpMakeAgg would read them), into an
// address that the Store compiles as an OpStore.
func (c *compiler) aggStore(lit *aggLit) *ssa.Store {
	ld := lit.load
	if !optAggLit {
		return nil
	}
	refs := ld.Referrers()
	if refs == nil || len(*refs) != 1 {
		return nil
	}
	st, ok := (*refs)[0].(*ssa.Store)
	if !ok || st.Val != ld || st.Addr == ld || st.Block() != ld.Block() || c.skip[st] {
		return nil // a skipped Store: the varargs array of an append (appendv.go)
	}
	switch a := st.Addr.(type) {
	case *ssa.Alloc:
		if c.spills[a] {
			return nil
		}
	case *ssa.FieldAddr:
		if c.fused[a] || isBlankField(a) {
			return nil
		}
	case *ssa.IndexAddr:
		if c.fused[a] {
			return nil
		}
	}
	if _, ok := c.aliasReg(st.Addr); ok {
		return nil
	}
	// lit.at, not a search of ld in the block: a package init that fills a
	// table has thousands of literals in one block, and a search for each
	// one made the compile time quadratic.
	instrs := ld.Block().Instrs
	if lit.at >= len(instrs) || instrs[lit.at] != ld {
		return nil
	}
	for _, ins := range instrs[lit.at+1:] {
		c.aggScan++
		if ins == st {
			return st
		}
		if _, ok := ins.(*ssa.DebugRef); !ok && !c.skip[ins] {
			return nil
		}
	}
	return nil
}

// skipAggregates finds the composite literals and the read-only locals of
// the function. It fills c.aggs, c.spills and c.aliases, and marks the
// instructions of the literals as skipped.
func (c *compiler) skipAggregates() {
	if !optAggLit && !optROSpill {
		return
	}
	fn := c.f.ssa
	pos := map[ssa.Instruction]int{} // index of each instruction in its block
	for _, b := range fn.Blocks {
		for i, ins := range b.Instrs {
			pos[ins] = i
		}
	}
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			a, ok := ins.(*ssa.Alloc)
			if !ok || c.skip[a] {
				continue
			}
			t := a.Type().(*types.Pointer).Elem()
			if !needsCopy(t) || c.m.isNativeNamed(t) || c.bytesAlloc(a) {
				continue
			}
			if optAggLit {
				if lit, dead, ok := aggLiteral(a, t, pos); ok {
					if c.skip == nil {
						c.skip = map[ssa.Instruction]bool{}
					}
					if c.aggs == nil {
						c.aggs = map[*ssa.UnOp]*aggLit{}
					}
					for _, d := range dead {
						c.skip[d] = true
					}
					c.aggs[lit.load] = lit
					continue
				}
			}
			if optROSpill {
				if addrs, ok := roSpill(a, pos); ok {
					if c.spills == nil {
						c.spills, c.aliases = map[*ssa.Alloc]bool{}, map[ssa.Value]bool{}
					}
					c.spills[a] = true
					for _, x := range addrs {
						c.aliases[x] = true
					}
				}
			}
		}
	}
}

// aggVals returns, for each literal built in a register, the values of its
// fields, keyed by the Alloc of the literal (for gclive.go).
func (c *compiler) aggVals() map[*ssa.Alloc][]ssa.Value {
	if len(c.aggs) == 0 {
		return nil
	}
	m := map[*ssa.Alloc][]ssa.Value{}
	for ld, lit := range c.aggs {
		m[ld.X.(*ssa.Alloc)] = lit.vals
	}
	return m
}

// aggLiteral reports whether a is the memory of a composite literal of
// type t that nothing else uses: its referrers are a FieldAddr (or an
// IndexAddr with a constant index) per written field, each stored once,
// and one Load of the whole value, all in the block of a, the Load last.
// It returns the literal and the instructions that only build it.
func aggLiteral(a *ssa.Alloc, t types.Type, pos map[ssa.Instruction]int) (*aggLit, []ssa.Instruction, bool) {
	if a.Referrers() == nil {
		return nil, nil, false
	}
	lit := &aggLit{t: t}
	var n int
	var isStruct bool
	switch u := t.Underlying().(type) {
	case *types.Struct:
		n, isStruct = u.NumFields(), true
		lit.elem = func(i int) types.Type { return u.Field(i).Type() }
	case *types.Array:
		if zeroSized(u.Elem()) {
			return nil, nil, false
		}
		n = int(u.Len())
		lit.elem = func(int) types.Type { return u.Elem() }
	default:
		return nil, nil, false
	}
	if n == 0 || n > maxAggCells {
		return nil, nil, false
	}
	written := make([]bool, n)
	dead := []ssa.Instruction{a}
	last := -1 // the position of the last store
	for _, r := range *a.Referrers() {
		if r.Block() != a.Block() {
			return nil, nil, false
		}
		switch r := r.(type) {
		case *ssa.UnOp:
			if r.Op != token.MUL || r.X != a || lit.load != nil {
				return nil, nil, false
			}
			lit.load = r
		case *ssa.FieldAddr:
			if !isStruct || r.X != a {
				return nil, nil, false
			}
			st, ok := singleStore(r)
			if !ok || st.Block() != a.Block() {
				return nil, nil, false
			}
			dead = append(dead, r, st)
			last = max(last, pos[st])
			if isBlankField(r) {
				continue // Go ignores the value: the field stays zero
			}
			if written[r.Field] {
				return nil, nil, false
			}
			written[r.Field] = true
			lit.cells = append(lit.cells, r.Field)
			lit.vals = append(lit.vals, st.Val)
			lit.types = append(lit.types, lit.elem(r.Field))
		case *ssa.IndexAddr:
			if isStruct || r.X != a {
				return nil, nil, false
			}
			i, ok := constIndex(r.Index)
			if !ok || i < 0 || i >= n || written[i] {
				return nil, nil, false
			}
			st, ok := singleStore(r)
			if !ok || st.Block() != a.Block() {
				return nil, nil, false
			}
			dead = append(dead, r, st)
			last = max(last, pos[st])
			written[i] = true
			lit.cells = append(lit.cells, i)
			lit.vals = append(lit.vals, st.Val)
			lit.types = append(lit.types, lit.elem(i))
		default:
			return nil, nil, false
		}
	}
	if lit.load == nil || pos[lit.load] < last {
		return nil, nil, false
	}
	lit.at = pos[lit.load]
	return lit, dead, true
}

// singleStore returns the Store of addr when it is the only use of addr.
func singleStore(addr ssa.Value) (*ssa.Store, bool) {
	refs := addr.Referrers()
	if refs == nil || len(*refs) != 1 {
		return nil, false
	}
	st, ok := (*refs)[0].(*ssa.Store)
	if !ok || st.Addr != addr || st.Val == addr {
		return nil, false
	}
	return st, true
}

// roSpill reports whether the local a is stored once, as a whole, and then
// only read: its other referrers are FieldAddr and IndexAddr chains that
// end in Loads, all reached after the Store. It returns the addresses of
// the chains.
func roSpill(a *ssa.Alloc, pos map[ssa.Instruction]int) (addrs []ssa.Value, ok bool) {
	if a.Heap || a.Referrers() == nil {
		return nil, false
	}
	var st *ssa.Store
	var uses []ssa.Instruction
	var walk func(addr ssa.Value) bool
	walk = func(addr ssa.Value) bool {
		refs := addr.Referrers()
		if refs == nil {
			return false
		}
		for _, r := range *refs {
			switch r := r.(type) {
			case *ssa.Store:
				if addr != a || st != nil || r.Addr != a || r.Val == a {
					return false
				}
				st = r
			case *ssa.UnOp:
				if r.Op != token.MUL || r.X != addr {
					return false
				}
				uses = append(uses, r)
			case *ssa.FieldAddr:
				if r.X != addr {
					return false
				}
				uses = append(uses, r)
				addrs = append(addrs, r)
				if !walk(r) {
					return false
				}
			case *ssa.IndexAddr:
				if r.X != addr {
					return false
				}
				uses = append(uses, r)
				addrs = append(addrs, r)
				if !walk(r) {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	if !walk(a) || st == nil || st.Block() != a.Block() {
		return nil, false
	}
	for _, u := range uses {
		if !st.Block().Dominates(u.Block()) || (u.Block() == st.Block() && pos[u] < pos[st]) {
			return nil, false
		}
	}
	return addrs, true
}

// aliasReg returns the register that holds the value behind the address
// x, when x is a read-only local or an address into one.
func (c *compiler) aliasReg(x ssa.Value) (int32, bool) {
	if a, ok := x.(*ssa.Alloc); ok && c.spills[a] {
		return c.reg(a), true
	}
	if c.aliases[x] {
		return c.reg(x), true
	}
	return 0, false
}

// aggStoreSiteOf builds the site of the OpStoreAgg of lit into the
// memory of mem.
func (c *compiler) aggStoreSiteOf(lit *aggLit, mem *memSite) *aggStoreSite {
	s := &aggStoreSite{agg: c.aggSiteOf(lit), mem: mem, flat: true}
	word := make([]bool, len(s.agg.zero))
	anyWord := false
	for i := range s.agg.zero {
		if needsCopy(lit.elem(i)) {
			s.flat = false
		}
		word[i] = wordKind(lit.elem(i))
		anyWord = anyWord || word[i]
	}
	if anyWord {
		s.word = word
		s.words = make([]int32, len(word))
		for c := range s.words {
			s.words[c] = -1
		}
		for i, c := range s.agg.cells {
			s.words[c] = int32(i)
		}
	}
	return s
}

// aggSiteOf builds the site of the OpMakeAgg of lit.
func (c *compiler) aggSiteOf(lit *aggLit) *aggSite {
	zero, _ := c.m.zero(lit.t).R.([]Value)
	s := &aggSite{zero: zero, regs: c.regList(lit.vals), types: lit.types}
	written := make([]bool, len(zero))
	for i, cell := range lit.cells {
		written[cell] = true
		s.cells = append(s.cells, int32(cell))
		s.deep = append(s.deep, needsCopy(lit.types[i]))
	}
	for i := range zero {
		if t := lit.elem(i); !written[i] && needsCopy(t) {
			s.zdeep = append(s.zdeep, int32(i))
			s.ztypes = append(s.ztypes, t)
		}
	}
	return s
}
