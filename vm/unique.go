package vm

// unique.Make and unique.Handle[T].Value for interpreted code.
//
// unique.Make is a generic function of a native package: no native instance
// of it exists, and its source needs runtime type data. The VM implements it.
//
// Handle[T] is struct{ value *T }. The value pointer of a VM handle is a
// native pointer (a reflect.Value of a *T allocated in native memory), the
// "native cell", and not a VM cell. So a handle keeps its identity when it
// goes through native code (a sync.Map, a context, any native container):
// its native copy holds the same pointer, and it comes back equal to
// unique.Make of the same value. A handle kept only in native memory also
// keeps its cell alive.
//
// As in Go's unique package, the set holds weak pointers to the native cells:
// a cell that no handle uses any more is freed, and a cleanup then removes
// its entries. A later Make of the same value creates a new cell, which is
// right because no old handle exists to compare it with.
//
// The interpreted value of each cell is kept beside it (vals), because the
// native copy of a value of an interpreted type can lose information (the
// methods of the dynamic type of an interface, for example).

import (
	"go/types"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
	"weak"
)

// uniqueSet is the set of unique.Make for one type T.
type uniqueSet struct {
	m    *Machine
	t    types.Type
	rt   reflect.Type // native type of T
	zero bool         // T has size 0: one cell for all values, never freed
	keys sync.Map     // keyOf(value) -> weak.Pointer[byte] to the native cell (a *byte when zero)
	vals sync.Map     // address of a native cell -> *uniqueVal
}

// uniqueVal is the interpreted value of a native cell.
type uniqueVal struct{ v Value }

// uniqueSetOf returns the set of unique.Make for the type t.
func (m *Machine) uniqueSetOf(t types.Type) *uniqueSet {
	rt := m.reflectType(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, _ := m.uniq.At(t).(*uniqueSet); s != nil {
		return s
	}
	s := &uniqueSet{m: m, t: t, rt: rt, zero: rt.Size() == 0}
	m.uniq.Set(t, s)
	return s
}

// handle returns the VM handle of the native cell p.
func (s *uniqueSet) handle(p *byte) Value {
	return Value{R: []Value{{R: reflect.NewAt(s.rt, unsafe.Pointer(p))}}}
}

// live returns the native cell of an entry of keys, or nil when it was freed.
func (s *uniqueSet) live(e any) *byte {
	if p, ok := e.(*byte); ok {
		return p
	}
	return e.(weak.Pointer[byte]).Value()
}

// make implements unique.Make(v).
func (s *uniqueSet) make(v Value) Value {
	k := keyOf(v, s.t)
	for {
		old, found := s.keys.Load(k)
		if found {
			if p := s.live(old); p != nil {
				return s.handle(p)
			}
		}
		cell := reflect.New(s.rt)
		if nv, ok := s.m.tryNative(func() reflect.Value { return s.m.toNative(v, s.t) }); ok && nv.Type() == s.rt {
			cell.Elem().Set(nv) // what native code sees (fmt prints the pointer only)
		}
		p := (*byte)(cell.UnsafePointer())
		addr := uintptr(unsafe.Pointer(p))
		val := &uniqueVal{v: deepCopy(v, s.t)}
		// The value is stored before the cell is published: a Make running
		// at the same time may return the cell and read it at once.
		s.vals.Store(addr, val)
		var e any = p
		if !s.zero {
			e = weak.Make(p)
		}
		var stored bool
		if found {
			stored = s.keys.CompareAndSwap(k, old, e)
		} else {
			_, loaded := s.keys.LoadOrStore(k, e)
			stored = !loaded
		}
		if !stored {
			s.vals.CompareAndDelete(addr, val)
			continue
		}
		if !s.zero {
			// The address of a freed cell can be used again before the
			// cleanup runs: only the entries of this cell are removed.
			runtime.AddCleanup(p, func(k any) {
				s.keys.CompareAndDelete(k, e)
				s.vals.CompareAndDelete(addr, val)
			}, k)
		}
		return s.handle(p)
	}
}

// value implements Handle[T].Value: h is the VM handle.
func (s *uniqueSet) value(h Value) Value {
	fs, _ := h.R.([]Value)
	if len(fs) == 0 || fs[0].R == nil {
		panic(runtimeError("invalid memory address or nil pointer dereference"))
	}
	rv := fs[0].R.(reflect.Value)
	if e, ok := s.vals.Load(uintptr(rv.UnsafePointer())); ok {
		return deepCopy(e.(*uniqueVal).v, s.t)
	}
	return s.m.fromNativeDeep(rv.Elem(), s.t)
}

// isUniqueHandle reports whether t is an instance of unique.Handle.
func isUniqueHandle(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.TypeArgs().Len() != 1 || n.Obj().Name() != "Handle" {
		return false
	}
	p := n.Obj().Pkg()
	return p != nil && p.Path() == "unique"
}

// handleFromNative converts the native copy of a handle back: the native
// cell pointer is the identity of the handle, it is kept as is.
func handleFromNative(rv reflect.Value) Value {
	f := rv.Field(0)
	if f.IsNil() {
		return Value{R: []Value{{}}}
	}
	return Value{R: []Value{{R: reflect.NewAt(f.Type().Elem(), f.UnsafePointer())}}}
}
