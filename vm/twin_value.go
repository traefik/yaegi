package vm

// Go-level twin of package reflect over VM values: reflect.Value.
//
// An RV is either
//   - VM-backed: t is the VM storage type of the cell p (VM representation);
//     when the cell holds native data (a native struct, pointer, map... as a
//     reflect.Value), operations go to that reflect.Value and writes that
//     reflect cannot perform in place are written back into the cell;
//   - native-backed: p == nil and n is a reflect.Value, t its native type.

import (
	"go/types"
	"math"
	"reflect"
	"slices"
	"strconv"
	"sync/atomic"
	"unsafe"
)

// RV is a value of the twin (reflect.Value).
type RV struct {
	t    *vtype
	p    *Value
	n    reflect.Value
	flag int64
	// b is the byte of a slice kept as Go bytes that the cell p copies
	// (Index of a []B, B a byte type with methods): a write goes to both.
	b *byte
}

func natRV(n reflect.Value) RV {
	if !n.IsValid() {
		return RV{}
	}
	return RV{t: natT(n.Type()), n: n}
}

func mkVM(t *vtype, p *Value, flag int64) RV { return RV{t: t, p: p, flag: flag} }

// native returns the native data of v, if any.
func (v RV) native() (reflect.Value, bool) {
	if v.p == nil {
		return v.n, v.n.IsValid()
	}
	// A cell with a native shadow (Interface of a pointer to it: yaml.v3
	// asks every field for an Unmarshaler with v.Addr().Interface()) is
	// read and written through the shadow, also for an interface: the
	// shadow goes back into the cell at the end of the call, and a value
	// written into the cell itself was lost (an `any` field stayed nil).
	if s := ownShadow(v.p); s != nil {
		return s.rv, true // before the cell: it may hold a native slice view
	}
	if r, ok := v.p.R.(*shadowRef); ok {
		return r.rv, true
	}
	if v.t.kind == reflect.Interface {
		return reflect.Value{}, false
	}
	switch r := v.p.R.(type) {
	case reflect.Value:
		return r, true
	case *byte:
		if v.t.t != nil {
			if p, ok := v.t.t.Underlying().(*types.Pointer); ok && hasMethods(p.Elem()) {
				// A pointer into a []U of a byte type U of the program with
				// methods: a native *uint8 has none, and json skipped the
				// UnmarshalJSON of *U. The VM keeps it
				// (Elem: byteElem, whose writes reach the byte).
				return reflect.Value{}, false
			}
		}
		return reflect.ValueOf(r), true
	}
	return reflect.Value{}, false
}

func (v RV) mach() *Machine { return v.t.vr.m }

func (v RV) mustValid(op string) {
	if v.t == nil {
		panic(&reflect.ValueError{Method: "reflect.Value." + op, Kind: reflect.Invalid})
	}
}

// boxedOf returns the VM value carried by a native value, if any.
func boxedOf(e reflect.Value) (vmBoxed, bool) {
	if !e.IsValid() || !e.CanInterface() {
		return nil, false
	}
	switch e.Kind() {
	case reflect.Struct, reflect.Interface:
	default:
		return nil, false
	}
	b, ok := e.Interface().(vmBoxed)
	return b, ok
}

func fromBoxed(b vmBoxed, flag int64) RV {
	o := b.obj()
	if o.seg > 0 && flag == 0 {
		// A value nested in interfaces deeper than maxBoxDepth: its native
		// copy, made once, as for the levels above the handle. Read through
		// its VM cells, AOT fmt converted the rest again at each level
		// (Interface, for handleMethods): quadratic.
		if nv := o.m.box(o.iv, o.segConv()); nv.IsValid() {
			if _, ok := boxedOf(nv); !ok {
				return natRV(nv)
			}
		}
	}
	return mkVM(o.m.vr.vtypeOf(o.iv.T.T), ifaceCell(o.iv.V), flag)
}

// ---- conversions between the two representations ----

// natVal returns x as a native value of type rt (x must be assignable to rt).
func (x RV) natVal(rt reflect.Type) reflect.Value {
	if x.t == nil {
		return reflect.Zero(rt)
	}
	if n, ok := x.native(); ok {
		return n
	}
	m := x.mach()
	val := *x.p
	if rt.Kind() == reflect.Interface && x.t.kind != reflect.Interface {
		val = Value{R: &Iface{T: m.rtypeOf(x.t.t), V: deepCopy(val, x.t.t)}}
	}
	return m.argNat(val, rt)
}

// natStored is natVal for a value that native memory keeps: a native
// variable (Set) or a native map (SetMapIndex, and MapIndex to find it
// again). An interpreted channel there, also in an element of a map, a
// slice, an array or a struct, gets its stand-in (chanstandin.go), as a
// channel in an any does: native code may only look at it or give it back.
// json v2 (Go 1.27) copies the value of json.Marshal(v) into a new variable
// of its type (reflect.New(t).Elem().Set(v)) before it looks at it: there
// natVal refused a channel, and the AOT copy panicked where go run returns
// "json: unsupported type: chan int".
//
// An interpreted value stored in a native variable of an interface type is
// boxed as for Interface (ifaceAny): json v2 copies the value of each map
// entry into such a variable (newAddressableValue(t.Elem()), then
// SetIterValue), and a native copy there lost the methods of the value
// (a Color with a MarshalText method in a map[string]any was encoded as a
// struct in default mode).
func (x RV) natStored(rt reflect.Type) reflect.Value {
	if x.t == nil {
		return x.natVal(rt)
	}
	if _, ok := x.native(); ok {
		return x.natVal(rt)
	}
	m := x.mach()
	if rt.Kind() == reflect.Interface {
		iv, _ := x.p.R.(*Iface)
		if x.t.kind != reflect.Interface {
			iv = &Iface{T: m.rtypeOf(x.t.t), V: deepCopy(*x.p, x.t.t)}
		}
		if iv != nil && iv.T.Native == nil {
			if b := reflect.ValueOf(m.ifaceAny(iv)); b.IsValid() && b.Type().Implements(rt) {
				return b
			}
		}
		return x.natVal(rt)
	}
	if ch, ok := x.p.R.(chan Value); ok && rt.Kind() == reflect.Chan && x.t.t != nil && !inChanReader() {
		return m.standIn(ch, x.t.t, rt) // keeps the type of x
	}
	return m.argNatAt(*x.p, rt, true)
}

// vmVal returns x in the VM representation of type t.
func (x RV) vmVal(m *Machine, t types.Type) Value {
	if x.t == nil {
		return m.zero(t)
	}
	if n, ok := x.native(); ok {
		return m.natToVM(n, t)
	}
	val := *x.p
	if _, isI := t.Underlying().(*types.Interface); isI && x.t.kind != reflect.Interface {
		return Value{R: &Iface{T: m.rtypeOf(x.t.t), V: deepCopy(val, x.t.t)}}
	}
	return deepCopy(val, t)
}

// argNat converts a VM value to a native value of type rt (reflect-driven:
// used where only the native type is known, e.g. AOT hooks).
func (m *Machine) argNat(v Value, rt reflect.Type) reflect.Value {
	return m.argNatAt(v, rt, false)
}

// argNatAt is argNat. stored is true when native memory keeps the result
// (natStored): an interpreted channel in it, also in an element of a map,
// a slice, an array or a struct, gets its stand-in (chanstandin.go).
// Otherwise a channel is refused: native code would use it.
func (m *Machine) argNatAt(v Value, rt reflect.Type, stored bool) reflect.Value {
	if r, ok := m.twinFromVM(v, rt); ok {
		return r
	}
	if r, ok := v.R.(reflect.Value); ok {
		switch {
		case r.Type() == rt:
			return r
		case rt.Kind() == reflect.Interface:
			w := reflect.New(rt).Elem()
			w.Set(r)
			return w
		case r.Type().ConvertibleTo(rt):
			return r.Convert(rt)
		}
		return r
	}
	switch rt.Kind() {
	case reflect.Bool:
		out := reflect.New(rt).Elem()
		out.SetBool(v.N != 0)
		return out
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		out := reflect.New(rt).Elem()
		out.SetInt(v.N)
		return out
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		out := reflect.New(rt).Elem()
		out.SetUint(uint64(v.N))
		return out
	case reflect.Float32, reflect.Float64:
		out := reflect.New(rt).Elem()
		out.SetFloat(f64(v))
		return out
	case reflect.Complex64, reflect.Complex128:
		out := reflect.New(rt).Elem()
		out.SetComplex(cplx(v)) // Value{R: complex128}, complex.go
		return out
	case reflect.String:
		out := reflect.New(rt).Elem()
		s, _ := v.R.(string)
		out.SetString(s)
		return out
	case reflect.UnsafePointer:
		return reflect.ValueOf(unsafe.Pointer(nil))
	case reflect.Chan:
		if v.R == nil {
			return reflect.Zero(rt)
		}
		if ch, ok := v.R.(chan Value); ok && stored && !inChanReader() {
			if t := m.goTypeOf(rt); t != nil {
				return m.standIn(ch, t, rt)
			}
		}
	case reflect.Interface:
		out := reflect.New(rt).Elem()
		iv, _ := v.R.(*Iface)
		if iv == nil {
			return out
		}
		var nv reflect.Value
		if iv.T.Native != nil {
			nv = iv.V.R.(reflect.Value)
		} else {
			nv = m.boxFor(iv, rt, newConv(nil))
		}
		if nv.IsValid() {
			out.Set(nv)
		}
		return out
	case reflect.Slice:
		switch s := v.R.(type) {
		case nil:
			return reflect.Zero(rt)
		case []byte:
			rv := reflect.ValueOf(s)
			if rv.Type() != rt {
				rv = rv.Convert(rt)
			}
			return rv
		case []Value:
			out := reflect.MakeSlice(rt, len(s), len(s))
			for i := range s {
				out.Index(i).Set(m.argNatAt(s[i], rt.Elem(), stored))
			}
			return out
		}
	case reflect.Array:
		out := reflect.New(rt).Elem()
		es, _ := v.R.([]Value)
		for i := range es {
			out.Index(i).Set(m.argNatAt(es[i], rt.Elem(), stored))
		}
		return out
	case reflect.Map:
		mp, _ := v.R.(*MapV)
		if mp == nil {
			return reflect.Zero(rt)
		}
		out := reflect.MakeMapWithSize(rt, len(mp.M))
		mp.each(func(k, e Value) {
			out.SetMapIndex(m.argNatAt(k, rt.Key(), stored), m.argNatAt(e, rt.Elem(), stored))
		})
		return out
	case reflect.Pointer:
		switch p := v.R.(type) {
		case nil:
			return reflect.Zero(rt)
		case *byte:
			return reflect.ValueOf(p)
		case *Value:
			if s := ownShadow(p); s != nil {
				return s.rv.Addr()
			}
			switch r := p.R.(type) {
			case reflect.Value:
				if r.CanAddr() && r.Type() == rt.Elem() {
					return r.Addr()
				}
			case *shadowRef:
				return r.rv.Addr()
			}
			if gt := m.goTypeOf(rt.Elem()); gt != nil {
				if n, ok := types.Unalias(gt).(*types.Named); ok && m.isNativeNamed(n) {
					return m.shadow(p, gt).Addr() // aliases the cell until the next VM transition
				}
			}
			sh := reflect.New(rt.Elem()) // pointer into VM memory: copy (no write-back)
			sh.Elem().Set(m.argNatAt(*p, rt.Elem(), true))
			return sh
		}
	case reflect.Func:
		cl, _ := v.R.(*Closure)
		if cl == nil {
			return reflect.Zero(rt)
		}
		if f, ok := m.valueCallback(v, rt); ok { // callbacks.go
			return f
		}
		return reflect.MakeFunc(rt, func(in []reflect.Value) []reflect.Value {
			args := make([]Value, len(in))
			for i := range in {
				args[i] = m.retVM(in[i])
			}
			res := m.callValue(v, args)
			switch rt.NumOut() {
			case 0:
				return nil
			case 1:
				return []reflect.Value{m.argNat(res, rt.Out(0))}
			}
			rs := res.R.([]Value)
			out := make([]reflect.Value, len(rs))
			for i := range rs {
				out[i] = m.argNat(rs[i], rt.Out(i))
			}
			return out
		})
	case reflect.Struct:
		out := reflect.New(rt).Elem()
		fs, _ := v.R.([]Value)
		for i := range fs {
			f := out.Field(i)
			if !f.CanSet() {
				f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
			}
			f.Set(m.argNatAt(fs[i], f.Type(), stored))
		}
		return out
	}
	unsupported("vm: converting %T to native %s", v.R, rt)
	return reflect.Value{}
}

// boxFor converts an interpreted interface value for a native interface type
// rt: a generated wrapper (RegisterWrapper) when the generic boxing does not
// implement rt.
func (m *Machine) boxFor(iv *Iface, rt reflect.Type, c *natConv) reflect.Value {
	nv := m.box(iv, c)
	if rt.NumMethod() > 0 && nv.IsValid() && !nv.Type().Implements(rt) {
		if w := wrapperFor(rt); w != nil {
			return reflect.ValueOf(w(vmObj{m: m, iv: iv}))
		}
	}
	return nv
}

// retVM converts a native value to the VM representation (reflect-driven).
func (m *Machine) retVM(rv reflect.Value) Value {
	return m.retVMs(rv, nil)
}

// retVMs is retVM with the count of the Value entries allocated so far (nil
// until a slice or an array is met, see natSeen.grow).
func (m *Machine) retVMs(rv reflect.Value, seen *natSeen) Value {
	if k := rv.Kind(); k == reflect.Pointer || k == reflect.Struct {
		if v, ok := m.twinToVM(rv); ok {
			return v
		}
	}
	switch rv.Kind() {
	case reflect.Invalid:
		return Value{}
	case reflect.Bool:
		return Value{N: b2i(rv.Bool())}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Value{N: rv.Int()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return Value{N: int64(rv.Uint())}
	case reflect.Float32, reflect.Float64:
		return Value{N: fv(rv.Float())}
	case reflect.String:
		return Value{R: rv.String()}
	case reflect.Interface:
		if rv.IsNil() {
			return Value{}
		}
		if seen == nil {
			seen = &natSeen{}
		}
		return m.ifaceVMs(rv.Elem(), seen)
	case reflect.Slice:
		if rv.IsNil() {
			return Value{}
		}
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return BytesVal(rv.Bytes())
		}
		if seen == nil {
			seen = &natSeen{}
		}
		seen.grow(rv.Len())
		out := make([]Value, rv.Len())
		if len(out) > 0 {
			if o, ok := seen.enter(compKey{rv.UnsafePointer(), rv.Len(), rv.Type()}, nil, Value{R: out}); !ok {
				return o
			}
		}
		for i := range out {
			out[i] = m.retVMs(rv.Index(i), seen)
		}
		return Value{R: out}
	case reflect.Array:
		if seen == nil {
			seen = &natSeen{}
		}
		seen.grow(rv.Len())
		out := make([]Value, rv.Len())
		for i := range out {
			out[i] = m.retVMs(rv.Index(i), seen)
		}
		return Value{R: out}
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		if rv.IsNil() {
			return Value{}
		}
		if s := m.standInOf(rv); s != nil {
			return Value{R: s.ch} // the stand-in of an interpreted channel
		}
		if cell := m.shadowCell(rv); cell != nil {
			return Value{R: cell}
		}
	}
	return Value{R: rv}
}

// shadowReaching returns the shadow of the current goroutine (or an orphan) of a
// cell that the native pointer p reaches: inside is true when p points into
// the memory of the shadow, false when the shadow is a pointer that holds p
// (AOT code allocated the value for a nil pointer of the cell).
func (m *Machine) shadowReaching(p reflect.Value) (s *shadowRef, inside bool) {
	if ownedShadows.Load() == 0 && m.nOrphans.Load() == 0 {
		return nil, false
	}
	addr, size := p.Pointer(), p.Type().Elem().Size()
	find := func(ss []*shadowRef) (*shadowRef, bool) {
		for _, s := range ss {
			if s.cell == nil || !s.rv.CanAddr() {
				continue
			}
			if s.rv.Kind() == reflect.Pointer && !s.rv.IsNil() && s.rv.Pointer() == addr && s.rv.Type() == p.Type() {
				return s, false
			}
			base := s.rv.Addr().Pointer()
			if addr >= base && addr+size <= base+s.rv.Type().Size() {
				return s, true
			}
		}
		return nil, false
	}
	if gs := lookupG(getg()); gs != nil {
		if s, inside := find(gs.shadows); s != nil {
			return s, inside
		}
	}
	if m.nOrphans.Load() > 0 {
		m.shadowMu.Lock()
		defer m.shadowMu.Unlock()
		return find(m.shadows)
	}
	return nil, false
}

// shadowCell returns the VM cell whose shadow p points to, or nil. AOT
// code that got a pointer to a VM cell has a native pointer to its shadow
// (RV.Interface). When AOT code gives it back to interpreted code (a
// function of the program that the copy calls, as MarshalFunc does), the
// program must get the pointer to the cell: during the call the cell holds
// the value (suspendShadows), and a write through the shadow was lost at
// resumeShadows.
func (m *Machine) shadowCell(p reflect.Value) *Value {
	cell, _ := m.shadowCellT(p)
	return cell
}

// shadowCellT is shadowCell, with the VM type of the cell when p is the
// address of a whole shadow (nil for a pointer inside the shadowed value).
func (m *Machine) shadowCellT(p reflect.Value) (*Value, types.Type) {
	if p.Kind() != reflect.Pointer || ownedShadows.Load() == 0 && m.nOrphans.Load() == 0 {
		return nil, nil
	}
	match := func(ss []*shadowRef) (*Value, types.Type) {
		for _, s := range ss {
			if s.cell == nil {
				continue
			}
			if s.rv.CanAddr() && s.rv.Type() == p.Type().Elem() && s.rv.Addr().Pointer() == p.Pointer() {
				return s.cell, s.t
			}
			// a pointer inside the shadowed value (toNativeW)
			if s.c != nil && s.c.g.cells != nil {
				if cell, ok := s.c.g.cells[p.Pointer()]; ok && s.c.g.shadows[cell].Type() == p.Type() {
					return cell, nil
				}
			}
		}
		return nil, nil
	}
	if gs := lookupG(getg()); gs != nil {
		if c, t := match(gs.shadows); c != nil {
			return c, t
		}
	}
	if m.nOrphans.Load() > 0 {
		m.shadowMu.Lock()
		defer m.shadowMu.Unlock()
		return match(m.shadows)
	}
	return nil, nil
}

// ifaceVM converts the dynamic value e of a native interface to a VM
// interface value.
func (m *Machine) ifaceVM(e reflect.Value) Value {
	return m.ifaceVMs(e, &natSeen{})
}

func (m *Machine) ifaceVMs(e reflect.Value, seen *natSeen) Value {
	if e.Type() == defaultMuxType {
		// The machine's DefaultServeMux that stands for a nil Handler of a
		// server (fillHandler, lateNilHandler): the plugin reads nil, as in
		// Go.
		return Value{}
	}
	e = m.guardHandler(e) // a handler that panics again (nativego.go)
	// A value of another machine (Traefik chains plugins, one machine
	// each) stays a native value: its type is not a type of m.
	if b, ok := boxedOf(e); ok && m.owns(b) {
		return Value{R: b.obj().iv}
	}
	if iv, ok := m.standInIface(e); ok {
		return Value{R: iv}
	}
	if iv := m.valueShadowOf(e); iv != nil {
		return Value{R: iv} // a plugin value given to native code in an interface
	}
	if e.Kind() == reflect.Pointer && !e.IsNil() {
		// The address of a shadowed cell in an interface: AOT code gives
		// the target of errors.As back to the program's As(any) method.
		// The program must get the cell, as retVMs gives it for a typed
		// pointer: a write through the shadow is lost at resumeShadows.
		if cell, t := m.shadowCellT(e); cell != nil && t != nil {
			return Value{R: &Iface{T: m.rtypeOf(types.NewPointer(t)), V: Value{R: cell}}}
		}
	}
	if rt := m.gobType(e); rt != nil {
		// the mirror of a type registered with gob.Register (gob.go)
		c := reflect.New(e.Type()).Elem()
		c.Set(e)
		return Value{R: &Iface{T: rt, V: m.fromNativeDeep(c, rt.T)}}
	}
	if v, ok := m.twinIfaceVM(e); ok {
		return v
	}
	if nt := m.routedValType(e.Type()); nt != nil {
		e = e.Convert(nt) // json.Number of the AOT copy of json (aoterrors.go)
	}
	if v, ok := m.valueErrorVM(e, seen); ok {
		return v
	}
	if bt, ok := kindToBasic[e.Kind()]; ok && e.Type().PkgPath() == "" {
		return Value{R: &Iface{T: m.rtypeOf(bt), V: m.retVM(e)}}
	}
	if e.Type().Name() == "" {
		// Unnamed slices and arrays become VM values (the VM cannot slice or
		// append native slices); maps stay native: the VM supports them
		// (lookup, update, range, len), and their aliasing is preserved.
		// Unnamed structs too: with the native type, a type assertion of
		// the program (v.(struct{ X int })) failed on the value of a
		// struct literal that sync.Pool's New had returned.
		switch e.Kind() {
		case reflect.Slice, reflect.Array, reflect.Struct:
			if gt := m.goTypeOf(e.Type()); gt != nil {
				return Value{R: &Iface{T: m.rtypeOf(gt), V: m.natToVMs(e, gt, seen)}}
			}
		}
	}
	return Value{R: &Iface{T: m.nativeRType(e.Type()), V: Value{R: e}}}
}

var valueErrorPtr = reflect.TypeFor[*reflect.ValueError]()

// valueErrorVM converts a native *reflect.ValueError (the panic of the VM
// reflect, twin_value.go) to the program's own *reflect.ValueError. With the
// VM reflect, the program's reflect types have the VM layout and no native
// counterpart, so r.(*reflect.ValueError) on the recovered value was false
// where gc gives true.
func (m *Machine) valueErrorVM(e reflect.Value, seen *natSeen) (Value, bool) {
	if !m.progVR || e.Type() != valueErrorPtr || e.IsNil() {
		return Value{}, false
	}
	vt := m.reflectNamed("ValueError")
	if vt == nil {
		return Value{}, false
	}
	pt := types.NewPointer(vt)
	return Value{R: &Iface{T: m.rtypeOf(pt), V: m.natToVMs(e, pt, seen)}}, true
}

// natToVM converts native data n to the VM representation of type t
// (go/types-driven; interface-typed parts carry the VM values boxed by AOT
// code back to the VM).
func (m *Machine) natToVM(n reflect.Value, t types.Type) Value {
	return m.natToVMs(n, t, &natSeen{})
}

// natSeen holds the native slices and maps that a conversion into the VM
// (natToVMs, retVMs, fromNativeS, fromNativeDeepC) has met, with their VM
// value. A native value can contain itself (s[0] = s in a []any that comes
// back from sync.Pool, or a type M map[string]M with m["self"] = m): when
// the conversion meets such a value again, it gives back the VM value being
// built, so the VM value has the same cycle. Without it, the conversion
// never ended and the Go stack grew until gc killed the whole host. A value
// reached again after its conversion (a DAG: the same map or slice reached
// by several paths) also gives back the same VM value: the conversion is
// linear in the number of distinct maps and slices, not exponential in the
// depth, and the VM value shares what the native value shares, as in Go.
type natSeen struct {
	conv map[convKey]Value
	// entries counts the Value entries that the conversion allocated so
	// far, bounded by maxZeroElems (see grow).
	entries int64
}

// convKey names a native slice or map converted to the VM type t (nil for
// the reflect-driven retVMs): two VM types can have the same native type
// and not the same VM representation (type T []T and []any).
type convKey struct {
	compKey
	t types.Type
}

// enter starts the conversion of the native slice or map k to v, of VM type
// t. When k was met before (in the middle of its conversion, or done), it
// returns the VM value of that conversion and ok is false.
func (s *natSeen) enter(k compKey, t types.Type, v Value) (Value, bool) {
	key := convKey{k, t}
	if o, ok := s.conv[key]; ok {
		return o, false
	}
	if s.conv == nil {
		s.conv = map[convKey]Value{}
	}
	s.conv[key] = v
	return v, true
}

func (m *Machine) natToVMs(n reflect.Value, t types.Type, seen *natSeen) Value {
	n, ok := unwrapCut(n, t)
	if !ok {
		return deepCopy(m.zero(t), t)
	}
	if m.twinNamed(t) != nil {
		if v, ok := m.twinToVM(n); ok {
			return v
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Interface:
		if !n.IsValid() {
			return Value{}
		}
		if n.Kind() == reflect.Interface {
			if n.IsNil() {
				return Value{}
			}
			n = n.Elem()
		}
		return m.ifaceVMs(n, seen)
	case *types.Basic:
		return m.retVM(n)
	case *types.Struct:
		if m.isNativeNamed(t) {
			c := reflect.New(n.Type()).Elem()
			c.Set(n)
			return Value{R: c}
		}
		seen.grow(u.NumFields())
		fs := make([]Value, u.NumFields())
		for i := range fs {
			f := n.Field(i)
			if !f.CanInterface() && f.CanAddr() {
				f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
			}
			fs[i] = m.natToVMs(f, u.Field(i).Type(), seen)
		}
		return Value{R: fs}
	case *types.Slice:
		if n.IsNil() {
			return Value{}
		}
		if n.Type().Elem().Kind() == reflect.Uint8 {
			return BytesVal(n.Bytes())
		}
		seen.grow(n.Len())
		out := make([]Value, n.Len())
		if len(out) > 0 {
			if o, ok := seen.enter(compKey{n.UnsafePointer(), n.Len(), n.Type()}, t, Value{R: out}); !ok {
				return o
			}
		}
		for i := range out {
			out[i] = m.natToVMs(n.Index(i), u.Elem(), seen)
		}
		return Value{R: out}
	case *types.Array:
		seen.grow(n.Len())
		out := make([]Value, n.Len())
		for i := range out {
			out[i] = m.natToVMs(n.Index(i), u.Elem(), seen)
		}
		return Value{R: out}
	case *types.Map:
		if n.IsNil() {
			return Value{}
		}
		if m.isNativeNamed(t) {
			return Value{R: n}
		}
		seen.grow(2 * n.Len()) // a key and a value per entry
		mp := newMapV(0)
		if o, ok := seen.enter(compKey{n.UnsafePointer(), 0, n.Type()}, t, Value{R: mp}); !ok {
			return o
		}
		it := n.MapRange()
		for it.Next() {
			k := m.natToVMs(it.Key(), u.Key(), seen)
			kk := storeKey(keyOf(k, u.Key()))
			mp.M[kk] = m.natToVMs(it.Value(), u.Elem(), seen)
			mp.setKey(kk, k)
		}
		return Value{R: mp}
	}
	return m.retVM(n)
}

// ---- reflect.Value methods ----

func (v RV) IsValid() bool { return v.t != nil }

func (v RV) Kind() reflect.Kind {
	if v.t == nil {
		return reflect.Invalid
	}
	return v.t.kind
}

func (v RV) Type() RT {
	if v.t == nil {
		panic(&reflect.ValueError{Method: "reflect.Value.Type", Kind: reflect.Invalid})
	}
	if v.p == nil {
		return v.t
	}
	return v.t.cn()
}

func (v RV) Elem() RV {
	v.mustValid("Elem")
	if n, ok := v.native(); ok {
		switch n.Kind() {
		case reflect.Interface:
			if n.IsNil() {
				return RV{}
			}
			e := n.Elem()
			if b, ok := boxedOf(e); ok {
				return fromBoxed(b, v.flag&flagRO)
			}
			return natRV(copyRV(e))
		case reflect.Pointer:
			return natRV(n.Elem())
		}
		panic(&reflect.ValueError{Method: "reflect.Value.Elem", Kind: n.Kind()})
	}
	// An AOT copy that walks a value recurses at each Elem: stop it before
	// the stack limit, which ends the host (stackguard.go).
	checkStack()
	switch v.t.kind {
	case reflect.Pointer:
		switch p := v.p.R.(type) {
		case nil:
			return RV{}
		case *Value:
			if p == nil {
				return RV{}
			}
			return mkVM(v.t.elemT(), p, flagAddr|v.flag&flagRO)
		case *byte:
			return byteElem(v.t.elemT(), p, v.flag&flagRO)
		case roByte:
			// A copy of the byte, not addressable: reflect must not give
			// a settable value of read-only memory.
			return mkVM(v.t.elemT(), &Value{N: int64(*p)}, flagROMem)
		case *byteView:
			// Plain data over bytes (byteview.go): a copy, not addressable.
			c := p.load(v.t.elemT().t)
			return mkVM(v.t.elemT(), &c, flagROMem)
		}
	case reflect.Interface:
		iv, _ := v.p.R.(*Iface)
		if iv == nil {
			return RV{}
		}
		if iv.T.Native != nil {
			return natRV(copyRV(unaddrNat(iv.V.R.(reflect.Value))))
		}
		if c := v.t.vr.vtypeOf(iv.T.T).cn(); c.isNat() && isRouteValType(c.nat) {
			// json.Number("2") of the program: the Number of the copy
			return natRV(copyRV(v.mach().argNat(iv.V, c.nat)))
		}
		return mkVM(v.t.vr.vtypeOf(iv.T.T), ifaceCell(iv.V), v.flag&flagRO)
	}
	panic(&reflect.ValueError{Method: "reflect.Value.Elem", Kind: v.t.kind})
}

func (v RV) Field(i int) RV {
	v.mustValid("Field")
	if n, ok := v.native(); ok {
		return natRV(n.Field(i))
	}
	st := v.t.structT()
	fs := v.p.R.([]Value)
	return mkVM(v.t.fieldT(i), &fs[i], fieldFlag(v.flag, st.Field(i)))
}

func (v RV) FieldByIndex(index []int) RV {
	if len(index) == 1 {
		return v.Field(index[0])
	}
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer && v.Type().Elem().Kind() == reflect.Struct {
			if v.IsNil() {
				panic("reflect: indirection through nil pointer to embedded struct")
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v
}

func (v RV) FieldByIndexErr(index []int) (RV, error) {
	if len(index) == 1 {
		return v.Field(index[0]), nil
	}
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer && v.Type().Elem().Kind() == reflect.Struct {
			if v.IsNil() {
				return RV{}, errorString("reflect: indirection through nil pointer to embedded struct field " + v.Type().Elem().Name())
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, nil
}

type errorString string

func (e errorString) Error() string { return string(e) }

func (v RV) FieldByName(name string) RV {
	if f, ok := v.Type().FieldByName(name); ok {
		return v.FieldByIndex(f.Index)
	}
	return RV{}
}

func (v RV) FieldByNameFunc(match func(string) bool) RV {
	if f, ok := v.Type().FieldByNameFunc(match); ok {
		return v.FieldByIndex(f.Index)
	}
	return RV{}
}

func (v RV) NumField() int {
	if n, ok := v.native(); ok {
		return n.NumField()
	}
	return v.t.structT().NumFields()
}

func (v RV) Index(i int) RV {
	v.mustValid("Index")
	if n, ok := v.native(); ok {
		return natRV(n.Index(i))
	}
	switch v.t.kind {
	case reflect.Slice:
		switch s := v.p.R.(type) {
		case []Value:
			if uint(i) >= uint(len(s)) {
				panic("reflect: slice index out of range")
			}
			return mkVM(v.t.elemT(), &s[i], flagAddr|v.flag&flagRO)
		case []byte:
			if uint(i) >= uint(len(s)) {
				panic("reflect: slice index out of range")
			}
			if et := v.t.elemT(); et.t != nil && (hasMethods(et.t) || v.mach().mentionsInterpNamed(et.t)) {
				// A byte type of the program with methods (type B byte
				// with a Format method) in a slice kept as Go bytes: the
				// element is a VM value of that type, so that fmt calls
				// Format (TestSprintf of package fmt). A native uint8 lost
				// the methods. The cell is a copy, and b keeps the byte
				// so that a write reaches the slice (binary.Read).
				// Methods with a pointer receiver are found on the
				// address of the byte too, as in native code.
				// A byte type of the program without methods keeps its
				// name too: fmt prints %!t(main.U8=104), not uint8
				//.
				return byteElem(et, &s[i], v.flag&flagRO)
			}
			return natRV(reflect.ValueOf(s).Index(i))
		case *zslice:
			if uint(i) >= uint(len(s.s)) {
				panic("reflect: slice index out of range")
			}
			e := s.elem() // an element has no state: a new cell
			return mkVM(v.t.elemT(), &e, flagAddr|v.flag&flagRO)
		case nil:
			panic("reflect: slice index out of range")
		}
	case reflect.Array:
		s := v.p.R.([]Value)
		if uint(i) >= uint(len(s)) {
			panic("reflect: array index out of range")
		}
		return mkVM(v.t.elemT(), &s[i], v.flag&(flagAddr|flagRO))
	case reflect.String:
		s, _ := v.p.R.(string)
		if uint(i) >= uint(len(s)) {
			panic("reflect: string index out of range")
		}
		return mkVM(v.t.vr.vtypeOf(types.Typ[types.Uint8]), &Value{N: int64(s[i])}, v.flag&flagRO)
	}
	panic(&reflect.ValueError{Method: "reflect.Value.Index", Kind: v.t.kind})
}

func (v RV) Len() int {
	v.mustValid("Len")
	if n, ok := v.native(); ok {
		return n.Len()
	}
	switch r := v.p.R.(type) {
	case []Value:
		return len(r)
	case []byte:
		return len(r)
	case string:
		return len(r)
	case *zslice:
		return len(r.s)
	case *MapV:
		if r == nil {
			return 0
		}
		return len(r.M)
	case chan Value:
		return len(r)
	}
	if v.t.kind == reflect.Array {
		return v.t.Len()
	}
	return 0
}

func (v RV) Cap() int {
	v.mustValid("Cap")
	if n, ok := v.native(); ok {
		return n.Cap()
	}
	switch r := v.p.R.(type) {
	case []Value:
		return cap(r)
	case []byte:
		return cap(r)
	case *zslice:
		return cap(r.s)
	case chan Value:
		return cap(r)
	}
	if v.t.kind == reflect.Array {
		return v.t.Len()
	}
	return 0
}

// Interface returns the value for AOT code: native values as themselves,
// interpreted values of predeclared types as Go values, other interpreted
// values boxed (vmBoxed: wrappers chosen by method set, or opaque handles),
// which ValueOf / TypeOf / the generated assertions unwrap.
func (v RV) Interface() any {
	v.mustValid("Interface")
	if v.flag&flagRO != 0 {
		panic("reflect.Value.Interface: cannot return value obtained from unexported field or method")
	}
	if n, ok := v.native(); ok {
		return n.Interface()
	}
	m := v.mach()
	if v.t.kind == reflect.Pointer {
		// A pointer to native data in VM memory is handed out as a native
		// pointer aliasing it (a shadow for VM-represented data, see shadow).
		if cell, ok := v.p.R.(*Value); ok && cell != nil {
			if s := ownShadow(cell); s != nil {
				return s.rv.Addr().Interface()
			}
			switch r := cell.R.(type) {
			case reflect.Value:
				if r.CanAddr() {
					return r.Addr().Interface()
				}
			case *shadowRef:
				return r.rv.Addr().Interface()
			default:
				if et := v.t.elemT().t; m.counterpart(et) != nil {
					return m.shadow(cell, et).Addr().Interface()
				}
			}
		}
	}
	var iv *Iface
	if v.t.kind == reflect.Interface {
		if iv, _ = v.p.R.(*Iface); iv == nil {
			return nil
		}
	} else {
		iv = &Iface{T: m.rtypeOf(v.t.t), V: deepCopy(*v.p, v.t.t)}
	}
	return m.ifaceAny(iv)
}

// ifaceAny converts a VM interface value to a Go value for AOT code. It
// boxes as in AOT mode (opaque handles, which the wrappers of the AOT
// copies implement), also in default mode, where AOT code runs for the
// calls routed to an AOT copy (aotroute.go): a native copy of a value lost
// its methods (encoding.TextMarshaler of a map key for AOT json).
func (m *Machine) ifaceAny(iv *Iface) any {
	if iv.T.Native != nil {
		return copyVal(iv.V.R.(reflect.Value).Interface())
	}
	nv := m.boxAs(iv, newConv(nil), true)
	if !nv.IsValid() {
		return nil
	}
	return nv.Interface()
}

func (v RV) CanInterface() bool {
	if v.t == nil {
		panic(&reflect.ValueError{Method: "reflect.Value.CanInterface", Kind: reflect.Invalid})
	}
	if v.p == nil {
		return v.n.CanInterface() && v.flag&flagRO == 0
	}
	return v.flag&flagRO == 0
}

func (v RV) CanAddr() bool {
	if v.p == nil {
		return v.n.IsValid() && v.n.CanAddr()
	}
	return v.flag&flagAddr != 0
}

func (v RV) CanSet() bool {
	if v.p == nil {
		return v.n.IsValid() && v.n.CanSet() && v.flag&flagRO == 0
	}
	return v.flag&flagAddr != 0 && v.flag&flagRO == 0
}

func (v RV) Addr() RV {
	if v.b != nil {
		// The address of the byte, as the VM has it (OpStore of *byte).
		return mkVM(v.t.ptrT(), &Value{R: v.b}, v.flag&flagRO)
	}
	if n, ok := v.native(); ok && (v.p == nil || n.CanAddr()) {
		return natRV(n.Addr())
	}
	if v.flag&flagAddr == 0 {
		panic("reflect.Value.Addr of unaddressable value")
	}
	return mkVM(v.t.ptrT(), &Value{R: v.p}, v.flag&flagRO)
}

func (v RV) IsNil() bool {
	v.mustValid("IsNil")
	if n, ok := v.native(); ok {
		return n.IsNil()
	}
	switch v.t.kind {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return isNilVal(rval{p: v.p})
	}
	panic(&reflect.ValueError{Method: "reflect.Value.IsNil", Kind: v.t.kind})
}

func (v RV) IsZero() bool {
	v.mustValid("IsZero")
	if n, ok := v.native(); ok {
		return n.IsZero()
	}
	return v.mach().vr.isZeroDeep(*v.p, v.t.t)
}

// isZeroDeep is isZero also for native data inside VM values.
func (vr *vmReflect) isZeroDeep(v Value, t types.Type) bool {
	if r, ok := v.R.(reflect.Value); ok {
		return r.IsZero()
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		fs, _ := v.R.([]Value)
		for i := range fs {
			if !vr.isZeroDeep(fs[i], u.Field(i).Type()) {
				return false
			}
		}
		return true
	case *types.Array:
		es, _ := v.R.([]Value)
		for i := range es {
			if !vr.isZeroDeep(es[i], u.Elem()) {
				return false
			}
		}
		return true
	case *types.Basic:
		if u.Info()&types.IsFloat != 0 {
			return math.Float64bits(f64(v)) == 0
		}
	}
	return vr.isZero(v, t)
}

func (v RV) kindPanic(op string, ok bool) {
	if !ok {
		panic(&reflect.ValueError{Method: "reflect.Value." + op, Kind: v.Kind()})
	}
}

func (v RV) Bool() bool {
	if n, ok := v.native(); ok {
		return n.Bool()
	}
	v.kindPanic("Bool", v.Kind() == reflect.Bool)
	return v.p.N != 0
}

func isIntKind(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Int64
}

func isUintKind(k reflect.Kind) bool {
	return k >= reflect.Uint && k <= reflect.Uintptr
}

func (v RV) Int() int64 {
	if n, ok := v.native(); ok {
		return n.Int()
	}
	v.kindPanic("Int", isIntKind(v.Kind()))
	return v.p.N
}

func (v RV) Uint() uint64 {
	if n, ok := v.native(); ok {
		return n.Uint()
	}
	v.kindPanic("Uint", isUintKind(v.Kind()))
	return truncU(uint64(v.p.N), v.t.kind)
}

func (v RV) Float() float64 {
	if n, ok := v.native(); ok {
		return n.Float()
	}
	v.kindPanic("Float", v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64)
	return f64(*v.p)
}

func (v RV) Complex() complex128 {
	if n, ok := v.native(); ok {
		return n.Complex()
	}
	v.kindPanic("Complex", v.CanComplex())
	return cplx(*v.p) // Value{R: complex128}, complex.go
}

func (v RV) CanInt() bool     { return isIntKind(v.Kind()) }
func (v RV) CanUint() bool    { return isUintKind(v.Kind()) }
func (v RV) CanFloat() bool   { return v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64 }
func (v RV) CanComplex() bool { return v.Kind() == reflect.Complex64 || v.Kind() == reflect.Complex128 }

func (v RV) String() string {
	if v.t == nil {
		return "<invalid Value>"
	}
	if n, ok := v.native(); ok {
		return n.String()
	}
	if v.t.kind == reflect.String {
		s, _ := v.p.R.(string)
		return s
	}
	return "<" + v.Type().String() + " Value>"
}

func (v RV) Bytes() []byte {
	if n, ok := v.native(); ok {
		return n.Bytes()
	}
	switch v.t.kind {
	case reflect.Slice:
		if v.t.Elem().Kind() == reflect.Uint8 {
			if b, ok := v.p.R.([]byte); ok {
				return b
			}
			if v.p.R == nil {
				return nil
			}
			return valBytes(*v.p) // copy: []Value cannot alias a []byte
		}
	case reflect.Array:
		if v.t.Elem().Kind() == reflect.Uint8 {
			return valBytes(*v.p)
		}
	}
	panic(&reflect.ValueError{Method: "reflect.Value.Bytes", Kind: v.t.kind})
}

func (v RV) UnsafePointer() unsafe.Pointer {
	if n, ok := v.native(); ok {
		return n.UnsafePointer()
	}
	switch r := v.p.R.(type) {
	case *Value:
		return unsafe.Pointer(r)
	case *MapV:
		return unsafe.Pointer(r)
	case *Closure:
		if r != nil && r.Fn != nil && v.t.kind == reflect.Func {
			return v.mach().funcPC(r.Fn) // like a code pointer (funcpc.go)
		}
		return unsafe.Pointer(r)
	case []Value:
		if cap(r) > 0 {
			return unsafe.Pointer(&r[:1][0])
		}
		if r != nil {
			return unsafe.Pointer(&zeroSizeBase)
		}
	case []byte:
		if cap(r) > 0 {
			return unsafe.Pointer(&r[:1][0])
		}
		if r != nil {
			return unsafe.Pointer(&zeroSizeBase)
		}
	}
	return nil
}

func (v RV) Pointer() uintptr { return uintptr(v.UnsafePointer()) }

// ptrIdent is the key PtrKey gives to an interpreted pointer: its identity
// (the VM cell, see UnsafePointer) and its type, as for a Go pointer in an
// interface.
type ptrIdent struct {
	p unsafe.Pointer
	t RT
}

// PtrKey returns v.Interface() as a map key for the pointer v: two calls
// with the same pointer of the same type give equal keys. For an
// interpreted pointer, v.Interface() is a new box at each call (an opaque
// handle or a wrapper) and never equals the last one. encoding/json uses
// v.Interface() as the key of its pointer cycle check (ptrEncoder, ptrSeen):
// with a box, it never found the cycle, and a pointer that points back to
// itself grew the Go stack until gc killed the whole host. cmd/aotgen
// rewrites that call to PtrKey (rewrite.go, jsonPtrKey).
func PtrKey(v RV) any {
	if _, ok := v.native(); ok {
		return v.Interface()
	}
	return ptrIdent{v.UnsafePointer(), v.Type()}
}

// AddrCopy returns an addressable copy of x, like
//
//	n := reflect.New(x.Type()).Elem()
//	n.Set(x)
//
// except for an interpreted map or slice whose type has a native
// counterpart (map[string]any, []any): New gives a native cell, and Set
// fills it with a new native copy of the map or slice. AddrCopy gives a VM
// cell instead, which shares the map or the backing array, as Set does in
// Go. encoding/json v2 makes such a copy of the value of each interface it
// marshals, then keys its cycle check on UnsafePointer: with a new native
// copy at each level, a map or a slice that contains itself was never seen
// twice. cmd/aotgen rewrites that copy to AddrCopy (rewrite.go,
// jsonV2Cycle).
func AddrCopy(x RV) RV {
	if x.t != nil && x.p != nil && x.flag&flagRO == 0 && (x.t.kind == reflect.Map || x.t.kind == reflect.Slice) {
		switch x.p.R.(type) {
		case *MapV, []Value:
			c := new(Value)
			*c = *x.p
			return mkVM(x.t, c, flagAddr)
		}
	}
	n := New(x.Type()).Elem()
	n.Set(x)
	return n
}

// UnsafeAddr returns the address of the VM cell (identity only: VM memory
// cannot be accessed through it).
// UnsafeAddrOf is unsafe.Pointer(v.UnsafeAddr()) for AOT code (cmd/aotgen
// rewrites that conversion: go vet only accepts it for reflect.Value).
func UnsafeAddrOf(v RV) unsafe.Pointer {
	if n, ok := v.native(); ok && v.p == nil {
		return unsafe.Pointer(n.UnsafeAddr())
	}
	if v.flag&flagAddr == 0 {
		panic("reflect.Value.UnsafeAddr of unaddressable value")
	}
	rememberUnsafe(v) // see unsafeField
	return unsafe.Pointer(v.p)
}

func (v RV) UnsafeAddr() uintptr {
	if n, ok := v.native(); ok && v.p == nil {
		return n.UnsafeAddr()
	}
	if v.flag&flagAddr == 0 {
		panic("reflect.Value.UnsafeAddr of unaddressable value")
	}
	rememberUnsafe(v) // see unsafeField
	return uintptr(unsafe.Pointer(v.p))
}

func (v RV) OverflowInt(x int64) bool     { return v.Type().OverflowInt(x) }
func (v RV) OverflowUint(x uint64) bool   { return v.Type().OverflowUint(x) }
func (v RV) OverflowFloat(x float64) bool { return v.Type().OverflowFloat(x) }
func (v RV) OverflowComplex(x complex128) bool {
	return v.Type().OverflowComplex(x)
}

func (v RV) Comparable() bool {
	switch v.Kind() {
	case reflect.Invalid:
		return false
	case reflect.Interface:
		return v.Elem().Comparable()
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !v.Field(i).Comparable() {
				return false
			}
		}
		return true
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !v.Index(i).Comparable() {
				return false
			}
		}
		return true
	}
	return v.Type().Comparable()
}

func (v RV) Equal(u RV) bool {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if u.Kind() == reflect.Interface {
		u = u.Elem()
	}
	if !v.IsValid() || !u.IsValid() {
		return v.IsValid() == u.IsValid()
	}
	if v.Kind() != u.Kind() || v.Type() != u.Type() {
		return false
	}
	vn, vok := v.native()
	un, uok := u.native()
	switch {
	case vok && uok:
		return vn.Equal(un)
	case !vok && !uok:
		if !v.t.Comparable() {
			panic("reflect.Value.Equal: values of type " + v.Type().String() + " are not comparable")
		}
		return v.mach().equal(*v.p, *u.p, v.t.t)
	}
	// Mixed: compare natively.
	if vok {
		return vn.Equal(u.natVal(vn.Type()))
	}
	return un.Equal(v.natVal(un.Type()))
}

// ---- setters ----

func (v RV) mustSet(op string) {
	if v.t == nil {
		panic(&reflect.ValueError{Method: "reflect.Value." + op, Kind: reflect.Invalid})
	}
	if v.flag&flagROMem != 0 {
		refuseROWrite()
	}
	if v.flag&flagRO != 0 {
		panic("reflect: reflect.Value." + op + " using value obtained using unexported field")
	}
	if v.p == nil {
		return // reflect checks native values itself
	}
	if v.flag&flagAddr == 0 {
		panic("reflect: reflect.Value." + op + " using unaddressable value")
	}
}

// setNat performs f on the native data of v when it can be set in place.
//
// A cell that holds a native slice view (natslice.go) holds a slice header
// of its own: the view itself is not settable (it comes from Slice3). f
// runs on a settable copy of the header, and the new header is stored back
// into the cell. The native array is shared: writes to the elements reach
// it, as in Go; a new length or a new array changes only this variable.
func (v RV) setNat(f func(n reflect.Value)) bool {
	n, ok := v.native()
	if !ok {
		return false
	}
	if v.p == nil || n.CanSet() {
		v.writer().hostWrite(n)
		f(n)
		return true
	}
	if _, ok := v.p.R.(reflect.Value); ok && isNatView(n) {
		c := reflect.New(n.Type()).Elem()
		c.Set(n)
		f(c)
		*v.p = viewValue(c)
		return true
	}
	return false
}

func (v RV) Set(x RV) {
	v.mustSet("Set")
	if v.setNat(func(n reflect.Value) { setNative(n, x.natStored(n.Type())) }) {
		return
	}
	storeCell(v.p, x.vmVal(v.mach(), v.t.t), v.t.t)
	v.syncByte()
}

func (v RV) SetBool(b bool) {
	v.mustSet("SetBool")
	if v.setNat(func(n reflect.Value) { n.SetBool(b) }) {
		return
	}
	*v.p = Value{N: b2i(b)}
}

func truncN(n int64, k reflect.Kind) int64 {
	switch k {
	case reflect.Int8:
		return int64(int8(n))
	case reflect.Int16:
		return int64(int16(n))
	case reflect.Int32:
		return int64(int32(n))
	case reflect.Uint8:
		return int64(uint8(n))
	case reflect.Uint16:
		return int64(uint16(n))
	case reflect.Uint32:
		return int64(uint32(n))
	case reflect.Int:
		return int64(int(n))
	case reflect.Uint:
		return int64(uint(n))
	case reflect.Uintptr:
		return int64(uintptr(n))
	}
	return n
}

func truncU(n uint64, k reflect.Kind) uint64 { return uint64(truncN(int64(n), k)) }

func (v RV) SetInt(x int64) {
	v.mustSet("SetInt")
	if v.setNat(func(n reflect.Value) { n.SetInt(x) }) {
		return
	}
	v.kindPanic("SetInt", isIntKind(v.t.kind))
	*v.p = Value{N: truncN(x, v.t.kind)}
}

func (v RV) SetUint(x uint64) {
	v.mustSet("SetUint")
	if v.setNat(func(n reflect.Value) { n.SetUint(x) }) {
		return
	}
	v.kindPanic("SetUint", isUintKind(v.t.kind))
	*v.p = Value{N: truncN(int64(x), v.t.kind)}
	v.syncByte()
}

// byteElem is the element at p of a slice kept as Go bytes, of the byte
// type t of the program: an addressable cell that copies the byte.
func byteElem(t *vtype, p *byte, flag int64) RV {
	return RV{t: t, p: &Value{N: int64(*p)}, flag: flagAddr | flag, b: p}
}

// syncByte writes the cell of v back to the byte it copies, if any.
func (v RV) syncByte() {
	if v.b != nil {
		*v.b = byte(v.p.N)
	}
}

func (v RV) SetFloat(x float64) {
	v.mustSet("SetFloat")
	if v.setNat(func(n reflect.Value) { n.SetFloat(x) }) {
		return
	}
	if v.t.kind == reflect.Float32 {
		x = float64(float32(x))
	}
	*v.p = Value{N: fv(x)}
}

func (v RV) SetComplex(x complex128) {
	v.mustSet("SetComplex")
	if v.setNat(func(n reflect.Value) { n.SetComplex(x) }) {
		return
	}
	v.kindPanic("SetComplex", v.CanComplex())
	*v.p = cplxVal(x, v.t.t) // complex64 rounds, as in gc
}

func (v RV) SetString(s string) {
	v.mustSet("SetString")
	if v.setNat(func(n reflect.Value) { n.SetString(s) }) {
		return
	}
	*v.p = Value{R: s}
}

func (v RV) SetBytes(b []byte) {
	v.mustSet("SetBytes")
	if v.setNat(func(n reflect.Value) { n.SetBytes(b) }) {
		return
	}
	if optBytes {
		*v.p = Value{R: b}
		return
	}
	*v.p = BytesVal(b)
}

func (v RV) SetZero() {
	v.mustSet("SetZero")
	if v.setNat(func(n reflect.Value) { n.SetZero() }) {
		return
	}
	storeCell(v.p, deepCopy(v.mach().zero(v.t.t), v.t.t), v.t.t)
	v.syncByte()
}

func (v RV) SetLen(n int) {
	v.mustSet("SetLen")
	if v.setNat(func(r reflect.Value) { r.SetLen(n) }) {
		return
	}
	switch s := v.p.R.(type) {
	case []Value:
		*v.p = Value{R: s[:n]}
	case []byte:
		*v.p = Value{R: s[:n]}
	case nil:
		if n != 0 {
			panic("reflect: slice length out of range in SetLen")
		}
	}
}

func (v RV) SetCap(n int) {
	v.mustSet("SetCap")
	if v.setNat(func(r reflect.Value) { r.SetCap(n) }) {
		return
	}
	switch s := v.p.R.(type) {
	case []Value:
		*v.p = Value{R: s[:len(s):n]}
	case []byte:
		*v.p = Value{R: s[:len(s):n]}
	}
}

func (v RV) Grow(n int) {
	v.mustSet("Grow")
	if v.setNat(func(r reflect.Value) { r.Grow(n) }) {
		return
	}
	switch s := v.p.R.(type) {
	case []byte:
		if cap(s)-len(s) < n {
			ns := make([]byte, len(s), max(2*cap(s), len(s)+n))
			copy(ns, s)
			*v.p = Value{R: ns}
		}
		return
	}
	s, _ := v.p.R.([]Value)
	if cap(s)-len(s) < n {
		nc := max(2*cap(s), len(s)+n, 4)
		et := v.t.t.Underlying().(*types.Slice).Elem()
		ns := v.mach().vr.makeSlice(et, len(s), nc)
		growValues(ns, s, et)
		*v.p = Value{R: ns}
	}
}

func (v RV) Clear() {
	if n, ok := v.native(); ok {
		v.writer().hostWriteIn(n)
		n.Clear()
		return
	}
	switch r := v.p.R.(type) {
	case *MapV:
		if r != nil {
			r.clear()
		}
	case []Value:
		et := v.t.elemT().t
		z := v.mach().zero(et)
		if wordKind(et) {
			for i := range r {
				storeWord(&r[i], deepCopy(z, et)) // a racy reader must not see half of it
			}
			break
		}
		for i := range r {
			r[i] = deepCopy(z, et)
		}
	case []byte:
		v.writer().hostBytes(r, len(r))
		clear(r)
	}
}

// ---- slices ----

func (v RV) Slice(i, j int) RV {
	if n, ok := v.native(); ok {
		return natRV(n.Slice(i, j))
	}
	return v.slice3(i, j, -1)
}

func (v RV) Slice3(i, j, k int) RV {
	if n, ok := v.native(); ok {
		return natRV(n.Slice3(i, j, k))
	}
	return v.slice3(i, j, k)
}

func (v RV) slice3(i, j, k int) RV {
	vr := v.mach().vr
	switch v.t.kind {
	case reflect.String:
		s, _ := v.p.R.(string)
		return mkVM(v.t, &Value{R: s[i:j]}, v.flag&flagRO)
	case reflect.Slice:
		var r any
		switch s := v.p.R.(type) {
		case []Value:
			if k < 0 {
				r = s[i:j]
			} else {
				r = s[i:j:k]
			}
		case []byte:
			if k < 0 {
				r = s[i:j]
			} else {
				r = s[i:j:k]
			}
		case *zslice:
			r = s.slice(i, j, k, k >= 0)
		case nil:
			if i != 0 || j != 0 {
				panic("reflect.Value.Slice: slice index out of bounds")
			}
		}
		return mkVM(v.t, &Value{R: r}, v.flag&flagRO)
	case reflect.Array:
		if v.flag&flagAddr == 0 {
			panic("reflect.Value.Slice: slice of unaddressable array")
		}
		s := v.p.R.([]Value)
		st := vr.vtypeOf(types.NewSlice(v.t.t.Underlying().(*types.Array).Elem()))
		if k < 0 {
			return mkVM(st, &Value{R: s[i:j]}, v.flag&flagRO)
		}
		return mkVM(st, &Value{R: s[i:j:k]}, v.flag&flagRO)
	}
	panic(&reflect.ValueError{Method: "reflect.Value.Slice", Kind: v.t.kind})
}

// ---- maps ----

func (v RV) mapT() *types.Map { return v.t.t.Underlying().(*types.Map) }

func (v RV) MapIndex(k RV) RV {
	if n, ok := v.native(); ok {
		e := n.MapIndex(k.natStored(n.Type().Key()))
		if b, ok := boxedOf(e); ok && n.Type().Elem().Kind() != reflect.Interface {
			return fromBoxed(b, 0)
		}
		return natRV(e)
	}
	mp, _ := v.p.R.(*MapV)
	if mp == nil {
		return RV{}
	}
	m := v.mach()
	mt := v.mapT()
	e, ok := mp.M[keyOf(k.vmVal(m, mt.Key()), mt.Key())]
	if !ok {
		return RV{}
	}
	return mkVM(m.vr.vtypeOf(mt.Elem()), &e, v.flag&flagRO)
}

func (v RV) MapKeys() []RV {
	if n, ok := v.native(); ok {
		ks := n.MapKeys()
		out := make([]RV, len(ks))
		for i := range ks {
			out[i] = natRV(ks[i])
		}
		return out
	}
	mp, _ := v.p.R.(*MapV)
	if mp == nil {
		return nil
	}
	kt := v.mach().vr.vtypeOf(v.mapT().Key())
	out := make([]RV, 0, len(mp.M))
	for k := range mp.M {
		c := mp.key(k)
		out = append(out, mkVM(kt, &c, v.flag&flagRO))
	}
	return out
}

func (v RV) SetMapIndex(k, e RV) {
	if n, ok := v.native(); ok {
		v.writer().hostWriteIn(n)
		mt := n.Type()
		if !e.IsValid() {
			n.SetMapIndex(k.natStored(mt.Key()), reflect.Value{})
			return
		}
		n.SetMapIndex(k.natStored(mt.Key()), e.natStored(mt.Elem()))
		return
	}
	if v.flag&flagRO != 0 {
		panic("reflect: reflect.Value.SetMapIndex using value obtained using unexported field")
	}
	mp, _ := v.p.R.(*MapV)
	if mp == nil {
		if !e.IsValid() {
			return // a delete from a nil map does nothing, as in gc
		}
		panic(errNilMapAssign)
	}
	m := v.mach()
	mt := v.mapT()
	kv := k.vmVal(m, mt.Key())
	kk := keyOf(kv, mt.Key())
	if !e.IsValid() {
		delete(mp.M, kk)
		mp.deleteKey(kk)
		return
	}
	kk = storeKey(kk)
	mp.M[kk] = e.vmVal(m, mt.Elem())
	mp.setKey(kk, kv)
}

// MapIter mirrors reflect.MapIter.
type MapIter struct {
	v    RV
	nat  *reflect.MapIter
	keys []any
	i    int
	cur  any
	ok   bool
}

func (v RV) MapRange() *MapIter {
	it := &MapIter{v: v}
	if n, ok := v.native(); ok {
		it.nat = n.MapRange()
		return it
	}
	if v.Kind() != reflect.Map {
		panic(&reflect.ValueError{Method: "reflect.Value.MapRange", Kind: v.Kind()})
	}
	if mp, _ := v.p.R.(*MapV); mp != nil {
		it.keys = make([]any, 0, len(mp.M))
		for k := range mp.M {
			it.keys = append(it.keys, k)
		}
	}
	return it
}

func (it *MapIter) Next() bool {
	if it.nat != nil {
		return it.nat.Next()
	}
	mp, _ := it.v.p.R.(*MapV)
	for it.i < len(it.keys) {
		k := it.keys[it.i]
		it.i++
		if _, ok := mp.M[k]; ok {
			it.cur, it.ok = k, true
			return true
		}
	}
	it.ok = false
	return false
}

func (it *MapIter) Key() RV {
	if it.nat != nil {
		return natRV(it.nat.Key())
	}
	if !it.ok {
		panic("MapIter.Key called before Next")
	}
	c := it.v.p.R.(*MapV).key(it.cur)
	return mkVM(it.v.mach().vr.vtypeOf(it.v.mapT().Key()), &c, it.v.flag&flagRO)
}

func (it *MapIter) Value() RV {
	if it.nat != nil {
		e := it.nat.Value()
		return natRV(e)
	}
	if !it.ok {
		panic("MapIter.Value called before Next")
	}
	c := it.v.p.R.(*MapV).M[it.cur]
	return mkVM(it.v.mach().vr.vtypeOf(it.v.mapT().Elem()), &c, it.v.flag&flagRO)
}

func (it *MapIter) Reset(v RV) { *it = *v.MapRange() }

func (v RV) SetIterKey(it *MapIter)   { v.Set(it.Key()) }
func (v RV) SetIterValue(it *MapIter) { v.Set(it.Value()) }

// ---- methods and calls ----

func (v RV) NumMethod() int {
	if v.t == nil {
		panic(&reflect.ValueError{Method: "reflect.Value.NumMethod", Kind: reflect.Invalid})
	}
	if n, ok := v.native(); ok {
		return n.NumMethod()
	}
	return v.t.NumMethod()
}

// boundMethod returns the method sel of v's type bound to v.
func (v RV) boundMethod(sel *types.Selection) RV {
	m := v.mach()
	sig := sel.Obj().Type().(*types.Signature)
	ft := types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic())
	recv := deepCopy(*v.p, v.t.t)
	fn := m.prog.MethodValue(sel)
	if m.isNativeNamed(baseNamed(v.t.t)) && !m.isInterp(fn) {
		// Method of a native type held in VM representation: call it natively.
		n, name := v.natVal(m.reflectType(v.t.t)), sel.Obj().Name()
		return natRV(restrictedMethodValue(n, name, n.MethodByName(name)))
	}
	f := m.funcOf(fn)
	cl := &Closure{Go: func(args []Value) Value {
		return m.Call(f, append([]Value{recv}, args...), nil)
	}}
	if m.isInterp(fn) {
		cl.Direct, cl.Recv = f, &recv
	}
	return mkVM(m.vr.vtypeOf(ft), &Value{R: cl}, v.flag&flagRO)
}

func baseNamed(t types.Type) types.Type {
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

func (v RV) Method(i int) RV {
	v.mustValid("Method")
	if n, ok := v.native(); ok {
		return natRV(restrictedMethodValue(n, n.Type().Method(i).Name, n.Method(i)))
	}
	if v.t.kind == reflect.Interface {
		return v.MethodByName(v.t.Method(i).Name)
	}
	return v.boundMethod(v.t.exported()[i])
}

func (v RV) MethodByName(name string) RV {
	v.mustValid("MethodByName")
	if n, ok := v.native(); ok {
		return natRV(restrictedMethodValue(n, name, n.MethodByName(name)))
	}
	if v.t.kind == reflect.Interface {
		iv, _ := v.p.R.(*Iface)
		if iv == nil {
			panic("reflect: Method on nil interface value")
		}
		return v.Elem().MethodByName(name)
	}
	for _, sel := range v.t.exported() {
		if sel.Obj().Name() == name {
			return v.boundMethod(sel)
		}
	}
	return RV{}
}

func (v RV) Call(in []RV) []RV      { return v.call(in, false) }
func (v RV) CallSlice(in []RV) []RV { return v.call(in, true) }

func (v RV) call(in []RV, isSlice bool) []RV {
	v.mustValid("Call")
	if n, ok := v.native(); ok {
		if isChanReaderFunc(n) {
			defer enterChanReader()() // before the data: see readsChans
		}
		if n.Kind() == reflect.Func {
			checkCallArgs(v.Type(), in, isSlice)
		}
		ft := n.Type()
		args := make([]reflect.Value, len(in))
		for i, a := range in {
			var pt reflect.Type
			switch {
			case ft.IsVariadic() && i >= ft.NumIn()-1 && !isSlice:
				pt = ft.In(ft.NumIn() - 1).Elem()
			default:
				pt = ft.In(i)
			}
			args[i] = a.natVal(pt)
		}
		var out []reflect.Value
		if isSlice {
			out = n.CallSlice(args)
		} else {
			out = n.Call(args)
		}
		res := make([]RV, len(out))
		for i := range out {
			res[i] = natRV(out[i])
		}
		return res
	}
	if v.t.kind != reflect.Func {
		panic(&reflect.ValueError{Method: "reflect.Value.Call", Kind: v.t.kind})
	}
	if v.p.R == nil {
		panic("reflect: call of nil function")
	}
	checkCallArgs(v.t, in, isSlice)
	m := v.mach()
	sig := v.t.sig()
	np := sig.Params().Len()
	args := make([]Value, 0, np)
	for i := 0; i < np; i++ {
		pt := sig.Params().At(i).Type()
		if sig.Variadic() && i == np-1 && !isSlice {
			et := pt.(*types.Slice).Elem()
			var rest []Value
			for _, a := range in[i:] {
				rest = append(rest, a.vmVal(m, et))
			}
			args = append(args, Value{R: rest})
			break
		}
		if i >= len(in) {
			panic("reflect: Call with too few input arguments")
		}
		args = append(args, in[i].vmVal(m, pt))
	}
	res := m.callValue(*v.p, args)
	nr := sig.Results().Len()
	switch nr {
	case 0:
		return nil
	case 1:
		c := new(Value)
		*c = res
		return []RV{mkVM(m.vr.vtypeOf(sig.Results().At(0).Type()), c, 0)}
	}
	rs := res.R.([]Value)
	out := make([]RV, nr)
	for i := range out {
		c := new(Value)
		*c = rs[i]
		out[i] = mkVM(m.vr.vtypeOf(sig.Results().At(i).Type()), c, 0)
	}
	return out
}

// twinAssignable is x.AssignableTo(t) for the checks of reflect calls. A
// native type and a VM type that is not an interface can be two
// descriptors of one type (the VM type of string and the native string).
// They are not assignable when their kinds differ, or when both have a
// name and the names differ. When only one has a name, the VM type is
// replaced by the native counterpart of its underlying type, which has no
// name: native AssignableTo then needs identical underlying types (type B
// []byte and []byte), or applies the rule of channels (chan int to type RC
// <-chan int). Else the conversion of the call decides, as before these
// checks.
func twinAssignable(x, t RT) bool {
	if x.isNat() != t.isNat() && t.kind != reflect.Interface {
		if x.kind != t.kind {
			return false
		}
		xn, tn := x.Name() != "", t.Name() != ""
		switch {
		case xn && tn:
			return x.String() == t.String()
		case xn || tn:
			if x.isNat() {
				c := t.vr.m.counterpart(t.t.Underlying())
				return c == nil || x.nat.AssignableTo(c)
			}
			c := x.vr.m.counterpart(x.t.Underlying())
			return c == nil || c.AssignableTo(t.nat)
		}
		return true
	}
	return x.AssignableTo(t)
}

// twinConvertible is x.ConvertibleTo(t) for Value.Convert, in the direction
// of the conversion. A VM type is compared through the native counterpart
// of its underlying type; known is false when it has none: the conversion
// then decides, as before these checks.
func twinConvertible(x, t RT) (ok, known bool) {
	if x.isNat() == t.isNat() || t.kind == reflect.Interface {
		return x.ConvertibleTo(t), true
	}
	nat := func(r RT) reflect.Type {
		if r.isNat() {
			return r.nat
		}
		return r.vr.m.counterpart(r.t.Underlying())
	}
	xn, tn := nat(x), nat(t)
	if xn == nil || tn == nil {
		// no counterpart: the kinds can still rule out a conversion
		// (a struct of the program to a string)
		if !kindConvertible(x.kind, t.kind) {
			return false, true
		}
		return false, false
	}
	return xn.ConvertibleTo(tn), true
}

// kindConvertible reports whether a conversion from kind x to kind t can be
// valid for some types of these kinds.
func kindConvertible(x, t reflect.Kind) bool {
	num := func(k reflect.Kind) bool { return k >= reflect.Int && k <= reflect.Float64 }
	cplx := func(k reflect.Kind) bool { return k == reflect.Complex64 || k == reflect.Complex128 }
	switch {
	case x == t, num(x) && num(t), cplx(x) && cplx(t):
		return true
	case t == reflect.String:
		return x == reflect.Slice || x >= reflect.Int && x <= reflect.Uintptr
	case x == reflect.String:
		return t == reflect.Slice
	case x == reflect.Slice:
		return t == reflect.Array || t == reflect.Pointer
	}
	return false
}

// checkCallArgs does the checks of gc's Value.Call on the arguments in of a
// call of a function of type t: their number and their types.
func checkCallArgs(t RT, in []RV, isSlice bool) {
	op := "Call"
	if isSlice {
		op = "CallSlice"
	}
	n, np := len(in), t.NumIn()
	if isSlice && !t.IsVariadic() {
		panic("reflect: CallSlice of non-variadic function")
	}
	variadic := !isSlice && t.IsVariadic()
	switch {
	case variadic && n < np-1, !variadic && n < np:
		panic("reflect: " + op + " with too few input arguments")
	case !variadic && n > np:
		panic("reflect: " + op + " with too many input arguments")
	}
	for _, x := range in {
		if x.t == nil {
			panic("reflect: " + op + " using zero Value argument")
		}
	}
	for i := 0; i < n && i < np; i++ {
		if variadic && i == np-1 {
			break
		}
		if xt, pt := in[i].Type(), t.In(i); !twinAssignable(xt, pt) {
			panic("reflect: " + op + " using " + xt.String() + " as type " + pt.String())
		}
	}
	if variadic {
		et := t.In(np - 1).Elem()
		for _, a := range in[np-1:] {
			if xt := a.Type(); !twinAssignable(xt, et) {
				panic("reflect: cannot use " + xt.String() + " as type " + et.String() + " in " + op)
			}
		}
	}
}

// ---- channels ----

func (v RV) vmChan() chan Value {
	ch, _ := v.p.R.(chan Value)
	return ch
}

func (v RV) chanElem() *vtype { return v.t.elemT() }

func (v RV) Send(x RV) {
	if n, ok := v.native(); ok {
		n.Send(x.natVal(n.Type().Elem()))
		return
	}
	v.vmChan() <- x.vmVal(v.mach(), v.chanElem().t)
}

func (v RV) TrySend(x RV) bool {
	if n, ok := v.native(); ok {
		return n.TrySend(x.natVal(n.Type().Elem()))
	}
	select {
	case v.vmChan() <- x.vmVal(v.mach(), v.chanElem().t):
		return true
	default:
		return false
	}
}

func (v RV) Recv() (RV, bool) {
	if n, ok := v.native(); ok {
		x, ok := n.Recv()
		return natRV(x), ok
	}
	x, ok := <-v.vmChan()
	if !ok {
		return v.zeroOf(v.chanElem()), false
	}
	return mkVM(v.chanElem(), &x, 0), true
}

func (v RV) TryRecv() (RV, bool) {
	if n, ok := v.native(); ok {
		x, ok := n.TryRecv()
		return natRV(x), ok
	}
	select {
	case x, ok := <-v.vmChan():
		if !ok {
			return v.zeroOf(v.chanElem()), false
		}
		return mkVM(v.chanElem(), &x, 0), true
	default:
		return RV{}, false
	}
}

func (v RV) zeroOf(t *vtype) RV { return mkVM(t, v.mach().vr.zeroCell(t.t), 0) }

func (v RV) Close() {
	if n, ok := v.native(); ok {
		n.Close()
		return
	}
	close(v.vmChan())
}

// GoString helps debugging.
func (v RV) GoString() string {
	if v.t == nil {
		return "vm.RV{}"
	}
	return "vm.RV{" + v.t.String() + " flag=" + strconv.FormatInt(v.flag, 10) + "}"
}

// ---- conversions ----

func isNumKind(k reflect.Kind) bool {
	return isIntKind(k) || isUintKind(k) || k == reflect.Float32 || k == reflect.Float64
}

// CanConvert mirrors reflect.Value.CanConvert.
func (v RV) CanConvert(t RT) bool {
	vt := v.Type()
	if !vt.ConvertibleTo(t) {
		return false
	}
	if vt.Kind() == reflect.Slice && t.Kind() == reflect.Array && t.Len() > v.Len() {
		return false
	}
	return true
}

// scalarOf builds a value of t set by set (on an addressable temporary).
func scalarOf(t RT, set func(RV)) RV {
	if t.isNat() {
		r := reflect.New(t.nat).Elem()
		set(natRV(r))
		return natRV(r)
	}
	c := t.vr.zeroCell(t.t)
	set(mkVM(t, c, flagAddr))
	return mkVM(t, c, 0)
}

// Convert mirrors reflect.Value.Convert.
func (v RV) Convert(t RT) RV {
	v.mustValid("Convert")
	if ok, known := twinConvertible(v.Type(), t); known && !ok {
		vt := v.Type()
		panic("reflect.Value.Convert: value of type " + vt.String() + " cannot be converted to type " + t.String())
	}
	if v.Kind() == reflect.Slice && t.kind == reflect.Array && t.Len() > v.Len() {
		panic("reflect: cannot convert slice with length " + strconv.Itoa(v.Len()) + " to array with length " + strconv.Itoa(t.Len()))
	}
	if v.Kind() == reflect.Slice && t.kind == reflect.Pointer && t.Elem().Kind() == reflect.Array && t.Elem().Len() > v.Len() {
		panic("reflect: cannot convert slice with length " + strconv.Itoa(v.Len()) + " to pointer to array with length " + strconv.Itoa(t.Elem().Len()))
	}
	n, isN := v.native()
	if isN && t.isNat() {
		return natRV(n.Convert(t.nat))
	}
	if t.kind == reflect.Interface {
		if t.isNat() {
			r := reflect.New(t.nat).Elem()
			r.Set(v.natVal(t.nat))
			return natRV(r)
		}
		c := new(Value)
		*c = v.vmVal(t.vr.m, t.t)
		return mkVM(t, c, 0)
	}
	sk, tk := v.Kind(), t.kind
	switch {
	case isNumKind(sk) && isNumKind(tk):
		return scalarOf(t, func(r RV) {
			switch {
			case tk == reflect.Float32 || tk == reflect.Float64:
				switch {
				case isIntKind(sk):
					r.SetFloat(float64(v.Int()))
				case isUintKind(sk):
					r.SetFloat(float64(v.Uint()))
				default:
					r.SetFloat(v.Float())
				}
			case isIntKind(tk):
				switch {
				case isIntKind(sk):
					r.SetInt(v.Int())
				case isUintKind(sk):
					r.SetInt(int64(v.Uint()))
				default:
					r.SetInt(int64(v.Float()))
				}
			default:
				switch {
				case isIntKind(sk):
					r.SetUint(uint64(v.Int()))
				case isUintKind(sk):
					r.SetUint(v.Uint())
				default:
					r.SetUint(uint64(v.Float()))
				}
			}
		})
	case sk == reflect.Bool && tk == reflect.Bool:
		return scalarOf(t, func(r RV) { r.SetBool(v.Bool()) })
	case tk == reflect.String && (isIntKind(sk) || isUintKind(sk)):
		var x int64
		if isIntKind(sk) {
			x = v.Int()
		} else {
			x = int64(v.Uint())
		}
		s := intToStr(x)
		return scalarOf(t, func(r RV) { r.SetString(s) })
	case tk == reflect.String && sk == reflect.String:
		return scalarOf(t, func(r RV) { r.SetString(v.String()) })
	case tk == reflect.String && sk == reflect.Slice:
		var s string
		if v.Type().Elem().Kind() == reflect.Uint8 {
			s = string(v.Bytes())
		} else {
			s = runesString(v.Len(), func(i int) rune { return rune(v.Index(i).Int()) })
		}
		return scalarOf(t, func(r RV) { r.SetString(s) })
	case tk == reflect.Slice && sk == reflect.String:
		s := v.String()
		if t.Elem().Kind() == reflect.Uint8 {
			if t.isNat() {
				return natRV(reflect.ValueOf([]byte(s)).Convert(t.nat))
			}
			return mkVM(t, &Value{R: BytesVal([]byte(s)).R}, 0)
		}
		if t.isNat() {
			return natRV(reflect.ValueOf([]rune(s)).Convert(t.nat))
		}
		return mkVM(t, &Value{R: strToRuneVals(s)}, 0)
	}
	if sk == reflect.Slice && tk == reflect.Array && v.Len() > t.Len() {
		// the first t.Len() elements of the slice
		v = v.Slice(0, t.Len())
		n, isN = v.native()
	}
	// Same underlying type: copy the representation.
	if t.isNat() {
		return natRV(v.natVal(t.nat))
	}
	c := new(Value)
	if isN {
		*c = t.vr.m.natToVM(n, t.t)
	} else {
		*c = deepCopy(*v.p, t.t)
	}
	return mkVM(t, c, 0)
}

// ---- shadows ----

// shadowRef replaces, while AOT code runs, the content of a VM cell holding
// a value of a native named type in VM representation (e.g. an
// asn1.ObjectIdentifier field, VM []Value) whose address AOT code took: AOT
// code reads and writes the native shadow through a real pointer, and the
// cell gets its VM representation back when control returns to the VM
// (flushShadows, at every native -> VM transition).
//
// A shadow belongs to the goroutine that made it, and only the transitions
// of that goroutine give it back. Many goroutines run AOT code on the same
// machine at the same time (concurrent requests of a plugin), often on the
// same cells (a shared config):
//   - an owned shadow (the goroutine runs interpreted code: it has a
//     gstate) is recorded in gstate.shadows and found there by its cell
//     (ownShadow); the cell itself is not changed, so other goroutines go on
//     reading its VM value. When it is given back, the cell is written only
//     if the value changed (storeChanged), so a read only call (json.Marshal
//     of a shared config) writes nothing, as in Go;
//   - a shadow made on a goroutine without gstate (AOT code called from a
//     goroutine that native code started) is an orphan: it is stored in the
//     cell and in Machine.shadows, and any goroutine gives it back.
type shadowRef struct {
	m    *Machine
	rv   reflect.Value // addressable native value
	t    types.Type
	cell *Value
	c    *natConv  // the conversion of the cell content: maps and slices keep their identity
	wb   *[]func() // its write-backs
	// A shadowRef without cell is the copy of a map or a slice that AOT code
	// got with Unbox: detach only runs its write-backs, once.
	owned bool // in gstate.shadows, not stored in the cell
	// parked is the cell as detach left it, while the shadow is suspended
	// (suspendShadows): resumeShadows keeps the native value when the
	// callback did not change the cell.
	parked   Value
	isParked bool
}

// ownedShadows counts the owned shadows of all machines, and shadowSlots
// counts them by a hash of their cell: a lookup costs one atomic load when
// no goroutine shadows a cell of the same slot.
var (
	ownedShadows atomic.Int32
	shadowSlots  [256]atomic.Int32
)

func shadowSlot(cell *Value) *atomic.Int32 {
	return &shadowSlots[uint64(uintptr(unsafe.Pointer(cell)))*0x9E3779B97F4A7C15>>56]
}

// findShadow returns the shadow of cell in ss, or nil.
func findShadow(ss []*shadowRef, cell *Value) *shadowRef {
	for i := len(ss) - 1; i >= 0; i-- {
		if ss[i].cell == cell {
			return ss[i]
		}
	}
	return nil
}

// ownShadow returns the shadow of cell that the current goroutine owns, or
// nil. Code that reads cells while AOT code runs must look there: an owned
// shadow is not stored in its cell (deepCopy does it for struct fields and
// array elements).
func ownShadow(cell *Value) *shadowRef {
	if shadowSlot(cell).Load() == 0 {
		return nil
	}
	if gs := lookupG(getg()); gs != nil {
		return findShadow(gs.shadows, cell)
	}
	return nil
}

// countShadow adds d to the counts of the owned shadow s.
func countShadow(s *shadowRef, d int32) {
	ownedShadows.Add(d)
	if s.cell != nil {
		shadowSlot(s.cell).Add(d)
	}
}

func (m *Machine) shadow(cell *Value, t types.Type) reflect.Value {
	if s := ownShadow(cell); s != nil {
		return s.rv
	}
	if s, ok := cell.R.(*shadowRef); ok {
		return s.rv
	}
	if gs := lookupG(getg()); gs != nil {
		if s := findShadow(gs.attaching, cell); s != nil {
			return s.rv // the cell holds a pointer to itself
		}
	}
	rv := reflect.New(m.reflectType(t)).Elem()
	s := &shadowRef{m: m, rv: rv, t: t, cell: cell}
	s.attach()
	m.addShadow(s)
	return rv
}

// addShadow records s with the current goroutine (see shadowRef).
func (m *Machine) addShadow(s *shadowRef) {
	if gs := lookupG(getg()); gs != nil {
		s.owned = true
		gs.shadows = append(gs.shadows, s)
		countShadow(s, 1)
		return
	}
	s.owned = false
	m.shadowMu.Lock()
	if s.cell != nil {
		*s.cell = Value{R: s}
	}
	m.shadows = append(m.shadows, s)
	m.nOrphans.Add(1)
	m.shadowMu.Unlock()
}

// takeShadows removes and returns the shadows of m that the current
// goroutine gives back: its own ones, and the orphans.
func (m *Machine) takeShadows() []*shadowRef {
	var ss []*shadowRef
	if ownedShadows.Load() > 0 {
		if gs := lookupG(getg()); gs != nil && len(gs.shadows) > 0 {
			own := gs.shadows
			gs.shadows = nil
			if !slices.ContainsFunc(own, func(s *shadowRef) bool { return s.m != m }) {
				ss = own
			} else {
				// another machine runs on this goroutine (a nested
				// run): its shadows wait for its own transitions
				for _, s := range own {
					if s.m == m {
						ss = append(ss, s)
					} else {
						gs.shadows = append(gs.shadows, s)
					}
				}
			}
			for _, s := range ss {
				countShadow(s, -1)
			}
		}
	}
	if m.nOrphans.Load() > 0 {
		m.shadowMu.Lock()
		ss = append(ss, m.shadows...)
		m.shadows = nil
		m.nOrphans.Store(0)
		m.shadowMu.Unlock()
	}
	return ss
}

// dropShadows gives back the shadows still recorded with gs when its
// goroutine stops running interpreted code (leaveG): the next goroutine
// that gets the same key must not inherit them. Normally a transition has
// already given them back.
func (gs *gstate) dropShadows() {
	ss := gs.shadows
	gs.shadows = nil
	for _, s := range ss {
		countShadow(s, -1)
		s.detach()
	}
}

// live reports whether s still shadows its cell, so that detach must give
// the cell its VM representation back.
func (s *shadowRef) live() bool {
	if s.cell == nil || s.owned {
		return true
	}
	cur, ok := s.cell.R.(*shadowRef)
	return ok && cur == s
}

// suspendShadows gives the shadowed cells their VM representation back for
// the duration of a call into interpreted code (resumeShadows re-attaches
// them, so that native pointers handed out earlier stay coherent).
func (m *Machine) suspendShadows() []*shadowRef {
	var out []*shadowRef
	for _, s := range m.takeShadows() {
		if !s.live() {
			continue
		}
		s.detach()
		if s.cell != nil { // an Unbox copy is written back now, not re-attached
			s.parked, s.isParked = *s.cell, true
			out = append(out, s)
		}
	}
	return out
}

// attach converts the content of the cell into the native value of the
// shadow. The maps and slices of the cell are recorded (natConv.track):
// detach writes the changes of native code back into them, so that every
// interpreted alias of a map sees them, as in Go.
func (s *shadowRef) attach() {
	s.wb = new([]func())
	s.c = newConv(s.wb)
	if gs := lookupG(getg()); gs != nil && s.cell != nil {
		gs.attaching = append(gs.attaching, s)
		defer func() { gs.attaching = gs.attaching[:len(gs.attaching)-1] }()
	}
	s.rv.Set(s.m.toNativeW(*s.cell, s.t, s.c))
}

// detach gives the cell its VM representation back.
func (s *shadowRef) detach() {
	if s.cell == nil {
		writeBack(*s.wb)
		*s.wb = nil
		return
	}
	v := s.m.fromNativeDeepC(s.rv, s.t, s.c)
	writeBack(*s.wb)
	if s.owned {
		// the cell still holds the VM value: other goroutines may read it
		storeChanged(s.cell, v, s.t)
	} else {
		*s.cell = v
	}
}

func (m *Machine) resumeShadows(ss []*shadowRef) {
	for _, s := range ss {
		if ownShadow(s.cell) != nil {
			continue
		}
		if _, ok := s.cell.R.(*shadowRef); ok {
			continue
		}
		// A callback that did not change the cell: the native value stays,
		// so the reflect.Values that the native code took before the
		// callback still point into it (gob decodes a []any element, calls
		// GobDecode of the program, then sets the element).
		// A new conversion made new memory, and the
		// decoded values went to the old one.
		parked := s.isParked && sameCell(*s.cell, s.parked)
		s.parked, s.isParked = Value{}, false
		if parked {
			// the same conversion for the next detach, so the maps and
			// slices still go back to the same VM ones (every alias sees
			// them), but ready to write back again
			s.c.g.reopen()
		} else {
			s.attach()
		}
		m.addShadow(s)
	}
}

// sameCell reports whether the cell values a and b are the same value: the
// same word, and the same reference (the same backing array for a slice,
// the same map, pointer, interface or string).
func sameCell(a, b Value) bool {
	if a.N != b.N {
		return false
	}
	switch x := a.R.(type) {
	case []Value:
		y, ok := b.R.([]Value)
		return ok && len(x) == len(y) && cap(x) == cap(y) && (cap(x) == 0 || unsafe.SliceData(x) == unsafe.SliceData(y))
	case []byte:
		y, ok := b.R.([]byte)
		return ok && len(x) == len(y) && cap(x) == cap(y) && (cap(x) == 0 || unsafe.SliceData(x) == unsafe.SliceData(y))
	case reflect.Value:
		return false
	}
	if a.R == nil || b.R == nil {
		return a.R == b.R
	}
	if t := reflect.TypeOf(a.R); t != reflect.TypeOf(b.R) || !t.Comparable() {
		return false
	}
	return a.R == b.R
}

// flushShadows gives the cells shadowed by AOT code their VM representation
// back. It takes the shadows of the current goroutine only, and the
// orphans (see shadowRef).
func (m *Machine) flushShadows() {
	if ownedShadows.Load() == 0 && m.nOrphans.Load() == 0 {
		return
	}
	for _, s := range m.takeShadows() {
		if s.live() {
			s.detach()
		}
	}
}

// hasMethods reports whether t or *t has methods.
func hasMethods(t types.Type) bool {
	if _, ok := types.Unalias(t).(*types.Named); !ok {
		return false
	}
	return types.NewMethodSet(types.NewPointer(t)).Len() > 0
}
