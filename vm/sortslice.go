package vm

// sort.Reverse, sort.Sort, sort.Stable and sort.IsSorted of an interpreted
// sort.IntSlice, sort.Float64Slice or sort.StringSlice.
//
// The slice is a []Value: native code gets a copy of it. The general call
// writes the copy back after the call, but sort.Reverse keeps the copy in
// the *sort.reverse it returns, so a later Swap or Sort through it never
// reaches the slice of the program. In AOT mode the glue of sort.Sort and
// sort.Stable does not write an interface argument back at all. These hooks
// give native sort a sort.Interface that reads and swaps the []Value itself.

import (
	"fmt"
	"go/types"
	"math"
	"sort"
)

// vmHooksAOT are the vmHooks that also replace the AOT copy of a function.
var vmHooksAOT = map[string]bool{}

func init() {
	for name, h := range map[string]NativeHook{
		"sort.Reverse":  func(m *Machine, a []Value) Value { return m.natIface(sort.Reverse(m.sortIface(a[0]))) },
		"sort.Sort":     func(m *Machine, a []Value) Value { sort.Sort(m.sortIface(a[0])); return Value{} },
		"sort.Stable":   func(m *Machine, a []Value) Value { sort.Stable(m.sortIface(a[0])); return Value{} },
		"sort.IsSorted": func(m *Machine, a []Value) Value { return Value{N: b2i(sort.IsSorted(m.sortIface(a[0])))} },
	} {
		vmHooks[name] = h
		vmHooksAOT[name] = true
	}
}

// sortIface returns the native sort.Interface of the interpreted value v.
// A *sort.IntSlice (and the others) reads the slice it points to at each
// call, as Go does: the hook does not write the native copy of the general
// case back, and the program can set *p after sort.Reverse(p).
func (m *Machine) sortIface(v Value) sort.Interface {
	if iv, ok := v.R.(*Iface); ok && iv.T.T != nil {
		if p, ok := iv.T.T.(*types.Pointer); ok {
			if cell, ok := iv.V.R.(*Value); ok {
				if k := sortSliceKind(p.Elem()); k != 0 {
					return &vmSortSlice{cell: cell, kind: k}
				}
			}
		} else if k := sortSliceKind(iv.T.T); k != 0 {
			if vals, ok := iv.V.R.([]Value); ok || iv.V.R == nil {
				return &vmSortSlice{vals: vals, kind: k}
			}
		}
	}
	p := m.prog.ImportedPackage("sort")
	if p == nil {
		panic("sort hook without package sort")
	}
	x, _ := m.toNative(v, p.Pkg.Scope().Lookup("Interface").Type()).Interface().(sort.Interface)
	return x
}

const (
	sortInts = iota + 1
	sortFloats
	sortStrings
)

// sortSliceKind returns the kind of t when it is sort.IntSlice,
// sort.Float64Slice or sort.StringSlice, else 0.
func sortSliceKind(t types.Type) int {
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != "sort" {
		return 0
	}
	switch n.Obj().Name() {
	case "IntSlice":
		return sortInts
	case "Float64Slice":
		return sortFloats
	case "StringSlice":
		return sortStrings
	}
	return 0
}

// vmSortSlice is an interpreted sort.IntSlice, sort.Float64Slice or
// sort.StringSlice (vals), or a pointer to one (cell), with the methods of
// the native type.
type vmSortSlice struct {
	vals []Value
	cell *Value
	kind int
}

// slice returns the elements: those of *cell now for a pointer.
func (s *vmSortSlice) slice() []Value {
	if s.cell != nil {
		vals, _ := s.cell.R.([]Value)
		return vals
	}
	return s.vals
}

func (s *vmSortSlice) Len() int { return len(s.slice()) }

func (s *vmSortSlice) Swap(i, j int) {
	vals := s.slice()
	vals[i], vals[j] = vals[j], vals[i]
}

func (s *vmSortSlice) Less(i, j int) bool {
	vals := s.slice()
	switch s.kind {
	case sortInts:
		return vals[i].N < vals[j].N
	case sortFloats:
		x, y := math.Float64frombits(uint64(vals[i].N)), math.Float64frombits(uint64(vals[j].N))
		return x < y || (math.IsNaN(x) && !math.IsNaN(y))
	}
	x, _ := vals[i].R.(string)
	y, _ := vals[j].R.(string)
	return x < y
}

// native returns a copy of the slice with its native type, nil for a nil
// slice (%#v prints sort.IntSlice(nil)).
func (s *vmSortSlice) native() any {
	vals := s.slice()
	switch s.kind {
	case sortInts:
		if vals == nil {
			return sort.IntSlice(nil)
		}
		x := make(sort.IntSlice, len(vals))
		for i, v := range vals {
			x[i] = int(v.N)
		}
		return x
	case sortFloats:
		if vals == nil {
			return sort.Float64Slice(nil)
		}
		x := make(sort.Float64Slice, len(vals))
		for i, v := range vals {
			x[i] = math.Float64frombits(uint64(v.N))
		}
		return x
	}
	if vals == nil {
		return sort.StringSlice(nil)
	}
	x := make(sort.StringSlice, len(vals))
	for i, v := range vals {
		x[i], _ = v.R.(string)
	}
	return x
}

// Format prints the slice as Go prints the native type, inside the
// *sort.reverse of sort.Reverse too.
func (s *vmSortSlice) Format(f fmt.State, verb rune) {
	fmt.Fprintf(f, fmt.FormatString(f, verb), s.native())
}

// String and GoString are for the AOT copy of fmt, whose Formatter takes
// its own fmt.State.
func (s *vmSortSlice) String() string   { return fmt.Sprint(s.native()) }
func (s *vmSortSlice) GoString() string { return fmt.Sprintf("%#v", s.native()) }
