package vm

// Static boxes of interface values.
//
// OpMakeInterface makes a new *Iface for each value put in an interface.
// As gc does with its static array of small integers and its zero value,
// the VM gives one shared box per type to the integers and bools 0 to 255
// and to the empty string.
//
// An *Iface is never written after it is made: a method call copies the
// receiver out of it, and a type assertion copies the value. Sharing a box
// changes no result; only the address of the boxed payload is the same for
// two equal values, as in gc. These are tables of the VM, per RType, not
// of the Go runtime.
//
// VM_OPT=-boxes makes a new box each time.

import (
	"go/types"
	"sync/atomic"
)

// smallBoxes is the number of small integers that have a static box.
const smallBoxes = 256

// ifaceBoxes are the static boxes of a type.
type ifaceBoxes struct {
	small [smallBoxes]atomic.Pointer[Iface]
	empty atomic.Pointer[Iface] // ""
}

// boxSite is the site of an OpMakeIfaceBox: the type, and whether its
// values are integers or bools kept in Value.N, or strings.
type boxSite struct {
	rt    *RType
	small bool
	str   bool
}

// boxSiteOf returns the boxSite of a MakeInterface of a value of type t,
// or nil when its values have no static box (all but the integer, bool
// and string types of the program).
func (m *Machine) boxSiteOf(t types.Type, rt *RType) *boxSite {
	if !optBoxes || rt == nil || rt.Native != nil || rt.T == nil {
		return nil
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&(types.IsInteger|types.IsBoolean) != 0:
			return &boxSite{rt: rt, small: true}
		case u.Info()&types.IsString != 0:
			return &boxSite{rt: rt, str: true}
		}
		return nil
	}
	// Not the pointers, slices, maps, channels and funcs: their values in
	// an interface are seldom nil, and a non-nil one needs a new box anyway
	// (the site would only add a call).
	return nil
}

// box returns the *Iface of the value v of the site s: a static box when
// there is one, else a new box.
func (s *boxSite) box(v Value) *Iface {
	var p *atomic.Pointer[Iface]
	switch {
	case v.R == nil && s.small && uint64(v.N) < smallBoxes:
		p = &s.rt.boxTable().small[v.N]
	case s.str && v.R == "":
		// an empty substring keeps its data address in N
		// (emptySubstring); in an interface, as in gc, it has none
		p, v = &s.rt.boxTable().empty, Value{R: ""}
	default:
		return &Iface{T: s.rt, V: v}
	}
	if b := p.Load(); b != nil {
		return b
	}
	b := &Iface{T: s.rt, V: v}
	if p.CompareAndSwap(nil, b) {
		return b
	}
	return p.Load()
}

// boxTable returns the static boxes of rt, made at the first use.
func (rt *RType) boxTable() *ifaceBoxes {
	if t := rt.boxes.Load(); t != nil {
		return t
	}
	t := new(ifaceBoxes)
	if rt.boxes.CompareAndSwap(nil, t) {
		return t
	}
	return rt.boxes.Load()
}
