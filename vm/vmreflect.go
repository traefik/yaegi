package vm

// VM-internal implementation of package reflect (and internal/reflectlite).
//
// When stdlib packages that use reflect (encoding/json, fmt, errors, sort)
// are interpreted from source, their calls to reflect are not sent to the
// native reflect package (which cannot see interpreted types: the "reflect
// wall") but to the intrinsics below, which operate directly on VM values and
// go/types types. Interpreted types therefore keep their methods
// (MarshalJSON, String...) and their identity.
//
// Representation inside the VM:
//   - reflect.Type value: interface whose dynamic type is vr.typeRT and payload *vtype
//     (one canonical *vtype per types.Type, so pointer equality is type identity);
//   - reflect.Value: its own 3-field struct layout []Value{ {R:*vtype}, {R:*Value cell}, {N:flags} };
//   - *reflect.MapIter: pointer cell holding a *vmapIter.
//
// Only the API used by encoding/json, fmt, errors, sort and internal/fmtsort
// is implemented (listed by cmd/reflscan); anything else is a CompileError.

import (
	"go/types"
	"math"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"
)

type vmReflect struct {
	m      *Machine
	typeRT *RType
	vtypes typeutil.Map   // types.Type -> *vtype
	sf     map[string]int // reflect.StructField field name -> index
	nsf    int
	ifaces map[reflect.Type]*types.Interface // native interface -> VM interface
	pkgs   map[string]*types.Package         // packages of the program by path (typesPkg)
}

// The flags of a reflect.Value of the VM (the third cell of its encoding,
// the flag field of reflect.Value). They have the values of gc's reflect
// (src/reflect/value.go): code that reads the flag field with unsafe sees the
// bits it expects. go-spew (bypass.go) checks at init that the read-only bits
// are 1<<5|1<<6 and the addressable bit is 1<<8, and later clears the first
// ones and sets the last one to print unexported fields. The kind bits and
// flagIndir of gc are not kept.
const (
	// flagRO: the value comes from an unexported field (gc's flagStickyRO).
	flagRO int64 = 1 << 5
	// flagEmbedRO: with flagRO, the value is an unexported embedded field.
	// As gc's flagEmbedRO, it does not stick: the exported fields of that
	// struct are not read-only (Field). Every other derived value (Elem,
	// Index...) keeps only flagRO, which sticks.
	flagEmbedRO int64 = 1 << 6
	flagAddr    int64 = 1 << 8
	// flagROMem: a byte of a string in read-only memory (roByte): it can
	// be read, a Set is refused (in gc it stops the process). gc has no such
	// bit: 1<<10 is above the bits of gc.
	flagROMem int64 = 1 << 10
)

// fieldFlag is the flag of the field f of a struct value of flag fl.
func fieldFlag(fl int64, f *types.Var) int64 {
	out := fl & flagAddr
	if fl&flagRO != 0 && fl&flagEmbedRO == 0 {
		out |= flagRO
	}
	switch {
	case f.Exported():
	case f.Embedded() && out&flagRO == 0:
		out |= flagRO | flagEmbedRO
	default:
		out |= flagRO
	}
	return out
}

type vtype struct {
	t    types.Type
	kind reflect.Kind
	ifc  *Iface
	vr   *vmReflect
	nat  reflect.Type // native type (t == nil, vr == nil): canonical, shared by all machines

	// Optimization 3: memoized derived types (avoid typeutil.Map + mutex).
	elem   atomic.Pointer[vtype]
	ptr    atomic.Pointer[vtype]
	fields atomic.Pointer[[]*vtype]
	offs   atomic.Pointer[[]int64] // struct field offsets (gc sizes)
	// canon is the descriptor exposed by the Go-level twin (twin*.go): the
	// native type when t has a native counterpart, vt itself otherwise.
	canon atomic.Pointer[vtype]
	// methods: exported methods of t sorted by name (twin Method/MethodByName).
	methods atomic.Pointer[[]*types.Selection]
	// caches holds the entries of the AOT type caches (TypeCache) for this
	// VM type: they go away with the machine.
	caches sync.Map // *TypeCache -> any
}

func (vt *vtype) elemT() *vtype {
	if e := vt.elem.Load(); e != nil && optMemo {
		return e
	}
	var et types.Type
	switch u := vt.t.Underlying().(type) {
	case *types.Pointer:
		et = u.Elem()
	case *types.Slice:
		et = u.Elem()
	case *types.Array:
		et = u.Elem()
	case *types.Map:
		et = u.Elem()
	case *types.Chan:
		et = u.Elem()
	default:
		panic("reflect: Elem of invalid type " + vt.t.String())
	}
	e := vt.vr.vtypeOf(et)
	vt.elem.Store(e)
	return e
}

func (vt *vtype) ptrT() *vtype {
	if p := vt.ptr.Load(); p != nil && optMemo {
		return p
	}
	p := vt.vr.vtypeOf(types.NewPointer(vt.t))
	vt.ptr.Store(p)
	return p
}

func (vt *vtype) fieldT(i int) *vtype {
	if fs := vt.fields.Load(); fs != nil && optMemo {
		return (*fs)[i]
	}
	st := vt.t.Underlying().(*types.Struct)
	fs := make([]*vtype, st.NumFields())
	for j := range fs {
		fs[j] = vt.vr.vtypeOf(st.Field(j).Type())
	}
	vt.fields.Store(&fs)
	return fs[i]
}

// fieldOffsets returns the offsets of the fields of the struct type vt, with
// gc's sizes (computed once per type).
func (vt *vtype) fieldOffsets() []int64 {
	if o := vt.offs.Load(); o != nil {
		return *o
	}
	st := vt.structT()
	fs := make([]*types.Var, st.NumFields())
	for i := range fs {
		fs[i] = st.Field(i)
	}
	o := sizes.Offsetsof(fs)
	vt.offs.Store(&o)
	return o
}

func isReflectPkg(p *types.Package) bool {
	return p != nil && (p.Path() == "reflect" || p.Path() == "internal/reflectlite")
}

func (m *Machine) setupReflect() {
	routes := m.setupAOTRoutes()
	m.progVR = m.OpaqueAny || m.ownReflect
	if !m.progVR && !routes {
		return
	}
	vr := &vmReflect{m: m, sf: map[string]int{}}
	vr.typeRT = &RType{T: types.Typ[types.UnsafePointer], Methods: map[string]*Func{}}
	if rp := m.prog.ImportedPackage("reflect"); rp != nil {
		if obj := rp.Pkg.Scope().Lookup("StructField"); obj != nil {
			st := obj.Type().Underlying().(*types.Struct)
			for i := 0; i < st.NumFields(); i++ {
				vr.sf[st.Field(i).Name()] = i
			}
			vr.nsf = st.NumFields()
		}
	}
	m.vr = vr
}

func (vr *vmReflect) vtypeOf(t types.Type) *vtype {
	m := vr.m
	m.mu.Lock()
	defer m.mu.Unlock()
	if vt, ok := vr.vtypes.At(t).(*vtype); ok {
		return vt
	}
	vt := &vtype{t: t, kind: kindOf(t), vr: vr}
	vt.ifc = &Iface{T: vr.typeRT, V: Value{R: vt}}
	vr.vtypes.Set(t, vt)
	return vt
}

func kindOf(t types.Type) reflect.Kind {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Bool:
			return reflect.Bool
		case types.Int:
			return reflect.Int
		case types.Int8:
			return reflect.Int8
		case types.Int16:
			return reflect.Int16
		case types.Int32:
			return reflect.Int32
		case types.Int64:
			return reflect.Int64
		case types.Uint:
			return reflect.Uint
		case types.Uint8:
			return reflect.Uint8
		case types.Uint16:
			return reflect.Uint16
		case types.Uint32:
			return reflect.Uint32
		case types.Uint64:
			return reflect.Uint64
		case types.Uintptr:
			return reflect.Uintptr
		case types.Float32:
			return reflect.Float32
		case types.Float64:
			return reflect.Float64
		case types.Complex64:
			return reflect.Complex64
		case types.Complex128:
			return reflect.Complex128
		case types.String:
			return reflect.String
		case types.UnsafePointer:
			return reflect.UnsafePointer
		}
	case *types.Pointer:
		return reflect.Pointer
	case *types.Slice:
		return reflect.Slice
	case *types.Array:
		return reflect.Array
	case *types.Map:
		return reflect.Map
	case *types.Chan:
		return reflect.Chan
	case *types.Signature:
		return reflect.Func
	case *types.Interface:
		return reflect.Interface
	case *types.Struct:
		return reflect.Struct
	}
	return reflect.Invalid
}

// ---- reflect.Value encoding ----

type rval struct {
	t    *vtype
	p    *Value
	flag int64
	// b is the byte of a Go []byte (bytes.go) that the value is: an
	// element of a []byte, or the target of a *byte into one. Its ptr word
	// holds b itself; p is then a new cell with a copy of the byte, read
	// from b at each decV (a read never writes a cell that goroutines may
	// share), and the setters write b too (store).
	b *byte
}

func decV(v Value) rval {
	s, _ := v.R.([]Value)
	if len(s) < 3 {
		return rval{}
	}
	t, _ := s[0].R.(*vtype)
	x := rval{t: t, flag: s[2].N}
	switch p := s[1].R.(type) {
	case *Value:
		x.p = p
	case *byte:
		x.b = p
		x.p = &Value{N: int64(*p)} // the byte now, as gc reads memory
	}
	return x
}

// encByte is encV for the byte b of a Go []byte (see rval.b). The value
// has the three words of a reflect.Value, like every other one: a copy of
// the value (a store into a variable, a struct field, an interface) is a
// deepCopy of a struct of three fields.
func encByte(t *vtype, b *byte, flag int64) Value {
	return Value{R: []Value{{R: t}, {R: b}, {N: flag}}}
}

// store writes v into the settable x: its cell, and its byte when x is a
// byte of a Go []byte. A restricted plugin may not write a byte of the
// host (hostmem.go).
func (vr *vmReflect) store(x rval, v Value) {
	if x.b != nil {
		vr.m.hostBytes(unsafe.Slice(x.b, 1), 1)
		*x.b = byte(v.N)
	}
	if x.t != nil {
		storeCell(x.p, v, x.t.t) // a racy reader must not see half of a pointer
		return
	}
	*x.p = v
}

func encV(t *vtype, p *Value, flag int64) Value {
	if t == nil {
		return Value{R: []Value{{}, {}, {}}}
	}
	return Value{R: []Value{{R: t}, {R: p}, {N: flag}}}
}

func (vr *vmReflect) typeVal(t types.Type) Value {
	if t == nil {
		return Value{}
	}
	return Value{R: vr.vtypeOf(t).ifc}
}

func argType(v Value) *vtype {
	iv, _ := v.R.(*Iface)
	if iv == nil {
		return nil
	}
	vt, _ := iv.V.R.(*vtype)
	return vt
}

func (vr *vmReflect) zeroCell(t types.Type) *Value {
	c := new(Value)
	*c = deepCopy(vr.m.zero(t), t)
	return c
}

// valueOfIface implements reflect.ValueOf(i).
func (vr *vmReflect) valueOfIface(v Value) Value {
	iv, _ := v.R.(*Iface)
	if iv == nil {
		return encV(nil, nil, 0)
	}
	if iv.T.Native != nil {
		return vr.natValueOfIface(iv.V.R.(reflect.Value))
	}
	return encV(vr.vtypeOf(iv.T.T), ifaceCell(iv.V), 0)
}

// ifaceOf implements v.Interface() (as an *Iface; nil for a nil interface).
func (vr *vmReflect) ifaceOf(x rval) *Iface {
	if x.t == nil {
		panic(&reflect.ValueError{Method: "reflect.Value.Interface", Kind: reflect.Invalid})
	}
	if x.t.kind == reflect.Interface {
		iv, _ := x.p.R.(*Iface)
		return iv
	}
	return &Iface{T: vr.m.rtypeOf(x.t.t), V: deepCopy(*x.p, x.t.t)}
}

func isNilVal(x rval) bool {
	switch r := x.p.R.(type) {
	case nil:
		return true
	case *Value:
		return r == nil
	case []Value:
		return r == nil
	case *MapV:
		return r == nil
	case *Iface:
		return r == nil
	case *Closure:
		return r == nil
	case chan Value:
		return r == nil
	case reflect.Value:
		switch r.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface, reflect.Chan:
			return r.IsNil()
		}
	}
	return false
}

func (vr *vmReflect) isZero(v Value, t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Info()&types.IsString != 0 {
			s, _ := v.R.(string)
			return s == ""
		}
		return v.N == 0
	case *types.Struct:
		fs, _ := v.R.([]Value)
		for i := range fs {
			if !vr.isZero(fs[i], u.Field(i).Type()) {
				return false
			}
		}
		return true
	case *types.Array:
		es, _ := v.R.([]Value)
		for i := range es {
			if !vr.isZero(es[i], u.Elem()) {
				return false
			}
		}
		return true
	}
	return isNilVal(rval{p: &v})
}

func bitsOf(t types.Type) int {
	b := basic(t)
	if b == nil {
		unsupported("vm reflect: Bits of %s", t)
	}
	switch b.Kind() {
	case types.Int8, types.Uint8:
		return 8
	case types.Int16, types.Uint16:
		return 16
	case types.Int32, types.Uint32, types.Float32:
		return 32
	case types.Int, types.Uint, types.Uintptr:
		return intSize
	case types.Complex128:
		return 128
	}
	return 64
}

func overflowInt(t types.Type, x int64) bool {
	bs := uint(bitsOf(t))
	tr := (x << (64 - bs)) >> (64 - bs)
	return x != tr
}

func overflowUint(t types.Type, x uint64) bool {
	bs := uint(bitsOf(t))
	tr := (x << (64 - bs)) >> (64 - bs)
	return x != tr
}

func overflowFloat(t types.Type, x float64) bool {
	if bitsOf(t) == 32 {
		if x < 0 {
			x = -x
		}
		return math.MaxFloat32 < x && x <= math.MaxFloat64
	}
	return false
}

func (vr *vmReflect) numMethod(t types.Type) int {
	if it, ok := t.Underlying().(*types.Interface); ok {
		return it.NumMethods()
	}
	n := 0
	for _, sel := range vr.m.methodSet(t) {
		if sel.Obj().Exported() {
			n++
		}
	}
	return n
}

func (vr *vmReflect) structField(vt *vtype, i int) Value {
	st := vt.structT()
	f := st.Field(i)
	out := make([]Value, vr.nsf)
	for k := range out {
		out[k] = Value{}
	}
	out[vr.sf["Name"]] = Value{R: f.Name()}
	pkgPath := ""
	if !f.Exported() && f.Pkg() != nil {
		pkgPath = reflectPkgPath(f.Pkg())
	}
	out[vr.sf["PkgPath"]] = Value{R: pkgPath}
	out[vr.sf["Type"]] = vr.typeVal(f.Type())
	out[vr.sf["Tag"]] = Value{R: st.Tag(i)}
	out[vr.sf["Offset"]] = Value{N: vt.fieldOffsets()[i]}
	out[vr.sf["Index"]] = Value{R: []Value{{N: int64(i)}}}
	out[vr.sf["Anonymous"]] = Value{N: b2i(f.Embedded())}
	return Value{R: out}
}

// ---- reflect.Type methods (interface calls) ----

func (vr *vmReflect) invoke(vt *vtype, name string, regs []Value, args []int32) Value {
	if vt.isNat() || natTypeArg(name, regs, args) {
		return vr.twinInvoke(vt, name, regs, args) // native descriptor (vmreflect_nat.go)
	}
	t := vt.t
	switch name {
	case "Kind":
		return Value{N: int64(vt.kind)}
	case "Elem":
		switch u := t.Underlying().(type) {
		case *types.Pointer:
			return vr.typeVal(u.Elem())
		case *types.Slice:
			return vr.typeVal(u.Elem())
		case *types.Array:
			return vr.typeVal(u.Elem())
		case *types.Map:
			return vr.typeVal(u.Elem())
		case *types.Chan:
			return vr.typeVal(u.Elem())
		}
		panic("reflect: Elem of invalid type " + vt.String())
	case "Key":
		return vr.typeVal(t.Underlying().(*types.Map).Key())
	case "Len":
		return Value{N: t.Underlying().(*types.Array).Len()}
	case "NumField":
		return Value{N: int64(t.Underlying().(*types.Struct).NumFields())}
	case "Field":
		return vr.structField(vt, int(regs[args[0]].N))
	case "Name":
		return Value{R: vt.Name()} // twin: reflect's format (type arguments)
	case "PkgPath":
		return Value{R: vt.PkgPath()}
	case "String":
		return Value{R: vt.String()} // twin: reflect's format (no parameter names)
	case "NumMethod":
		return Value{N: int64(vr.numMethod(t))}
	case "Implements":
		u := argType(regs[args[0]])
		if u == nil {
			panic("reflect: nil type passed to Type.Implements")
		}
		it, ok := u.t.Underlying().(*types.Interface)
		if !ok {
			panic("reflect: non-interface type passed to Type.Implements")
		}
		return Value{N: b2i(vr.m.typesImplements(t, it))}
	case "AssignableTo":
		return Value{N: b2i(types.AssignableTo(t, argType(regs[args[0]]).t))}
	case "ConvertibleTo":
		return Value{N: b2i(types.ConvertibleTo(t, argType(regs[args[0]]).t))}
	case "Comparable":
		return Value{N: b2i(types.Comparable(t))}
	case "Bits":
		return Value{N: int64(bitsOf(t))}
	case "OverflowInt":
		return Value{N: b2i(overflowInt(t, regs[args[0]].N))}
	case "OverflowUint":
		return Value{N: b2i(overflowUint(t, uint64(regs[args[0]].N)))}
	case "OverflowFloat":
		return Value{N: b2i(overflowFloat(t, f64(regs[args[0]])))}
	case "Size":
		return Value{N: sizes.Sizeof(t)}
	}
	return vr.twinInvoke(vt, name, regs, args)
}

// ---- static calls: functions and methods of reflect.Value & co ----

type vmapIter struct {
	keys []any
	m    *MapV
	t    types.Type
	i    int
	cur  any
	nat  *reflect.MapIter // iterator of a native map (vmreflect_nat.go)
}

// intrinsic1 returns the VM implementation of a call to a reflect function.
func (vr *vmReflect) intrinsic1(f *ssa.Function, args []int32) func([]Value) Value {
	m := vr.m
	name := f.Name()
	if o := f.Origin(); o != nil {
		name = o.Name()
	}
	if recv := f.Signature.Recv(); recv != nil {
		rt := recv.Type()
		ptr := ""
		if p, ok := rt.(*types.Pointer); ok {
			rt, ptr = p.Elem(), "*"
		}
		name = ptr + rt.(*types.Named).Obj().Name() + "." + name
	}
	a := func(regs []Value, i int) Value { return regs[args[i]] }
	v0 := func(regs []Value) rval { return decV(regs[args[0]]) }
	switch name {
	// -- package functions --
	case "TypeOf":
		return func(regs []Value) Value {
			iv, _ := a(regs, 0).R.(*Iface)
			if iv == nil {
				return Value{}
			}
			if iv.T.Native != nil {
				return vr.natTypeVal(iv.T.Native)
			}
			return vr.typeVal(iv.T.T)
		}
	case "TypeFor":
		t := vr.typeVal(f.TypeArgs()[0])
		return func([]Value) Value { return t }
	case "ValueOf":
		return func(regs []Value) Value { return vr.valueOfIface(a(regs, 0)) }
	case "New":
		return func(regs []Value) Value {
			t := argType(a(regs, 0))
			return encV(vr.vtypeOf(types.NewPointer(t.t)), &Value{R: vr.zeroCell(t.t)}, 0)
		}
	case "NewAt":
		// reflect.NewAt(t, p): a Value of type *t that is the pointer p
		// itself. p is an interpreted pointer (a cell, a view of bytes, a
		// native pointer), never made from an integer: a pointer outside
		// the memory of the VM (badAddr) stays refused, and a cell of
		// another shape makes the reflect operations panic, not read
		// memory (protobuf's impl.pointer.AsValueOf).
		return func(regs []Value) Value {
			t := argType(a(regs, 0))
			p := a(regs, 1)
			switch q := p.R.(type) {
			case badAddr:
				q.refuse()
			case nil, *Value, *byteView, reflect.Value, *byte:
			case *hdrView:
				// a string or slice variable reached by an offset: its cell
				// when t is its type
				if p = q.as(t.t); func() bool { _, ok := p.R.(*Value); return !ok }() {
					unsupported("vm reflect: reflect.NewAt of a %s as a %s", q.src, t.t)
				}
			default:
				unsupported("vm reflect: reflect.NewAt of a %T", p.R)
			}
			// a struct and its first field (protobuf's messageState):
			// the cell of the type t at that address
			p = m.resolveAt(p, t.t)
			return encV(vr.vtypeOf(types.NewPointer(t.t)), &p, 0)
		}
	case "Zero":
		return func(regs []Value) Value {
			t := argType(a(regs, 0))
			return encV(t, vr.zeroCell(t.t), 0)
		}
	case "PointerTo", "PtrTo":
		return func(regs []Value) Value { return vr.typeVal(types.NewPointer(argType(a(regs, 0)).t)) }
	case "MakeSlice":
		return func(regs []Value) Value {
			t := argType(a(regs, 0))
			n, c := int(a(regs, 1).N), int(a(regs, 2).N)
			return encV(t, &Value{R: vr.makeSlice(t.t.Underlying().(*types.Slice).Elem(), n, c)}, 0)
		}
	case "MakeMap", "MakeMapWithSize":
		return func(regs []Value) Value {
			t := argType(a(regs, 0))
			if t.kind != reflect.Map {
				panic("reflect.MakeMapWithSize of non-map type")
			}
			return encV(t, &Value{R: newMapV(0)}, 0)
		}
	case "TypeAssert":
		target := f.TypeArgs()[0]
		it, isIface := target.Underlying().(*types.Interface)
		zero := m.zero(target)
		return func(regs []Value) Value {
			x := v0(regs)
			iv := vr.ifaceOf(x)
			ok := false
			out := zero
			if iv != nil {
				if isIface {
					if ok = m.implements(iv.T, it); ok {
						out = Value{R: iv}
					}
				} else if iv.T.T != nil && types.Identical(iv.T.T, target) {
					ok, out = true, iv.V
				}
			}
			return Value{R: []Value{out, {N: b2i(ok)}}}
		}
	case "Swapper":
		return func(regs []Value) Value {
			x := decV(vr.valueOfIface(a(regs, 0)))
			if x.t == nil || x.t.kind != reflect.Slice {
				// As gc: a nil or a struct gave a swapper of its cells.
				kind := reflect.Invalid
				if x.t != nil {
					kind = x.t.kind
				}
				panic(&reflect.ValueError{Method: "Swapper", Kind: kind})
			}
			if nv, ok := natViewOf(x.p.R); ok {
				return Value{R: reflect.ValueOf(reflect.Swapper(nv.Interface()))}
			}
			if bs, ok := x.p.R.([]byte); ok {
				m := vr.m
				return Value{R: reflect.ValueOf(func(i, j int) {
					m.hostBytes(bs, min(len(bs), 1))
					bs[i], bs[j] = bs[j], bs[i]
				})}
			}
			s, _ := x.p.R.([]Value)
			if wordKind(x.t.elemT().t) {
				return Value{R: reflect.ValueOf(func(i, j int) { swapWords(s, i, j) })}
			}
			return Value{R: reflect.ValueOf(func(i, j int) { s[i], s[j] = s[j], s[i] })}
		}

	// -- StructTag / StructField --
	case "StructTag.Get":
		return func(regs []Value) Value {
			s, _ := a(regs, 0).R.(string)
			k, _ := a(regs, 1).R.(string)
			return Value{R: reflect.StructTag(s).Get(k)}
		}
	case "StructTag.Lookup":
		return func(regs []Value) Value {
			s, _ := a(regs, 0).R.(string)
			k, _ := a(regs, 1).R.(string)
			r, ok := reflect.StructTag(s).Lookup(k)
			return Value{R: []Value{{R: r}, {N: b2i(ok)}}}
		}
	case "StructField.IsExported":
		i := vr.sf["PkgPath"]
		return func(regs []Value) Value {
			s, _ := a(regs, 0).R.([]Value)
			p, _ := s[i].R.(string)
			return Value{N: b2i(p == "")}
		}
	case "Method.IsExported":
		// reflect.Method: Name, PkgPath, Type, Func, Index
		return func(regs []Value) Value {
			s, _ := a(regs, 0).R.([]Value)
			p, _ := s[1].R.(string)
			return Value{N: b2i(p == "")}
		}
	case "Kind.String":
		return func(regs []Value) Value { return Value{R: reflect.Kind(a(regs, 0).N).String()} }

	// -- MapIter --
	case "*MapIter.Next":
		return func(regs []Value) Value {
			it := a(regs, 0).R.(*Value).R.(*vmapIter)
			if it.nat != nil {
				return Value{N: b2i(it.nat.Next())}
			}
			for it.i < len(it.keys) {
				k := it.keys[it.i]
				it.i++
				if _, ok := it.m.M[k]; ok {
					it.cur = k
					return Value{N: 1}
				}
			}
			return Value{}
		}
	case "*MapIter.Key", "*MapIter.Value":
		isKey := name == "*MapIter.Key"
		return func(regs []Value) Value {
			return vr.iterEntry(a(regs, 0).R.(*Value).R.(*vmapIter), isKey)
		}
	}

	if !strings.HasPrefix(name, "Value.") {
		return vr.twinIntrinsic(f, name, args)
	}
	switch strings.TrimPrefix(name, "Value.") {
	case "Kind":
		return func(regs []Value) Value {
			x := v0(regs)
			if x.t == nil {
				return Value{}
			}
			return Value{N: int64(x.t.kind)}
		}
	case "Type":
		return func(regs []Value) Value {
			x := v0(regs)
			if x.t == nil {
				panic(&reflect.ValueError{Method: "reflect.Value.Type", Kind: reflect.Invalid})
			}
			return Value{R: x.t.ifc}
		}
	case "IsValid":
		return func(regs []Value) Value { return Value{N: b2i(v0(regs).t != nil)} }
	case "Elem":
		return func(regs []Value) Value {
			x := v0(regs)
			switch x.t.kind {
			case reflect.Pointer:
				if b, ok := x.p.R.(roByte); ok {
					// a copy of a byte of read-only memory (see RV.Elem)
					return encV(vr.vtypeOf(x.t.t.Underlying().(*types.Pointer).Elem()), &Value{N: int64(*b)}, flagROMem)
				}
				if bv, ok := x.p.R.(*byteView); ok {
					// plain data in real bytes (byteview.go): a copy, not
					// addressable, as in RV.Elem
					et := x.t.t.Underlying().(*types.Pointer).Elem()
					c := bv.load(et)
					return encV(vr.vtypeOf(et), &c, flagROMem)
				}
				if b, ok := x.p.R.(*byte); ok && b != nil {
					// a byte of a Go []byte (bytes.go)
					return encByte(vr.vtypeOf(x.t.t.Underlying().(*types.Pointer).Elem()), b, flagAddr|x.flag&flagRO)
				}
				p, _ := x.p.R.(*Value)
				if p == nil {
					return encV(nil, nil, 0)
				}
				return encV(vr.vtypeOf(x.t.t.Underlying().(*types.Pointer).Elem()), p, flagAddr|x.flag&flagRO)
			case reflect.Interface:
				iv, _ := x.p.R.(*Iface)
				if iv == nil {
					return encV(nil, nil, 0)
				}
				if iv.T.Native != nil {
					v := decV(vr.natValueOfIface(iv.V.R.(reflect.Value)))
					return encV(v.t, v.p, x.flag&flagRO)
				}
				return encV(vr.vtypeOf(iv.T.T), ifaceCell(iv.V), x.flag&flagRO)
			}
			panic(&reflect.ValueError{Method: "reflect.Value.Elem", Kind: x.t.kind})
		}
	case "Field":
		return func(regs []Value) Value {
			x := v0(regs)
			i := int(a(regs, 1).N)
			mustBe(x, "Field", func(k reflect.Kind) bool { return k == reflect.Struct })
			st := x.t.t.Underlying().(*types.Struct)
			if uint(i) >= uint(st.NumFields()) {
				panic("reflect: Field index out of range")
			}
			fs := x.p.R.([]Value)
			fl := fieldFlag(x.flag, st.Field(i))
			return encV(vr.vtypeOf(st.Field(i).Type()), &fs[i], fl)
		}
	case "NumField":
		return func(regs []Value) Value {
			x := v0(regs)
			mustBe(x, "NumField", func(k reflect.Kind) bool { return k == reflect.Struct })
			return Value{N: int64(x.t.t.Underlying().(*types.Struct).NumFields())}
		}
	case "NumMethod":
		return func(regs []Value) Value { return Value{N: int64(vr.numMethod(v0(regs).t.t))} }
	case "Index":
		return func(regs []Value) Value {
			x := v0(regs)
			i := int(a(regs, 1).N)
			mustBe(x, "Index", func(k reflect.Kind) bool {
				return k == reflect.Slice || k == reflect.Array || k == reflect.String
			})
			if n := vr.lenOf(x); uint(i) >= uint(n) && n >= 0 {
				panic("reflect: " + strings.ToLower(x.t.kind.String()) + " index out of range")
			}
			switch u := x.t.t.Underlying().(type) {
			case *types.Slice:
				if z, ok := x.p.R.(*zslice); ok {
					e := z.addr(int64(i)).R.(*Value) // an element has no state: a new cell
					return encV(vr.vtypeOf(u.Elem()), e, flagAddr|x.flag&flagRO)
				}
				if nv, ok := natViewOf(x.p.R); ok {
					// a native slice view (natslice.go): a copy of the
					// element, not addressable (a Set panics, it is not lost)
					e := natScalar(nv.Index(i))
					return encV(vr.vtypeOf(u.Elem()), &e, x.flag&flagRO)
				}
				if bs, ok := x.p.R.([]byte); ok {
					if uint(i) >= uint(len(bs)) {
						panic("reflect: slice index out of range")
					}
					return encByte(vr.vtypeOf(u.Elem()), &bs[i], flagAddr|x.flag&flagRO)
				}
				s := x.p.R.([]Value)
				return encV(vr.vtypeOf(u.Elem()), &s[i], flagAddr|x.flag&flagRO)
			case *types.Array:
				s := x.p.R.([]Value)
				return encV(vr.vtypeOf(u.Elem()), &s[i], x.flag&(flagAddr|flagRO))
			case *types.Basic:
				if u.Info()&types.IsString == 0 {
					break
				}
				s := x.p.R.(string)
				return encV(vr.vtypeOf(types.Typ[types.Uint8]), &Value{N: int64(s[i])}, x.flag&flagRO)
			}
			if k := x.t.kind; k != reflect.Slice && k != reflect.Array && k != reflect.String {
				panic(&reflect.ValueError{Method: "reflect.Value.Index", Kind: k})
			}
			unsupported("vm reflect: Index on %s", x.t.t)
			return Value{}
		}
	case "Len":
		return func(regs []Value) Value {
			x := v0(regs)
			if x.t != nil && x.t.kind == reflect.Pointer && x.t.elemT().kind != reflect.Array {
				panic("reflect: call of reflect.Value.Len on ptr to non-array Value")
			}
			mustBe(x, "Len", func(k reflect.Kind) bool {
				switch k {
				case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String, reflect.Pointer:
					return true
				}
				return false
			})
			if x.t.kind == reflect.Pointer {
				// a pointer to an array: the length of the array type
				return Value{N: x.t.t.Underlying().(*types.Pointer).Elem().Underlying().(*types.Array).Len()}
			}
			return Value{N: int64(max(vr.lenOf(x), 0))}
		}
	case "Cap":
		return func(regs []Value) Value {
			x := v0(regs)
			k := reflect.Invalid
			if x.t != nil {
				k = x.t.kind
			}
			switch {
			case k == reflect.Array:
				return Value{N: x.t.t.Underlying().(*types.Array).Len()}
			case k == reflect.Pointer && x.t.elemT().kind == reflect.Array:
				return Value{N: x.t.t.Underlying().(*types.Pointer).Elem().Underlying().(*types.Array).Len()}
			case k == reflect.Pointer:
				panic("reflect: call of reflect.Value.Cap on ptr to non-array Value")
			case k != reflect.Slice && k != reflect.Chan:
				panic(&reflect.ValueError{Method: "reflect.Value.Cap", Kind: k})
			}
			switch r := x.p.R.(type) {
			case *zslice:
				return Value{N: int64(cap(r.s))}
			case []byte:
				return Value{N: int64(cap(r))}
			case chan Value:
				// a channel of the program (chan K of a struct K)
				return Value{N: int64(cap(r))}
			case reflect.Value:
				if r.Kind() == reflect.Slice || r.Kind() == reflect.Chan {
					return Value{N: int64(r.Cap())}
				}
			}
			s, _ := x.p.R.([]Value)
			return Value{N: int64(cap(s))}
		}
	case "Grow":
		return func(regs []Value) Value {
			x := v0(regs)
			vr.mustSet(x, "Grow")
			n := int(a(regs, 1).N)
			s, _ := x.p.R.([]Value)
			if cap(s)-len(s) < n {
				nc := max(2*cap(s), len(s)+n, 4)
				et := x.t.t.Underlying().(*types.Slice).Elem()
				ns := vr.makeSlice(et, len(s), nc)
				growValues(ns, s, et)
				*x.p = Value{R: ns}
			}
			return Value{}
		}
	case "SetLen":
		return func(regs []Value) Value {
			x := v0(regs)
			vr.mustSet(x, "SetLen")
			s, _ := x.p.R.([]Value)
			*x.p = Value{R: s[:a(regs, 1).N]}
			return Value{}
		}
	case "Interface":
		return func(regs []Value) Value {
			x := v0(regs)
			if x.flag&flagRO != 0 {
				panic("reflect.Value.Interface: cannot return value obtained from unexported field or method")
			}
			iv := vr.ifaceOf(x)
			if iv == nil {
				return Value{}
			}
			return Value{R: iv}
		}
	case "CanInterface":
		return func(regs []Value) Value { return Value{N: b2i(v0(regs).flag&flagRO == 0)} }
	case "CanAddr":
		return func(regs []Value) Value { return Value{N: b2i(v0(regs).flag&flagAddr != 0)} }
	case "CanSet":
		return func(regs []Value) Value {
			x := v0(regs)
			return Value{N: b2i(x.flag&flagAddr != 0 && x.flag&flagRO == 0)}
		}
	case "Addr":
		return func(regs []Value) Value {
			x := v0(regs)
			if x.flag&flagAddr == 0 {
				panic("reflect.Value.Addr of unaddressable value")
			}
			if x.b != nil {
				return encV(vr.vtypeOf(types.NewPointer(x.t.t)), &Value{R: x.b}, x.flag&flagRO)
			}
			return encV(vr.vtypeOf(types.NewPointer(x.t.t)), &Value{R: x.p}, x.flag&flagRO)
		}
	case "IsNil":
		return func(regs []Value) Value { return Value{N: b2i(isNilVal(v0(regs)))} }
	case "IsZero":
		return func(regs []Value) Value { x := v0(regs); return Value{N: b2i(vr.isZero(*x.p, x.t.t))} }
	case "Bool":
		return func(regs []Value) Value { return Value{N: v0(regs).p.N} }
	case "Int", "Uint", "Float":
		return func(regs []Value) Value { return Value{N: v0(regs).p.N} }
	case "String":
		return func(regs []Value) Value {
			x := v0(regs)
			if x.t == nil {
				return Value{R: "<invalid Value>"}
			}
			if x.t.kind == reflect.String {
				return Value{R: x.p.R}
			}
			return Value{R: "<" + x.t.String() + " Value>"}
		}
	case "Bytes":
		return func(regs []Value) Value { return *v0(regs).p }
	case "Pointer", "UnsafePointer":
		return func(regs []Value) Value { return m.vrPointer(v0(regs)) }
	case "OverflowInt":
		return func(regs []Value) Value { return Value{N: b2i(overflowInt(v0(regs).t.t, a(regs, 1).N))} }
	case "OverflowUint":
		return func(regs []Value) Value { return Value{N: b2i(overflowUint(v0(regs).t.t, uint64(a(regs, 1).N)))} }
	case "OverflowFloat":
		return func(regs []Value) Value { return Value{N: b2i(overflowFloat(v0(regs).t.t, f64(a(regs, 1))))} }
	case "Set":
		return func(regs []Value) Value {
			x, y := v0(regs), decV(a(regs, 1))
			vr.mustSet(x, "Set")
			if y.t == nil {
				panic(&reflect.ValueError{Method: "reflect.Value.Set", Kind: reflect.Invalid})
			}
			if y.flag&flagRO != 0 {
				panic("reflect: reflect.Value.Set using value obtained using unexported field")
			}
			vr.assign(x, y, "reflect.Set")
			return Value{}
		}
	case "SetIterKey", "SetIterValue":
		// v.SetIterKey(it) is v.Set(it.Key()) without the copy of the key
		// (fxamacker/cbor's map encoder).
		op, isKey := strings.TrimPrefix(name, "Value."), name == "Value.SetIterKey"
		ctx := "reflect.MapIter.SetValue" // the name gc gives the check of the value
		if isKey {
			ctx = "reflect.MapIter.SetKey"
		}
		return func(regs []Value) Value {
			x := v0(regs)
			vr.mustSet(x, op)
			vr.assign(x, decV(vr.iterEntry(a(regs, 1).R.(*Value).R.(*vmapIter), isKey)), ctx)
			return Value{}
		}
	case "SetBool", "SetInt", "SetUint", "SetFloat":
		op := strings.TrimPrefix(name, "Value.")
		kinds := setKinds[op]
		return func(regs []Value) Value {
			x := v0(regs)
			vr.mustSet(x, op)
			mustBe(x, op, kinds)
			n := a(regs, 1).N
			if x.t.kind == reflect.Float32 {
				n = fv(float64(float32(f64(a(regs, 1)))))
			} else {
				// Cut the value to the size of the kind, like Go:
				// SetInt(300) on an int8 gives 44, and on a 32-bit
				// platform SetInt(1<<40+3) on an int gives 3.
				n = truncN(n, x.t.kind)
			}
			vr.store(x, Value{N: n})
			return Value{}
		}
	case "SetString", "SetBytes":
		op := strings.TrimPrefix(name, "Value.")
		kinds := setKinds[op]
		return func(regs []Value) Value {
			x := v0(regs)
			vr.mustSet(x, op)
			mustBe(x, op, kinds)
			if op == "SetBytes" && x.t.elemT().kind != reflect.Uint8 {
				panic("reflect.Value.SetBytes of non-byte slice")
			}
			*x.p = Value{R: a(regs, 1).R}
			return Value{}
		}
	case "SetZero":
		return func(regs []Value) Value {
			x := v0(regs)
			vr.mustSet(x, "SetZero")
			vr.store(x, deepCopy(m.zero(x.t.t), x.t.t))
			return Value{}
		}
	case "SetMapIndex":
		return func(regs []Value) Value {
			x, k, e := v0(regs), decV(a(regs, 1)), decV(a(regs, 2))
			// Like gc: the map, the key and the element may not come
			// from an unexported field; the map need not be addressable.
			for _, y := range []rval{x, k, e} {
				if y.flag&flagRO != 0 {
					panic("reflect: reflect.Value.SetMapIndex using value obtained using unexported field")
				}
			}
			mp, _ := x.p.R.(*MapV)
			if mp == nil {
				if e.t == nil {
					return Value{} // a delete from a nil map does nothing, as in gc
				}
				panic(errNilMapAssign)
			}
			mt := x.t.t.Underlying().(*types.Map)
			if k.t == nil {
				panic(&reflect.ValueError{Method: "reflect.Value.SetMapIndex", Kind: reflect.Invalid})
			}
			kv := vr.assignedVal(k, mt.Key(), "reflect.Value.SetMapIndex")
			kk := keyOf(kv, mt.Key())
			if e.t == nil {
				delete(mp.M, kk)
				mp.deleteKey(kk)
				return Value{}
			}
			kk = storeKey(kk)
			mp.M[kk] = vr.assignedVal(e, mt.Elem(), "reflect.Value.SetMapIndex")
			mp.setKey(kk, kv)
			return Value{}
		}
	case "MapIndex":
		return func(regs []Value) Value {
			x, k := v0(regs), decV(a(regs, 1))
			mp, _ := x.p.R.(*MapV)
			mt := x.t.t.Underlying().(*types.Map)
			if mp == nil {
				return encV(nil, nil, 0)
			}
			e, ok := mp.M[keyOf(*k.p, mt.Key())]
			if !ok {
				return encV(nil, nil, 0)
			}
			return encV(vr.vtypeOf(mt.Elem()), &e, 0)
		}
	case "MapRange":
		return func(regs []Value) Value {
			x := v0(regs)
			if r, ok := x.p.R.(reflect.Value); ok && x.t.kind == reflect.Map {
				return Value{R: &Value{R: &vmapIter{nat: r.MapRange()}}}
			}
			mp, _ := x.p.R.(*MapV)
			it := &vmapIter{m: mp, t: x.t.t}
			if mp != nil {
				it.keys = make([]any, 0, len(mp.M))
				for k := range mp.M {
					it.keys = append(it.keys, k)
				}
			} else {
				it.m = &MapV{}
			}
			return Value{R: &Value{R: it}}
		}
	case "MapKeys":
		return func(regs []Value) Value {
			x := v0(regs)
			mp, _ := x.p.R.(*MapV)
			mt := x.t.t.Underlying().(*types.Map)
			kt := vr.vtypeOf(mt.Key())
			var out []Value
			if mp != nil {
				for k := range mp.M {
					c := mp.key(k)
					out = append(out, encV(kt, &c, 0))
				}
			}
			return Value{R: out}
		}
	}
	return vr.twinIntrinsic(f, name, args)
}

// zeroSizeBase is the data address of the non-nil empty VM slices, like
// the runtime's zerobase: Pointer of an empty, non-nil slice is not 0.
var zeroSizeBase Value

// mustSet panics, with the message of gc, when x cannot be set by the
// reflect.Value method op ("SetInt").
//
// The panic value is a Go value, not a *VMPanic: the VM converts it to a
// program value (panicValue), so that recover gets the message. A *VMPanic
// must hold a value of the program (an interface), and a bare string there
// is seen as nil by recover.
func (vr *vmReflect) mustSet(x rval, op string) {
	if x.t == nil {
		panic(&reflect.ValueError{Method: "reflect.Value." + op, Kind: reflect.Invalid})
	}
	if x.flag&flagROMem != 0 {
		refuseROWrite()
	}
	if x.flag&flagRO != 0 {
		panic("reflect: reflect.Value." + op + " using value obtained using unexported field")
	}
	if x.flag&flagAddr == 0 {
		panic("reflect: reflect.Value." + op + " using unaddressable value")
	}
}

// lenOf is the length of the value x of kind array, chan, map, slice or
// string; -1 when its representation is not known here.
func (vr *vmReflect) lenOf(x rval) int {
	switch r := x.p.R.(type) {
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
	case reflect.Value:
		if isNatView(r) {
			return r.Len()
		}
		return -1
	case nil:
		if x.t != nil && x.t.kind == reflect.Array {
			return int(x.t.t.Underlying().(*types.Array).Len())
		}
		return 0 // a nil slice, map or chan, or an empty string
	}
	return -1
}

// kindOfRval is the kind of x, Invalid for the zero Value.
func kindOfRval(x rval) reflect.Kind {
	if x.t == nil {
		return reflect.Invalid
	}
	return x.t.kind
}

// mustBe panics, as gc's Value methods do, when the kind of x is not one
// that the method op ("SetInt") accepts.
func mustBe(x rval, op string, ok func(reflect.Kind) bool) {
	if k := kindOfRval(x); !ok(k) {
		panic(&reflect.ValueError{Method: "reflect.Value." + op, Kind: k})
	}
}

// setKinds are the kinds that the Set methods of reflect.Value accept.
var setKinds = map[string]func(reflect.Kind) bool{
	"SetBool":   func(k reflect.Kind) bool { return k == reflect.Bool },
	"SetInt":    isIntKind,
	"SetUint":   isUintKind,
	"SetFloat":  func(k reflect.Kind) bool { return k == reflect.Float32 || k == reflect.Float64 },
	"SetString": func(k reflect.Kind) bool { return k == reflect.String },
	"SetBytes":  func(k reflect.Kind) bool { return k == reflect.Slice },
}

// assign stores y into the settable x, as Value.Set does (ctx names the
// operation in the panic of a value that is not assignable).
func (vr *vmReflect) assign(x, y rval, ctx string) {
	vr.store(x, vr.assignedVal(y, x.t.t, ctx))
}

// assignedVal returns y as a value of type dst, as gc's Value.assignTo: a
// copy when the type of y is assignable to dst, boxed when dst is an
// interface. Otherwise it panics with the message of gc (ctx names the
// operation): a Set of a Rect into an interface that only *Rect
// implements stored the value, and SetMapIndex into a map of interfaces
// stored the value without its type, so that a later read found nil. The
// check is for types of the program; with a native type on either side the
// conversion decides, as before.
func (vr *vmReflect) assignedVal(y rval, dst types.Type, ctx string) Value {
	if y.t != nil && !y.t.isNat() && dst != nil && !types.AssignableTo(y.t.t, dst) {
		panic(ctx + ": value of type " + y.t.String() + " is not assignable to type " + vr.vtypeOf(dst).String())
	}
	if _, isI := dst.Underlying().(*types.Interface); isI && y.t.kind != reflect.Interface {
		return Value{R: &Iface{T: vr.m.rtypeOf(y.t.t), V: deepCopy(*y.p, y.t.t)}}
	}
	return deepCopy(*y.p, dst)
}

// iterEntry is MapIter.Key (key) or MapIter.Value of the iterator it.
func (vr *vmReflect) iterEntry(it *vmapIter, key bool) Value {
	if it.nat != nil {
		if key {
			return vr.natValueOf(it.nat.Key())
		}
		return vr.natValueOf(it.nat.Value())
	}
	mt := it.t.Underlying().(*types.Map)
	if key {
		c := it.m.key(it.cur)
		return encV(vr.vtypeOf(mt.Key()), &c, 0)
	}
	c := it.m.M[it.cur]
	return encV(vr.vtypeOf(mt.Elem()), &c, 0)
}

// makeSlice allocates a slice whose whole capacity holds zero values of elem.
func (vr *vmReflect) makeSlice(elem types.Type, n, c int) []Value {
	var memN int64
	if m := vr.m; m.mem != nil && c > 0 {
		// reflect.MakeSlice counts in the quota as make does (OpMakeSlice):
		// a huge cap is ErrMemory, not a fatal out of memory of the host
		var entries int64
		if needsCopy(elem) {
			entries = min(m.zeroEntries(elem), maxZeroElems+1)
		}
		memN = m.memMakeSlice(int64(c), entries)
	}
	s := make([]Value, n, c)
	if memN > 0 {
		vr.m.memAdd(unsafe.Pointer(unsafe.SliceData(s[:c])), memN)
	}
	z := vr.m.zero(elem)
	if needsCopy(elem) || z.R != nil {
		full := s[:c]
		for i := range full {
			full[i] = deepCopy(z, elem)
		}
	}
	return s
}

// callValue calls an interpreted func value with args.
func (m *Machine) callValue(f Value, args []Value) Value {
	defer m.resumeShadows(m.suspendShadows())
	cl, _ := f.R.(*Closure)
	if cl == nil {
		panic(errNilDeref)
	}
	if cl.Go != nil {
		return cl.Go(args)
	}
	return m.Call(cl.Fn, args, cl.Env)
}

// callGenericMethod calls fn, a method of an instance of a generic type of a
// native package (unique.Handle[T].Value), through an interface: the method
// has no native instance, genericIntrinsic implements it.
func (m *Machine) callGenericMethod(fn *ssa.Function, recv Value, regs []Value, args []int32) Value {
	g, ok := m.genMethods.Load(fn)
	if !ok {
		idx := make([]int32, len(args)+1) // the receiver is the first argument
		for i := range idx {
			idx[i] = int32(i)
		}
		g, _ = m.genMethods.LoadOrStore(fn, m.genericIntrinsic(fn, idx))
	}
	a := make([]Value, len(args)+1)
	a[0] = recv
	for i, r := range args {
		a[i+1] = regs[r]
	}
	return g.(func([]Value) Value)(a)
}

// vrPointer is reflect.Value.Pointer and UnsafePointer of x in the VM
// reflect. N is the address, and R is the pointer the program gets back
// with unsafe: the object itself, or a pointer to the first element of a
// slice. unsafe.Pointer(v.Pointer()) gives back the object from R
// (uintptrToUnsafe), as gc keeps it alive in that expression.
func (m *Machine) vrPointer(x rval) Value {
	var p unsafe.Pointer
	ptr := x.p.R
	switch r := x.p.R.(type) {
	case *Value:
		p = unsafe.Pointer(r)
		// unsafe.Pointer(v.Pointer()) + off finds the fields
		// (protobuf's pointerOfValue)
		m.rememberPointee(*x.p, x.t.t)
	case *MapV:
		p = unsafe.Pointer(r)
	case *Closure:
		if r != nil && r.Fn != nil {
			p = m.funcPC(r.Fn) // like a code pointer (funcpc.go)
		} else {
			p = unsafe.Pointer(r)
		}
	case reflect.Value:
		switch r.Kind() {
		case reflect.Func, reflect.Pointer, reflect.Map, reflect.Chan, reflect.UnsafePointer:
			// native value: its own pointer
			return Value{N: int64(r.Pointer()), R: r}
		case reflect.Slice:
			if r.IsNil() {
				return Value{}
			}
			if r.Cap() == 0 {
				return Value{N: int64(r.Pointer()), R: r}
			}
			e := reflect.NewAt(r.Type().Elem(), r.UnsafePointer())
			return Value{N: int64(e.Pointer()), R: e}
		}
	case *byte, roByte, *byteView:
		// a byte of a Go []byte or of a string, or plain data in real
		// bytes: the real address, as uintptr(unsafe.Pointer(p)) gives
		// (golang.org/x/crypto/internal/alias with purego compares them)
		return Value{N: int64(addrOf(*x.p)), R: ptr}
	case []Value:
		switch {
		case cap(r) > 0:
			c := &r[:1][0]
			p, ptr = unsafe.Pointer(c), c
		case r != nil:
			p, ptr = unsafe.Pointer(&zeroSizeBase), &zeroSizeBase
		}
	case []byte:
		switch {
		case cap(r) > 0:
			c := &r[:1][0]
			p, ptr = unsafe.Pointer(c), c
		case r != nil:
			p, ptr = unsafe.Pointer(&zeroSizeBase), &zeroSizeBase
		}
	}
	return Value{N: int64(uintptr(p)), R: ptr}
}

// isVRPointer reports whether fn is the method Pointer or UnsafePointer of
// reflect.Value.
func isVRPointer(fn *ssa.Function) bool {
	if fn.Name() != "Pointer" && fn.Name() != "UnsafePointer" || fn.Pkg == nil || !isReflectPkg(fn.Pkg.Pkg) {
		return false
	}
	recv := fn.Signature.Recv()
	if recv == nil {
		return false
	}
	n, ok := types.Unalias(recv.Type()).(*types.Named)
	return ok && n.Obj().Name() == "Value"
}
