package vm

// Bridge between the two reflect implementations an AOT program meets:
//
//   - the interpreted program's own reflect calls run as VM intrinsics
//     (vmreflect.go): a reflect.Type is an interface value of dynamic type
//     vr.typeRT carrying a *vtype, a reflect.Value the 3-field VM struct of
//     encV;
//   - AOT code (vreflect, twin_*.go) uses native RT (*vtype) and RV values.
//
// Library APIs that mention reflect (mapstructure decode hooks, validator's
// FieldLevel, go-cmp options...) hand such values across the boundary: they
// are converted here, both ways, sharing the *vtype descriptors and the VM
// cells (no copy for VM-backed values).

import (
	"go/types"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

var (
	rtNative = reflect.TypeFor[*vtype]()
	rvNative = reflect.TypeFor[RV]()
)

// twinNamed returns the native twin type standing for the reflect type t
// (reflect.Type, reflect.Value) when the VM reflect is enabled, nil otherwise.
func (m *Machine) twinNamed(t types.Type) reflect.Type {
	if !m.progVR {
		return nil
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || !isReflectPkg(n.Obj().Pkg()) {
		return nil
	}
	switch n.Obj().Name() {
	case "Type":
		return rtNative
	case "Value":
		return rvNative
	}
	return nil
}

// reflectNamed returns the go/types reflect.Type or reflect.Value of the
// interpreted program (nil when it does not import reflect).
func (m *Machine) reflectNamed(name string) types.Type {
	if !m.progVR {
		return nil
	}
	p := m.vr.typesPkg("reflect")
	if p == nil {
		return nil
	}
	obj, ok := p.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil
	}
	return obj.Type()
}

// twinFromVM converts the VM encoding of a reflect.Type or reflect.Value to
// the twin's native value of type rt (ok is false when rt is neither).
func (m *Machine) twinFromVM(v Value, rt reflect.Type) (reflect.Value, bool) {
	if !m.progVR || rt != rtNative && rt != rvNative {
		return reflect.Value{}, false
	}
	if r, ok := v.R.(reflect.Value); ok && r.Type() == rt {
		return r, true
	}
	if rt == rtNative {
		var vt *vtype
		switch x := v.R.(type) {
		case *Iface:
			vt, _ = x.V.R.(*vtype)
		case *vtype:
			vt = x
		}
		return reflect.ValueOf(vt.cn()), true
	}
	x := decV(v)
	if x.t != nil && x.t.isNat() && x.p != nil {
		if n, ok := x.p.R.(reflect.Value); ok {
			// native data (vmreflect_nat.go): a native-backed value, so that
			// native reflect decides what can be set
			return reflect.ValueOf(RV{t: x.t, n: n, flag: x.flag & flagRO}), true
		}
	}
	return reflect.ValueOf(RV{t: x.t, p: x.p, flag: x.flag}), true
}

// twinToVM converts a native twin value (RT or RV) to the VM encoding used
// by the interpreted program's reflect (ok is false for other values).
func (m *Machine) twinToVM(rv reflect.Value) (Value, bool) {
	if !m.progVR || !rv.IsValid() {
		return Value{}, false
	}
	switch rv.Type() {
	case rtNative:
		nt := rv.Interface().(*vtype)
		vt := m.vr.vmVT(nt)
		if vt == nil {
			if nt != nil {
				// no go/types type: keep the native descriptor (vmreflect_nat.go)
				return Value{R: &Iface{T: m.vr.typeRT, V: Value{R: nt}}}, true
			}
			return Value{}, true
		}
		return Value{R: vt.ifc}, true
	case rvNative:
		x := rv.Interface().(RV)
		if x.t == nil {
			return encV(nil, nil, 0), true
		}
		if x.b != nil {
			// a byte of a Go []byte: the copy in x.p alone would lose the
			// writes (rval.b)
			return encByte(x.t, x.b, x.flag), true
		}
		if x.p != nil {
			return encV(x.t, x.p, x.flag), true
		}
		// native-backed value: keep the native data, not a copy, so that
		// writes reach it (vmreflect_nat.go)
		return encV(x.t, &Value{R: x.n}, x.flag&flagRO), true
	}
	return Value{}, false
}

var (
	realTypeT  = reflect.TypeFor[reflect.Type]()
	realValueT = reflect.TypeFor[reflect.Value]()
)

// realReflectArgs converts the twin values (RT, RV) of in that a native
// function of type ft takes as a real reflect.Type or reflect.Value
// (testing/quick.Value(reflect.TypeOf(0), r)). The twin types stand for the
// program's reflect types in the parameters of AOT code only.
func (m *Machine) realReflectArgs(ft reflect.Type, in []reflect.Value) {
	if !m.progVR {
		return
	}
	n := ft.NumIn()
	if ft.IsVariadic() {
		n-- // the variadic slice is passed as it is
	}
	for i := 0; i < n && i < len(in); i++ {
		x, want := in[i], ft.In(i)
		if !x.IsValid() || want != realTypeT && want != realValueT {
			continue
		}
		switch x.Type() {
		case rtNative:
			if rt := m.realType(x.Interface().(*vtype)); rt != nil {
				in[i] = reflect.ValueOf(&rt).Elem()
			} else {
				in[i] = reflect.Zero(realTypeT)
			}
		case rvNative:
			in[i] = reflect.ValueOf(m.realValue(x.Interface().(RV)))
		}
	}
}

// realType returns the native reflect.Type of vt (nil for nil). For an
// interpreted named type, this is the native type of reflectType: its
// methods are lost (the reflect wall).
func (m *Machine) realType(vt *vtype) (rt reflect.Type) {
	if vt == nil {
		return nil
	}
	if vt.isNat() {
		return vt.nat
	}
	if c := m.counterpart(vt.t); c != nil {
		return c
	}
	defer func() {
		if recover() != nil {
			rt = nil
		}
	}()
	return m.reflectType(vt.t)
}

// realValue returns the native reflect.Value of v: its native data, or a
// native copy of a VM value (not addressable: writes do not reach the VM).
func (m *Machine) realValue(v RV) reflect.Value {
	if v.t == nil {
		return reflect.Value{}
	}
	if n, ok := v.native(); ok {
		return n
	}
	rt := m.realType(v.t)
	if rt == nil {
		return reflect.Value{}
	}
	return v.natVal(rt)
}

// realReflectToVM converts a real reflect.Type or reflect.Value returned by
// native code (rv) to the program's VM encoding of the reflect type tw
// (rtNative or rvNative); ok is false for other values.
func (m *Machine) realReflectToVM(rv reflect.Value, tw reflect.Type) (Value, bool) {
	if !m.progVR || !rv.IsValid() {
		return Value{}, false
	}
	e := rv
	if e.Kind() == reflect.Interface {
		if e.IsNil() {
			return Value{}, e.Type() == realTypeT
		}
		e = e.Elem()
	}
	switch {
	case tw == rtNative && e.Type() != rtNative:
		if rt, ok := e.Interface().(reflect.Type); ok {
			return m.vr.natTypeVal(rt), true
		}
	case tw == rvNative && e.Type() == realValueT:
		n := e.Interface().(reflect.Value)
		if !n.IsValid() {
			return encV(nil, nil, 0), true
		}
		return m.vr.natValueOf(n), true
	}
	return Value{}, false
}

// twinIfaceVM converts a twin value stored in a native interface to a VM
// interface value of the matching reflect type.
func (m *Machine) twinIfaceVM(e reflect.Value) (Value, bool) {
	if e.Type() != rtNative && e.Type() != rvNative {
		return Value{}, false
	}
	v, ok := m.twinToVM(e)
	if !ok || e.Type() == rtNative {
		return v, ok // a reflect.Type is already an interface value
	}
	gt := m.reflectNamed("Value")
	if gt == nil {
		return Value{}, false
	}
	return Value{R: &Iface{T: m.rtypeOf(gt), V: v}}, true
}

// vmVT returns the VM descriptor of vt (VM descriptors have a go/types type;
// native ones are mapped to the type of the interpreted program, nil when
// there is none).
func (vr *vmReflect) vmVT(vt *vtype) *vtype {
	if vt == nil || !vt.isNat() {
		return vt
	}
	gt := vr.m.goTypeOf(vt.nat)
	if gt == nil {
		return nil
	}
	return vr.vtypeOf(gt)
}

// twinNames maps the twin's native type names to reflect's in type strings.
var twinNames = strings.NewReplacer(
	"*vm.vtype", "reflect.Type",
	"vm.RV", "reflect.Value",
	"vm.StructField", "reflect.StructField",
	"vm.RMethod", "reflect.Method",
	"vm.MapIter", "reflect.MapIter",
	"vm.SelectCase", "reflect.SelectCase",
)

// natString formats a native type like reflect does for the program: the
// twin's types are reported as reflect's.
func natString(rt reflect.Type) string {
	s := rt.String()
	if strings.Contains(s, "vm.") {
		return twinNames.Replace(s)
	}
	return s
}

// ---- unsafe access to unexported fields ----
//
// reflect.NewAt(f.Type, unsafe.Pointer(v.UnsafeAddr()+f.Offset)) is the
// idiom for reading unexported fields (go-cmp's AllowUnexported; validator
// uses it with offset 0). The address of a VM-backed value is the address of
// its VM cell, not of Go memory laid out like the value: UnsafeAddr remembers
// the last such cells and NewAt maps the address of one of them, or of a
// field inside a struct cell, back to the VM cell.

type unsafeCell struct {
	base uintptr
	v    RV
}

var (
	unsafeMu   sync.Mutex
	unsafeRing [64]unsafeCell
	unsafeNext int
)

func rememberUnsafe(v RV) {
	if v.t == nil || v.t.isNat() || v.p == nil {
		return
	}
	unsafeMu.Lock()
	unsafeRing[unsafeNext] = unsafeCell{base: uintptr(unsafe.Pointer(v.p)), v: v}
	unsafeNext = (unsafeNext + 1) % len(unsafeRing)
	unsafeMu.Unlock()
}

// unsafeField finds the remembered VM cell at address p (offset 0, e.g.
// validator's NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr()))), or the field
// of a remembered VM struct cell at address p, whose type is t.
func unsafeField(t RT, p unsafe.Pointer) (RV, bool) {
	addr := uintptr(p)
	unsafeMu.Lock()
	defer unsafeMu.Unlock()
	for i := range unsafeRing {
		c := unsafeRing[i]
		if c.v.t == nil || addr < c.base {
			continue
		}
		if addr == c.base && (c.v.t.cn() == t || c.v.t == t) {
			return mkVM(c.v.t, c.v.p, flagAddr), true
		}
		cells, ok := c.v.p.R.([]Value)
		if !ok || c.v.t.kind != reflect.Struct {
			continue
		}
		offs := c.v.t.fieldOffsets()
		for j, off := range offs {
			if c.base+uintptr(off) != addr {
				continue
			}
			ft := c.v.t.fieldT(j)
			if ft.cn() != t && ft != t {
				continue
			}
			return mkVM(ft, &cells[j], flagAddr), true
		}
	}
	return RV{}, false
}

// unsafeCellAt is unsafeField for interpreted code: the program converts the
// uintptr of Value.UnsafeAddr back to an unsafe.Pointer (openai-go's
// apijson: NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())) on an unexported
// field). It returns the remembered VM cell of the machine m at address
// addr, or the cell of a field of a remembered struct at that address, with
// its type. The ring is shared by all the machines: a cell of another
// plugin is never given, whatever address the program makes up.
func unsafeCellAt(m *Machine, addr uintptr) (*Value, types.Type, bool) {
	unsafeMu.Lock()
	defer unsafeMu.Unlock()
	for i := range unsafeRing {
		c := unsafeRing[i]
		if c.v.t == nil || c.v.t.t == nil || c.v.t.vr == nil || c.v.mach() != m || addr < c.base {
			continue
		}
		if addr == c.base {
			return c.v.p, c.v.t.t, true
		}
		cells, ok := c.v.p.R.([]Value)
		if !ok || c.v.t.kind != reflect.Struct {
			continue
		}
		for j, off := range c.v.t.fieldOffsets() {
			if c.base+uintptr(off) == addr {
				return &cells[j], c.v.t.fieldT(j).t, true
			}
		}
	}
	return nil, nil, false
}
