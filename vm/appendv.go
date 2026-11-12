package vm

import (
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// append(s, x1, ..., xk) without the varargs array.
//
// go/ssa compiles append(s, x1, ..., xk) as an Alloc of a [k]T array (its
// Comment is "varargs"), one Store per element, a Slice of the whole array
// and the builtin call with that slice. The VM ran it as written: a heap
// cell, a []Value for the array, a boxed slice header, then the append.
// For append(b, c) on a []byte that was four allocations for one byte,
// and most of the allocations of code that builds a slice one element at
// a time. When nothing else uses the array, the compiler skips the four
// instructions and emits OpAppendV, which appends the element registers
// themselves (optAppendV; VM_OPT=-appendv keeps the old code).

// appendSite is the X of an OpAppendV.
type appendSite struct {
	elems []int32    // the registers of the appended elements, in order
	et    types.Type // the element type
	deep  bool       // needsCopy(et): elements are copied
	bytes bool       // et is byte: an append to a nil slice makes a Go []byte
	word  bool       // wordKind(et): elements are written with storeWord (wordcell.go)
}

// skipAppendVarargs finds the append calls whose varargs array is used by
// nothing else, records their elements in c.appendV, and marks the
// instructions that only build the array as skipped.
func (c *compiler) skipAppendVarargs() {
	if !optAppendV {
		return
	}
	for _, b := range c.f.ssa.Blocks {
		for _, ins := range b.Instrs {
			call, ok := ins.(*ssa.Call)
			if !ok {
				continue
			}
			if bi, ok := call.Call.Value.(*ssa.Builtin); !ok || bi.Name() != "append" {
				continue
			}
			elems, dead, ok := appendVarargs(call)
			if !ok {
				continue
			}
			if c.skip == nil {
				c.skip = map[ssa.Instruction]bool{}
			}
			if c.appendV == nil {
				c.appendV = map[*ssa.Call][]ssa.Value{}
			}
			for _, d := range dead {
				c.skip[d] = true
			}
			c.appendV[call] = elems
		}
	}
}

// appendVarargs reports whether call is append(s, x1, ..., xk) whose
// varargs array is used by nothing else. It returns the elements in order
// and the instructions that only build the array: the Alloc, one
// IndexAddr and one Store per element, and the Slice.
func appendVarargs(call *ssa.Call) (elems []ssa.Value, dead []ssa.Instruction, ok bool) {
	args := call.Call.Args
	if len(args) != 2 {
		return nil, nil, false
	}
	sl, ok := args[1].(*ssa.Slice)
	if !ok || sl.Low != nil || sl.High != nil || sl.Max != nil || !onlyReferrer(sl, call) {
		return nil, nil, false
	}
	a, ok := sl.X.(*ssa.Alloc)
	if !ok || a.Comment != "varargs" || a.Referrers() == nil {
		return nil, nil, false
	}
	at, ok := a.Type().(*types.Pointer).Elem().Underlying().(*types.Array)
	if !ok || at.Len() < 1 {
		return nil, nil, false
	}
	k := int(at.Len())
	elems = make([]ssa.Value, k)
	dead = append(dead, a)
	sliced := false
	for _, r := range *a.Referrers() {
		switch r := r.(type) {
		case *ssa.Slice:
			if r != sl || sliced {
				return nil, nil, false
			}
			sliced = true
		case *ssa.IndexAddr:
			i, ok := constIndex(r.Index)
			if !ok || i < 0 || i >= k || elems[i] != nil || r.Referrers() == nil || len(*r.Referrers()) != 1 {
				return nil, nil, false
			}
			st, ok := (*r.Referrers())[0].(*ssa.Store)
			if !ok || st.Addr != r {
				return nil, nil, false
			}
			elems[i] = st.Val
			dead = append(dead, r, st)
		default:
			return nil, nil, false
		}
	}
	if !sliced {
		return nil, nil, false
	}
	for _, e := range elems {
		if e == nil {
			return nil, nil, false // an element with no store: keep the array
		}
	}
	dead = append(dead, sl)
	return elems, dead, true
}

// onlyReferrer reports whether ins is the only instruction that uses v.
func onlyReferrer(v ssa.Value, ins ssa.Instruction) bool {
	refs := v.Referrers()
	return refs != nil && len(*refs) == 1 && (*refs)[0] == ins
}

// constIndex returns the value of an integer constant.
func constIndex(v ssa.Value) (int, bool) {
	c, ok := v.(*ssa.Const)
	if !ok || c.Value == nil {
		return 0, false
	}
	i, exact := constant.Int64Val(c.Value)
	return int(i), exact
}

// appendV runs OpAppendV: append(s, elems...) with the elements in
// registers. The elements go in one append, as in Go: when they do not
// all fit in the spare capacity, none of them is written into the
// array, which another slice may share (GOROOT test
// fixedbugs/issue79909.go).
func (m *Machine) appendV(regs []Value, s any, site *appendSite) Value {
	switch sv := s.(type) {
	case []byte:
		m.hostBytesAppend(sv, len(site.elems))
		return Value{R: appendBytesV(sv, regs, site.elems)}
	case nil:
		if site.bytes {
			return Value{R: appendBytesV(nil, regs, site.elems)}
		}
		return Value{R: m.appendCells(nil, regs, site)}
	case []Value:
		return Value{R: m.appendCells(sv, regs, site)}
	}
	// A zero-size element type, or a slice in native memory: the general
	// path, with the elements in a temporary slice.
	t := make([]Value, len(site.elems))
	for i, r := range site.elems {
		t[i] = regs[r]
	}
	return m.appendAny(s, t, site.et)
}

// appendBytesV is append(b, x1, ..., xk) for byte elements in registers.
func appendBytesV(b []byte, regs []Value, elems []int32) []byte {
	switch len(elems) {
	case 1:
		return append(b, byte(regs[elems[0]].N))
	case 2:
		return append(b, byte(regs[elems[0]].N), byte(regs[elems[1]].N))
	case 3:
		return append(b, byte(regs[elems[0]].N), byte(regs[elems[1]].N), byte(regs[elems[2]].N))
	}
	t := make([]byte, len(elems))
	for i, r := range elems {
		t[i] = byte(regs[r].N)
	}
	return append(b, t...)
}

// appendCells is append(s, x1, ..., xk) for element registers and cells.
func (m *Machine) appendCells(s []Value, regs []Value, site *appendSite) []Value {
	elems := site.elems
	if site.deep && len(elems) == 1 {
		return appendValues(m, s, regs[elems[0]:elems[0]+1], site.et)
	}
	if !site.deep && !site.word {
		switch len(elems) {
		case 1:
			return append(s, regs[elems[0]])
		case 2:
			return append(s, regs[elems[0]], regs[elems[1]])
		case 3:
			return append(s, regs[elems[0]], regs[elems[1]], regs[elems[2]])
		}
	}
	var buf [4]Value
	t := buf[:0]
	if len(elems) > len(buf) {
		t = make([]Value, 0, len(elems))
	}
	for _, r := range elems {
		t = append(t, regs[r])
	}
	if site.deep {
		return appendValues(m, s, t, site.et)
	}
	if site.word {
		return appendWords(s, t)
	}
	return append(s, t...)
}

// appendAny is append(b, t...) for any representation of b and t: a Go
// []byte, cells, a zero-size slice or a slice in native memory. et is the
// element type (nil when unknown).
func (m *Machine) appendAny(b, t any, et types.Type) Value {
	if bb, ok := b.([]byte); ok || (b == nil && isBytes(t)) {
		m.hostBytesAppend(bb, seqLen(t))
		return Value{R: appendBytes(bb, t)}
	}
	if z, ok := b.(*zslice); ok {
		return Value{R: z.appendN(seqLen(t))}
	}
	m.hostAppend(b)
	if v, ok := appendNat(b, t); ok {
		return v
	}
	s, _ := b.([]Value)
	if tz, ok := t.(*zslice); ok {
		// s has a zero-size element type too: the result is a zslice.
		z := &zslice{s: make([]struct{}, len(s)), zero: tz.zero, t: tz.t}
		return Value{R: z.appendN(len(tz.s))}
	}
	switch t := t.(type) {
	case []byte:
		for _, c := range t {
			s = append(s, Value{N: int64(c)})
		}
	case []Value:
		if et != nil && needsCopy(et) {
			s = appendValues(m, s, t, et)
		} else if et != nil && wordKind(et) {
			s = appendWords(s, t)
		} else {
			s = append(s, t...)
		}
	case string:
		for i := 0; i < len(t); i++ {
			s = append(s, Value{N: int64(t[i])})
		}
	}
	return Value{R: s}
}
