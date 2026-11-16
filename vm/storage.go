package vm

// sync.Map and atomic.Value with interpreted values.
//
// These native containers only store what they are given. In the default
// mode an interpreted value passed to a native `any` parameter becomes a
// native copy, which comes back without its interpreted type (r12b:
// "interface is struct { User string }, not main.Session"); keys of
// interpreted types would also need native equality. The methods are
// replaced by hooks that store interpreted values as vmStored: the dynamic
// type plus a payload, converted back to the original VM value when it is
// read (vmStored implements vmBoxed). Native values and values of
// predeclared types (string, int...) stay native, so that native code that
// shares the container sees them as usual. The AOT mode needs the hooks too:
// its opaque handles do not compare equal from one call to the next, so keys
// of interpreted types were not found.
//
// A key must be comparable by native code: a struct or array key is its
// type plus keyOf, the key of the maps of the VM (storedKey), and the value
// of its entry holds the key itself for Range (vmEntry). A value is never
// compared by native code: it is
// a *vmStored that holds the VM value itself (storedVal), so that a struct
// that comes back shares its slices, maps and pointers with the stored one,
// as in Go. CompareAndSwap and CompareAndDelete compare with the == of the
// VM, then swap the loaded *vmStored, which native code compares by
// identity. atomic.Value checks the interpreted types itself: all the
// *vmStored have the same native type.

import (
	"go/types"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// vmStored is an interpreted value stored in a native container.
type vmStored struct {
	m  *Machine
	rt *RType
	v  any // payload: the VM reference (pointer, slice, map, chan, func) or the Value
}

func (s vmStored) iface() *Iface {
	if x, ok := s.v.(Value); ok {
		return &Iface{T: s.rt, V: x}
	}
	return &Iface{T: s.rt, V: Value{R: s.v}}
}

func (s vmStored) obj() vmObj { return vmObj{m: s.m, iv: s.iface()} }

// vmMapKey is the key of a sync.Map for a struct or an array of the program:
// its dynamic type and keyOf of its value, with the == of the VM (a pointer
// field compares by identity). The key itself cannot be read back from it:
// the value of the entry is a *vmEntry, which holds it for Range.
type vmMapKey struct {
	rt *RType
	k  any
}

// vmEntry is the value of a sync.Map entry whose key is a vmMapKey.
type vmEntry struct {
	key Value // the key, of type any
	val any   // the stored value
}

// storedVal returns the form in which a VM value of type any is stored as
// the value of a sync.Map or of an atomic.Value. The interface holds its own
// copy of a struct or an array, which nobody writes: it is shared, not
// copied, so its slices, maps and pointers stay the program's own.
func (m *Machine) storedVal(v Value) any {
	iv, _ := v.R.(*Iface)
	switch {
	case iv == nil:
		return nil
	case iv.T.Native != nil:
		return iv.V.R.(reflect.Value).Interface()
	}
	if _, ok := types.Unalias(iv.T.T).(*types.Basic); ok {
		return m.box(iv, newConv(nil)).Interface() // predeclared type: native
	}
	return &vmStored{m: m, rt: iv.T, v: iv.V}
}

// storedEntry is storedVal for the value of the key k (a storedKey) of a
// sync.Map.
func (m *Machine) storedEntry(k any, key, v Value) any {
	x := m.storedVal(v)
	if _, ok := k.(vmMapKey); ok {
		return &vmEntry{key: key, val: x}
	}
	return x
}

// storedKey returns the form in which a VM value of type any is stored as
// a key of a sync.Map: native code hashes it and compares it.
func (m *Machine) storedKey(v Value) any {
	iv, _ := v.R.(*Iface)
	switch {
	case iv == nil:
		return nil
	case iv.T.Native != nil:
		return iv.V.R.(reflect.Value).Interface()
	}
	t := types.Unalias(iv.T.T)
	if _, ok := t.(*types.Basic); ok {
		return m.box(iv, newConv(nil)).Interface() // predeclared type: native
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return vmStored{m: m, rt: iv.T, v: iv.V.R}
	case *types.Basic:
		return vmStored{m: m, rt: iv.T, v: iv.V}
	}
	// Struct or array: the key of the maps of the VM. A native copy would
	// give a new native pointer for each pointer field at each call.
	return vmMapKey{rt: iv.T, k: keyOf(iv.V, iv.T.T)}
}

// anyVM converts a stored value back to a VM value of type any.
func (m *Machine) anyVM(x any) Value {
	if x == nil {
		return Value{}
	}
	switch s := x.(type) {
	case *vmStored:
		return Value{R: s.iface()}
	case vmStored:
		return Value{R: s.iface()}
	case *vmEntry:
		return m.anyVM(s.val)
	}
	return m.fromNative(reflect.ValueOf(&x).Elem(), emptyIface)
}

// storageHook replaces a method of a native container (f gives the
// receiver type).
type storageHook func(m *Machine, f *ssa.Function, a []Value) Value

var storageHooks = map[string]storageHook{}

func syncMapOf(m *Machine, f *ssa.Function, recv Value) *sync.Map {
	sm, _ := m.toNative(recv, f.Signature.Recv().Type()).Interface().(*sync.Map)
	if sm == nil {
		panic(errNilDeref)
	}
	return sm
}

func atomicValueOf(m *Machine, f *ssa.Function, recv Value) *atomic.Value {
	av, _ := m.toNative(recv, f.Signature.Recv().Type()).Interface().(*atomic.Value)
	if av == nil {
		panic(errNilDeref)
	}
	return av
}

// mustHash panics as gc does for a key that cannot be hashed. A map or a
// func of the program is stored as a pointer, which native code would hash,
// and in the AOT mode a slice in an any field is an opaque handle: the key
// is checked here, with the types of the program.
func mustHash(key Value) {
	if name := unhashableIn(key, emptyIface); name != "" {
		panic(runtimeError("hash of unhashable type " + name))
	}
}

// unhashableIn returns the first type that gc cannot hash in v of type t, in
// the order gc hashes: the dynamic type of an interface, then the fields of
// a struct and the elements of an array. It returns "" when v can be hashed.
func unhashableIn(v Value, t types.Type) string {
	if rv, ok := v.R.(reflect.Value); ok {
		// a native value, also of a type of an AOT copy (go/ast.Object)
		return unhashableNative(rv)
	}
	switch u := t.Underlying().(type) {
	case *types.Interface:
		iv, _ := v.R.(*Iface)
		switch {
		case iv == nil:
			return ""
		case iv.T.Native != nil:
			return unhashableNative(iv.V.R.(reflect.Value))
		case iv.T.T == nil:
			return ""
		case !types.Comparable(iv.T.T):
			return typeString(iv.T.T)
		}
		return unhashableIn(iv.V, iv.T.T)
	case *types.Struct:
		fs, _ := v.R.([]Value)
		for i := range fs {
			if f := u.Field(i); f.Name() != "_" {
				if name := unhashableIn(loadCell(&fs[i], f.Type()), f.Type()); name != "" {
					return name
				}
			}
		}
	case *types.Array:
		es, _ := v.R.([]Value)
		for i := range es {
			if name := unhashableIn(loadCell(&es[i], u.Elem()), u.Elem()); name != "" {
				return name
			}
		}
	}
	return ""
}

// unhashableNative is unhashableIn for a native value.
func unhashableNative(rv reflect.Value) string {
	if !rv.IsValid() {
		return ""
	}
	if !rv.Type().Comparable() {
		return rv.Type().String()
	}
	switch rv.Kind() {
	case reflect.Interface:
		// In the AOT mode, a value of the program in an any field of a
		// native value is an opaque handle, which native code can hash.
		if e := rv.Elem(); e.IsValid() && e.CanInterface() {
			if b, ok := e.Interface().(vmBoxed); ok && b.iface() != nil {
				return unhashableIn(Value{R: b.iface()}, emptyIface)
			}
		}
		return unhashableNative(rv.Elem())
	case reflect.Struct:
		for i := range rv.NumField() {
			if rv.Type().Field(i).Name != "_" {
				if name := unhashableNative(rv.Field(i)); name != "" {
					return name
				}
			}
		}
	case reflect.Array:
		for i := range rv.Len() {
			if name := unhashableNative(rv.Index(i)); name != "" {
				return name
			}
		}
	}
	return ""
}

// casStored is CompareAndSwap (del false) or CompareAndDelete (del true) of
// sync.Map. The current value is compared with old by the VM, which panics
// as gc does when both have the same uncomparable type; then the native
// call swaps or deletes the loaded value, which it compares by identity
// (a *vmStored), or by value for a native one. It loads again when another
// goroutine changed the value in between.
func casStored(m *Machine, sm *sync.Map, key, old, new Value, del bool) bool {
	k := m.storedKey(key)
	var nv any
	if !del {
		nv = m.storedEntry(k, key, new)
	}
	for {
		cur, ok := sm.Load(k)
		if !ok || !m.equal(m.anyVM(cur), old, emptyIface) {
			return false
		}
		if del && sm.CompareAndDelete(k, cur) || !del && sm.CompareAndSwap(k, cur, nv) {
			return true
		}
	}
}

// sameStoredType reports whether two stored values that are not nil have
// the same dynamic type, for atomic.Value.
func sameStoredType(a, b any) bool {
	sa, aok := a.(*vmStored)
	sb, bok := b.(*vmStored)
	switch {
	case aok && bok:
		return sa.rt == sb.rt || sa.rt.T != nil && sb.rt.T != nil && types.Identical(sa.rt.T, sb.rt.T)
	case aok || bok:
		return false
	}
	return reflect.TypeOf(a) == reflect.TypeOf(b)
}

// atomicStore is Store (swap false) or Swap (swap true) of atomic.Value
// with x, a stored value. Native code panics on a nil x and on a change
// between a native type and a *vmStored; the type of a *vmStored is checked
// here. The type of an atomic.Value never changes after the first store,
// so the check stays true until the native call.
func atomicStore(av *atomic.Value, x any, swap bool, op string) any {
	if s, ok := x.(*vmStored); ok {
		for {
			cur := av.Load()
			if cur == nil {
				if av.CompareAndSwap(nil, s) {
					return nil
				}
				continue
			}
			if _, isVM := cur.(*vmStored); isVM && !sameStoredType(cur, s) {
				panic("sync/atomic: " + op + " of inconsistently typed value into Value")
			}
			break
		}
	}
	if swap {
		return av.Swap(x)
	}
	av.Store(x)
	return nil
}

// atomicCAS is CompareAndSwap of atomic.Value, with the checks of gc in the
// same order, the types and the == of the VM.
func atomicCAS(m *Machine, av *atomic.Value, old, new Value) bool {
	nv := m.storedVal(new)
	if nv == nil {
		panic("sync/atomic: compare and swap of nil value into Value")
	}
	if ov := m.storedVal(old); ov != nil && !sameStoredType(ov, nv) {
		panic("sync/atomic: compare and swap of inconsistently typed values")
	}
	for {
		cur := av.Load()
		if cur == nil {
			if old.R != nil {
				return false
			}
			if av.CompareAndSwap(nil, nv) {
				return true
			}
			continue
		}
		if !sameStoredType(cur, nv) {
			panic("sync/atomic: compare and swap of inconsistently typed value into Value")
		}
		if !m.equal(m.anyVM(cur), old, emptyIface) {
			return false
		}
		return av.CompareAndSwap(cur, nv)
	}
}

func init() {
	pair := func(m *Machine, x any, ok bool) Value { return Value{R: []Value{m.anyVM(x), {N: b2i(ok)}}} }
	for name, h := range map[string]storageHook{
		"(*sync.Map).Load": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			v, ok := syncMapOf(m, f, a[0]).Load(m.storedKey(a[1]))
			return pair(m, v, ok)
		},
		"(*sync.Map).Store": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			k := m.storedKey(a[1])
			syncMapOf(m, f, a[0]).Store(k, m.storedEntry(k, a[1], a[2]))
			return Value{}
		},
		"(*sync.Map).LoadOrStore": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			k := m.storedKey(a[1])
			v, ok := syncMapOf(m, f, a[0]).LoadOrStore(k, m.storedEntry(k, a[1], a[2]))
			return pair(m, v, ok)
		},
		"(*sync.Map).LoadAndDelete": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			v, ok := syncMapOf(m, f, a[0]).LoadAndDelete(m.storedKey(a[1]))
			return pair(m, v, ok)
		},
		"(*sync.Map).Delete": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			syncMapOf(m, f, a[0]).Delete(m.storedKey(a[1]))
			return Value{}
		},
		"(*sync.Map).Swap": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			k := m.storedKey(a[1])
			v, ok := syncMapOf(m, f, a[0]).Swap(k, m.storedEntry(k, a[1], a[2]))
			return pair(m, v, ok)
		},
		"(*sync.Map).CompareAndSwap": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			return Value{N: b2i(casStored(m, syncMapOf(m, f, a[0]), a[1], a[2], a[3], false))}
		},
		"(*sync.Map).CompareAndDelete": func(m *Machine, f *ssa.Function, a []Value) Value {
			mustHash(a[1])
			return Value{N: b2i(casStored(m, syncMapOf(m, f, a[0]), a[1], a[2], Value{}, true))}
		},
		"(*sync.Map).Range": func(m *Machine, f *ssa.Function, a []Value) Value {
			fn := a[1]
			syncMapOf(m, f, a[0]).Range(func(k, v any) bool {
				kv := m.anyVM(k)
				if e, ok := v.(*vmEntry); ok {
					kv = e.key
				}
				return m.callValue(fn, []Value{kv, m.anyVM(v)}).N != 0
			})
			return Value{}
		},
		"(*sync.Map).Clear": func(m *Machine, f *ssa.Function, a []Value) Value {
			syncMapOf(m, f, a[0]).Clear()
			return Value{}
		},
		"(*sync/atomic.Value).Load": func(m *Machine, f *ssa.Function, a []Value) Value {
			return m.anyVM(atomicValueOf(m, f, a[0]).Load())
		},
		"(*sync/atomic.Value).Store": func(m *Machine, f *ssa.Function, a []Value) Value {
			atomicStore(atomicValueOf(m, f, a[0]), m.storedVal(a[1]), false, "store")
			return Value{}
		},
		"(*sync/atomic.Value).Swap": func(m *Machine, f *ssa.Function, a []Value) Value {
			return m.anyVM(atomicStore(atomicValueOf(m, f, a[0]), m.storedVal(a[1]), true, "swap"))
		},
		"(*sync/atomic.Value).CompareAndSwap": func(m *Machine, f *ssa.Function, a []Value) Value {
			return Value{N: b2i(atomicCAS(m, atomicValueOf(m, f, a[0]), a[1], a[2]))}
		},
	} {
		storageHooks[name] = h
	}
}

// storageHook returns the replacement of f, or nil.
func (m *Machine) storageHook(f *ssa.Function) storageHook {
	if funcPkg(f) == nil || m.isInterp(f) || f.Signature.Recv() == nil {
		return nil
	}
	return storageHooks[f.String()]
}

// runtime.SetFinalizer on an interpreted pointer: the native call would need
// a native copy (reflect.StructOf fails on embedded unexported fields, and
// an opaque handle is a fatal error), and a finalizer on a copy would run at
// the wrong time anyway. The finalizer is set on the VM cell; it gets the
// cell back as the interpreted pointer. A native pointer (a field of a
// native struct, the AOT mode) gets a native func that calls the
// interpreted finalizer.
func init() {
	vmHooks["runtime.SetFinalizer"] = func(m *Machine, a []Value) Value {
		iv, _ := a[0].R.(*Iface)
		fiv, _ := a[1].R.(*Iface)
		m.checkFinalizer(iv, fiv) // PanicOnFatal: a panic instead of a throw
		inst := m.curInstance()   // the finalizer runs in the instance of this call
		if iv == nil {
			panic("runtime.SetFinalizer: first argument is nil")
		}
		cell, ok := iv.V.R.(*Value)
		if bp, isB := iv.V.R.(*byte); isB && iv.T.Native == nil {
			// A *byte into Go memory (unsafe.StringData, unsafe.SliceData
			// of a native []byte): set the finalizer on it (GOROOT test
			// fixedbugs/issue57823.go).
			if fiv == nil {
				m.setFinalizer(unsafe.Pointer(bp), true, func() { runtime.SetFinalizer(bp, nil) })
				return Value{}
			}
			run := m.vmFinalizer(iv.T, fiv, inst)
			track := m.panicOnFatal
			m.setFinalizer(unsafe.Pointer(bp), false, func() {
				runtime.SetFinalizer(bp, func(b *byte) {
					if track {
						finalized.Delete(uintptr(unsafe.Pointer(b)))
					}
					run(Value{R: b})
				})
			})
			return Value{}
		}
		if iv.T.Native != nil || !ok {
			obj := m.box(iv, newConv(nil))
			if obj.Kind() == reflect.Pointer && !obj.IsNil() {
				// The pointer that the runtime gets, maybe not the one
				// that checkFinalizer saw.
				m.checkFinalizerStart(obj.UnsafePointer(), obj.Type().Elem())
			}
			call := func(fin reflect.Value) func() {
				return func() { reflect.ValueOf(runtime.SetFinalizer).Call([]reflect.Value{obj, fin}) }
			}
			if fiv == nil {
				// A nil any, not a typed nil func: SetFinalizer removes
				// the finalizer only when its argument is a nil interface,
				// and throws on a nil func() ("cannot pass *T to
				// finalizer func()").
				fin := reflect.Zero(reflect.TypeFor[any]())
				if obj.Kind() != reflect.Pointer || obj.IsNil() {
					call(fin)()
					return Value{}
				}
				m.setFinalizer(obj.UnsafePointer(), true, call(fin))
				return Value{}
			}
			fin := m.box(fiv, newConv(nil))
			if obj.Kind() != reflect.Pointer || obj.IsNil() {
				call(fin)() // the runtime throws, as in Go
				return Value{}
			}
			// The runtime gets a native func of the type of obj that calls
			// the finalizer of the plugin (runFinalizer): an interpreted
			// func is an opaque handle in AOT mode (the runtime throws "not
			// a function"), and a panic of the finalizer, also of a native
			// func, must not reach the finalizer goroutine of the runtime.
			if nf, ok := m.nativeFinalizer(iv, fiv, fin, obj.Type(), inst); ok {
				fin = nf
			}
			m.setFinalizer(obj.UnsafePointer(), false, call(fin))
			return Value{}
		}
		key := unsafe.Pointer(cell)
		if fiv == nil {
			m.setFinalizer(key, true, func() { runtime.SetFinalizer(cell, nil) })
			return Value{}
		}
		run := m.vmFinalizer(iv.T, fiv, inst)
		track := m.panicOnFatal
		m.setFinalizer(key, false, func() {
			// The finalizer must not capture the cell, or it would never run.
			runtime.SetFinalizer(cell, func(c *Value) {
				if track {
					finalized.Delete(uintptr(unsafe.Pointer(c)))
				}
				run(Value{R: c})
			})
		})
		return Value{}
	}
}

// finParamIsIface reports whether the parameter of the interpreted
// finalizer fiv is an interface: it gets the pointer in an interface.
func finParamIsIface(fiv *Iface) bool {
	sig, ok := fiv.T.T.Underlying().(*types.Signature)
	if !ok || sig.Params().Len() != 1 {
		return false
	}
	_, isIface := sig.Params().At(0).Type().Underlying().(*types.Interface)
	return isIface
}

// nativeFinalizer returns a native func(ot) that calls the finalizer fiv
// (fin once boxed) with its argument, for a native pointer of type ot (the
// runtime needs a real func). fiv is an interpreted func, or a native func
// (reflect.TypeOf, (*os.File).Close). ok is false when fiv is not a func of
// one parameter that can take an ot (the runtime then throws, as in Go;
// checkFinalizer refused it before with PanicOnFatal). The finalizer runs
// in the instance inst (nil: none, instance.go).
func (m *Machine) nativeFinalizer(iv, fiv *Iface, fin reflect.Value, ot reflect.Type, inst *Instance) (reflect.Value, bool) {
	nf, nat := m.finNative(fiv, fin)
	if nat {
		if !nf.IsValid() {
			return reflect.Value{}, false
		}
		fin = nf
		t := fin.Type()
		if t.IsVariadic() || t.NumIn() != 1 || !ot.AssignableTo(t.In(0)) {
			return reflect.Value{}, false
		}
	} else {
		if fiv.T.T == nil {
			return reflect.Value{}, false
		}
		sig, ok := fiv.T.T.Underlying().(*types.Signature)
		if !ok || sig.Variadic() || sig.Params().Len() != 1 || !m.finAccepts(iv, sig.Params().At(0).Type()) {
			return reflect.Value{}, false
		}
	}
	rt, fn := iv.T, fiv.V
	toIface := !nat && finParamIsIface(fiv)
	track := m.panicOnFatal
	native := iv.T.Native != nil
	ft := reflect.FuncOf([]reflect.Type{ot}, nil, false)
	return reflect.MakeFunc(ft, func(args []reflect.Value) []reflect.Value {
		p := args[0]
		if track {
			finalized.Delete(p.Pointer())
		}
		if nat {
			m.runFinalizer(Value{}, nil, fin, func() reflect.Value { return p }, inst)
			return nil
		}
		var arg Value
		if native {
			arg = Value{R: p}
		} else {
			arg = m.fromNative(p, rt.T)
		}
		if toIface {
			arg = Value{R: &Iface{T: rt, V: arg}}
		}
		m.runFinalizer(fn, []Value{arg}, reflect.Value{}, nil, inst)
		return nil
	}), true
}

// finNative returns the native func of the finalizer fiv, and true, when
// fiv is not an interpreted func (reflect.TypeOf, json.Marshal, a method
// value of a native object): callValue cannot call it. boxed is fiv once
// boxed, or the zero Value. The func is invalid when fiv is not a non-nil
// func.
func (m *Machine) finNative(fiv *Iface, boxed reflect.Value) (reflect.Value, bool) {
	if _, ok := fiv.V.R.(*Closure); ok {
		return reflect.Value{}, false
	}
	fn, ok := fiv.V.R.(reflect.Value)
	if !ok {
		// The func itself: AOT mode boxes it as an opaque handle.
		fn = boxed
		if !fn.IsValid() {
			fn = m.box(fiv, newConv(nil))
		}
	}
	if fn.Kind() != reflect.Func || fn.IsNil() {
		return reflect.Value{}, true
	}
	return fn, true
}

// vmFinalizer returns the call of the finalizer fiv of the plugin with a
// pointer of the VM of type rt (a cell, a *byte): the native func that the
// runtime calls, on its finalizer goroutine, calls it. The finalizer runs
// in the instance inst (nil: none, instance.go).
func (m *Machine) vmFinalizer(rt *RType, fiv *Iface, inst *Instance) func(p Value) {
	if nf, nat := m.finNative(fiv, reflect.Value{}); nat {
		if !nf.IsValid() {
			// Not a func: callValue panics, and GoPanic gets it.
			return func(Value) { m.runFinalizer(fiv.V, nil, reflect.Value{}, nil, inst) }
		}
		return func(p Value) {
			m.runFinalizer(Value{}, nil, nf, func() reflect.Value {
				return m.box(&Iface{T: rt, V: p}, newConv(nil))
			}, inst)
		}
	}
	toIface := finParamIsIface(fiv)
	fn := fiv.V
	return func(p Value) {
		if toIface {
			p = Value{R: &Iface{T: rt, V: p}}
		}
		m.runFinalizer(fn, []Value{p}, reflect.Value{}, nil, inst)
	}
}

// runFinalizer runs a finalizer of the plugin. The finalizer is the
// interpreted func fn with args, or, when nat is valid, the native func
// nat with the argument that natArg gives.
//
// The runtime calls all the finalizers of the process, one after the
// other, on one goroutine where nothing recovers. The call of the plugin
// code does not run there: it goes to the queue of the Machine, and
// runFinalizer returns at once. A finalizer of the plugin that blocks
// (select{}) then stops only the later finalizers of the same plugin, as
// in Go for its own program, and never the finalizers of the host (an
// os.File that is never closed). One goroutine per Machine runs the
// queue, in order, as Go runs finalizers. Each call runs as a goroutine of
// the program (runGoroutine): a panic, ErrBudget or ErrStackOverflow goes
// to GoPanic and does not end the host, and a runtime.Goexit ends only the
// call. The call runs in inst, the instance of the call that set the
// finalizer (nil: none, instance.go): the goroutines that it starts belong
// to inst, and Instance.Close stops it.
func (m *Machine) runFinalizer(fn Value, args []Value, nat reflect.Value, natArg func() reflect.Value, inst *Instance) {
	var d deferred
	switch cl, _ := fn.R.(*Closure); {
	case nat.IsValid():
		d = deferred{nat: func() { nat.Call([]reflect.Value{natArg()}) }}
	case cl != nil && cl.Go == nil && cl.Fn != nil:
		d = deferred{fn: cl.Fn, env: cl.Env, args: args}
	default:
		d = deferred{nat: func() { m.callValue(fn, args) }}
	}
	m.finMu.Lock()
	m.finQ = append(m.finQ, func() { m.runGoroutine(d, m.instGoCtx(inst)) })
	start := !m.finRun
	m.finRun = true
	m.finMu.Unlock()
	if start {
		go m.runFinalizers()
	}
}

// runFinalizers runs the queue of finalizers of the Machine until it is
// empty. A native finalizer can still end the goroutine with a real
// runtime.Goexit: a new goroutine then takes the rest of the queue.
func (m *Machine) runFinalizers() {
	done := false
	defer func() {
		if !done {
			go m.runFinalizers()
		}
	}()
	for {
		m.finMu.Lock()
		if len(m.finQ) == 0 {
			m.finRun = false
			m.finMu.Unlock()
			done = true
			return
		}
		f := m.finQ[0]
		m.finQ[0] = nil
		m.finQ = m.finQ[1:]
		m.finMu.Unlock()
		f()
	}
}

// finAccepts reports whether a finalizer with a parameter of type pt can
// take the pointer of iv, as the runtime checks it.
func (m *Machine) finAccepts(iv *Iface, pt types.Type) bool {
	if iv.T.T != nil {
		return types.AssignableTo(iv.T.T, pt)
	}
	if iv.T.Native == nil {
		return false
	}
	nt := m.reflectType(pt)
	if nt == nil {
		return false
	}
	if nt.Kind() == reflect.Interface {
		return iv.T.Native.Implements(nt)
	}
	return nt == iv.T.Native
}

// setFinalizer runs set, the native call of runtime.SetFinalizer on the
// object at p (clear: with a nil finalizer). With PanicOnFatal, the check
// of finalized ("finalizer already set"), the native call and the update
// of finalized are one step: two goroutines that set a finalizer on the
// same object would both pass a check done before, and the second native
// call would throw. The finalizer that set installs must delete p from
// finalized when it runs.
func (m *Machine) setFinalizer(p unsafe.Pointer, clear bool, set func()) {
	if !m.panicOnFatal {
		set()
		return
	}
	key := uintptr(p)
	finalizeMu.Lock()
	defer finalizeMu.Unlock()
	// The object stays reachable until the update of finalized, so its
	// finalizer cannot run (and delete the entry) before it.
	defer runtime.KeepAlive(p)
	if clear {
		set()
		finalized.Delete(key)
		return
	}
	if _, set := finalized.Load(key); set {
		panic(&FatalError{"runtime.SetFinalizer: finalizer already set"})
	}
	set()
	finalized.Store(key, struct{}{})
}
