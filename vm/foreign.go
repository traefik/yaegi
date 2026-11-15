package vm

// Values of another machine.
//
// Traefik loads each plugin in its own machine and chains them: a plugin
// gets values of the plugin before it (a reader passed to io.Copy, a value
// stored in the request context) as native values that box a value of the
// other machine (a hand wrapper such as stringerW, a generic wrapper of
// wrappers.go, the ResponseWriter wrapper of rwwriter.go, or a vmOpaque).
// Native code cannot build a type with methods at run time, so the box
// only has the methods of a known interface: an io.Reader wrapper only has
// Read. A plugin that asserted r.(interface{ Tag() string }) on such a
// value got false, where Go gives true.
//
// Such a value gets its own RType in the receiving machine, one per type of
// the owner and native box type (foreignRType). Its type assertions see the
// methods of the owner's type, and a call of one of these methods that
// the box does not have runs in the owner machine (callForeign), with the
// arguments and results in native form, as for any call between machines.
// A method whose signature has no native form (it uses a named type of a
// plugin) does not cross: the two machines do not share these types.

import (
	"go/types"
	"reflect"
	"sync"
)

// foreignType is the owner of the values of a foreign RType.
type foreignType struct {
	m  *Machine // the owner machine
	rt *RType   // the type of the values in the owner machine
	// sigs caches, by method name, the native type of the method of rt
	// (a reflect.Type, or nil when it has none).
	sigs sync.Map
}

type foreignKey struct {
	rt  *RType
	nat reflect.Type
}

// foreignObj returns the value of another machine than m that the native
// value e boxes.
func (m *Machine) foreignObj(e reflect.Value) (vmObj, bool) {
	b, ok := boxedOf(e)
	if !ok {
		if e.Kind() != reflect.Struct || !isWrapperType(e.Type()) {
			return vmObj{}, false
		}
		if b, ok = e.Field(0).Interface().(vmBoxed); !ok {
			return vmObj{}, false
		}
	}
	o := b.obj()
	if o.m == nil || o.m == m || o.iv == nil || o.iv.T == nil || o.iv.T.T == nil || len(o.iv.T.Methods) == 0 {
		return vmObj{}, false
	}
	return o, true
}

// nativeRTypeOf is the RType of the native value e in an interface: the
// foreign RType of a value of another machine with methods, nativeRType
// otherwise.
func (m *Machine) nativeRTypeOf(e reflect.Value) *RType {
	if o, ok := m.foreignObj(e); ok {
		return m.foreignRType(o, e.Type())
	}
	return m.nativeRType(e.Type())
}

// foreignRType returns the RType in m of the values of type o.iv.T of
// another machine boxed in the native type nat.
func (m *Machine) foreignRType(o vmObj, nat reflect.Type) *RType {
	k := foreignKey{o.iv.T, nat}
	if rt, ok := m.foreign.Load(k); ok {
		return rt.(*RType)
	}
	rt, _ := m.foreign.LoadOrStore(k, &RType{Native: nat, fo: &foreignType{m: o.m, rt: o.iv.T}})
	return rt.(*RType)
}

// methodSig returns the native type of the method name of the owner type,
// without the receiver, or nil. It is nil too when the signature uses a
// named type of the owner's program: its native form (a struct for
// type T struct{ N int }) would match a signature of another machine
// that uses a different type.
func (f *foreignType) methodSig(name string) reflect.Type {
	if t, ok := f.sigs.Load(name); ok {
		t, _ := t.(reflect.Type)
		return t
	}
	var t reflect.Type
	if fn := f.rt.Methods[name]; fn != nil && !f.m.mentionsInterpNamed(fn.ssa.Signature) {
		t = f.m.methodType(f.rt, name)
	}
	f.sigs.Store(name, t)
	return t
}

// foreignHas reports whether the owner type of the foreign RType rt has
// the method fn of an interface of m, with the same native signature.
// A signature that uses a named type of a plugin, on either side, never
// matches: two machines never share these types, as two packages of Go
// never share a named type. Before, only the native signatures were
// compared: B's interface{ Get() T } matched A's Get() T when the two T
// had the same fields.
func (m *Machine) foreignHas(rt *RType, fn *types.Func) bool {
	sig := fn.Type().(*types.Signature)
	if m.mentionsInterpNamed(sig) {
		return false
	}
	ot := rt.fo.methodSig(fn.Name())
	if ot == nil {
		return false
	}
	mt := m.tryReflectType(types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic()))
	return mt == ot
}

// callForeign calls the method name of the value of another machine that
// the native value rv boxes, when the box does not have it. ok is false
// when rv boxes no such value or its type has no such method.
func (m *Machine) callForeign(rv reflect.Value, name string, sig *types.Signature, regs []Value, args []int32) (Value, bool) {
	o, ok := m.foreignObj(rv)
	if !ok || o.iv.T.Methods[name] == nil {
		return Value{}, false
	}
	ps := tupleTypes(sig.Params())
	in := make([]reflect.Value, len(args))
	var wb []func()
	c := newConv(&wb)
	for i, a := range args {
		in[i] = m.toNativeW(regs[a], ps[i], c)
	}
	defer writeBack(wb)
	defer m.flushShadows()
	return m.pack(o.callRes(name, in, o.m.foreignResult), tupleTypes(sig.Results())), true
}

// foreignResult converts v, a result of type t of a method of m that
// another machine called (callForeign). A value of the program with
// methods in an interface result is a box of m, as the values of the
// context are (nativeVal): the caller sees the methods of its type
// (foreignRType), and two results that hold the same value are equal
// (equalForeign). Before, resultToNative gave a new native copy or
// wrapper at each call: Get() any, which returned the same *User each
// time, gave results that were never equal, and a method of the type
// that the wrapper did not have was lost. The box is used only when it
// fits the native result type: for a non-empty interface such as
// io.Reader that the box does not implement, the result is converted as
// before (resultToNative), or reflect.Set panics in the caller.
func (m *Machine) foreignResult(v Value, t types.Type) reflect.Value {
	if iv, _ := v.R.(*Iface); iv != nil && types.IsInterface(t) && iv.T.T != nil && len(iv.T.Methods) > 0 {
		p, ok := types.Unalias(iv.T.T).(*types.Pointer)
		if !m.isNativeNamed(iv.T.T) && !(ok && m.isNativeNamed(p.Elem())) {
			o := vmObj{m: m, iv: iv}
			bv, ok := m.handWrapper(o)
			if !ok {
				bv = reflect.ValueOf(vmOpaque{o})
			}
			if rt := m.tryReflectType(t); rt != nil && rt.Kind() == reflect.Interface && bv.Type().Implements(rt) {
				return bv
			}
		}
	}
	return m.resultToNative(v, t)
}

// equalForeign compares x and y, two values of the same foreign RType, in
// their machine: as in Go, two interfaces that hold the same pointer are
// equal, also when each one came in its own box.
func (m *Machine) equalForeign(x, y *Iface) bool {
	a, aok := m.foreignObj(x.V.R.(reflect.Value))
	b, bok := m.foreignObj(y.V.R.(reflect.Value))
	if !aok || !bok || a.m != b.m {
		return x.V.R.(reflect.Value).Interface() == y.V.R.(reflect.Value).Interface()
	}
	return a.m.equal(Value{R: a.iv}, Value{R: b.iv}, types.Universe.Lookup("any").Type())
}
