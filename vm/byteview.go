package vm

import (
	"encoding/binary"
	"go/token"
	"go/types"
	"math"
	"reflect"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// Typed views of real bytes.
//
// A []byte made by make, by a conversion or by native code is real Go
// memory (bytes.go), and so are the bytes of a string. A program that
// reads a word out of such bytes, or lays a plain struct over them
// ((*uint64)(unsafe.Pointer(&b[i])) of xxhash, the entry header of
// freecache over its buffer, the control messages of x/sys), gets a
// byteView: a window of the array at an offset, with the type the program
// sees. A load decodes the bytes in the host order, a store encodes them,
// a field or element address is a window further in. Every access is a
// bounds-checked slice access, so the view never reaches outside the
// array the VM holds, and no pointer is ever made from an integer (the
// rules of docs/UNSAFE.md).
//
// Only plain data gets a view: integers, floats, booleans, structs and
// arrays of them (isPOD). A type with a pointer, a string, a slice, a map,
// a channel, a func or an interface stays refused: a pointer word written
// as bytes would be invisible to the garbage collector. byteViewAt makes
// every view and checks this, and every conversion of a view to another
// pointer type goes through as (viewOf in unsafehdr.go, unsafeView in
// unsafe.go): a view never carries a type that is not plain data, so
// native code, which gets the real pointer (toNativeW), never reads a
// pointer word out of bytes the program wrote.
//
// A variable of plain data whose address the SSA converts to another plain
// type (a local [8]byte read as a uint64, a struct seen as bytes, a float32
// read as a uint32) is allocated in real bytes from the start (bytesAlloc).
// Other cells are not bytes: a slice of cells, or a pointer whose origin the
// SSA does not show, is still refused with "needs byte-level memory".

// bytesAlloc reports whether the variable that a allocates must live in real
// bytes: it is plain data, and the SSA converts its address (or the address
// of one of its fields or elements) through unsafe.Pointer to a pointer to
// another plain type that cells cannot show. The cases are a local [8]byte
// read as a uint64, a struct seen as [N]byte or laid over with another
// struct, a float32 read as a uint32 and a uint16 seen as [2]byte. The
// variable then is a byteView over a new Go []byte of its gc size: loads
// and stores decode and encode its fields, and the views of other types
// share its bytes (docs/UNSAFE.md). Conversions that cells can show (the
// same layout, an array and its element type) keep the cells, so atomic
// operations through (*int64)(unsafe.Pointer(&s.n)) do not change.
func (c *compiler) bytesAlloc(a *ssa.Alloc) bool {
	t := a.Type().(*types.Pointer).Elem()
	if !c.m.isPOD(t) || gcSizes.Sizeof(t) == 0 {
		return false
	}
	if c.viewedAsOther(a, t, 0) || c.uintptrToNative(a, 0) {
		return true
	}
	// The reflect of the program reads a variable in bytes through a copy
	// (docs/UNSAFE.md): reflect.ValueOf(&c).Elem().Field(i).SetInt(1) would
	// panic where gc sets the field. In a program that has its own reflect
	// (ownReflect), a variable whose own address escapes keeps its cells:
	// native code gets a copy of them and writes it back, as before. Only a
	// variable that gives away the addresses of its parts goes to bytes.
	return c.innerToNative(a, 0) && !(c.m.ownReflect && addrEscapes(a))
}

// addrEscapes reports whether the address of the whole variable at a goes
// anywhere else than to its own fields, elements, loads, stores and slices:
// an interface, a call, a closure, another variable (bytesAlloc).
func addrEscapes(a *ssa.Alloc) bool {
	for _, r := range *a.Referrers() {
		switch r := r.(type) {
		case *ssa.FieldAddr, *ssa.IndexAddr, *ssa.DebugRef:
		case *ssa.UnOp:
			if r.Op != token.MUL {
				return true
			}
		case *ssa.Store:
			if r.Addr != a {
				return true
			}
		case *ssa.Slice:
			if r.X != a {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// innerToNative reports whether the address of a field or an element of the
// variable at v (or of a field of a field...) can reach native code: it is
// put in an interface, or passed (also by go or defer) to a function that
// does not run in the VM.
// Native code gets a copy of a cell, at an address of its own, so the
// addresses of two elements were not gc's element size apart: fmt's %p of
// &a[0] and &a[1] of a [10]struct{ x uint8 } differed by 24, not 1 (GOROOT
// test fixedbugs/bug260.go). In real bytes (bytesAlloc), native code gets
// the real pointer into the variable, with gc's layout, and shares it.
func (c *compiler) innerToNative(v ssa.Value, depth int) bool {
	if depth > 8 {
		return false
	}
	for _, r := range *v.Referrers() {
		var inner ssa.Value
		switch r := r.(type) {
		case *ssa.FieldAddr:
			if r.X == v {
				inner = r
			}
		case *ssa.IndexAddr:
			if r.X == v {
				if _, ok := v.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Array); ok {
					inner = r
				}
			}
		}
		if inner == nil {
			continue
		}
		if c.innerToNative(inner, depth+1) {
			return true
		}
		for _, u := range *inner.Referrers() {
			switch u := u.(type) {
			case *ssa.MakeInterface:
				if u.X == inner {
					return true
				}
			case ssa.CallInstruction: // a call, a go or a defer statement
				if c.nativeArg(u.Common(), inner) {
					return true
				}
			}
		}
	}
	return false
}

// nativeArg reports whether the call cc runs native code and gets v as an
// argument: not code of the VM, nor a generic function that the VM runs
// itself (weak.Make, atomic.Pointer).
func (c *compiler) nativeArg(cc *ssa.CallCommon, v ssa.Value) bool {
	fn, ok := cc.Value.(*ssa.Function)
	if !ok || cc.IsInvoke() || c.m.isInterp(fn) || isVMGeneric(fn) || fn.Origin() != nil {
		return false
	}
	for _, a := range cc.Args {
		if a == v {
			return true
		}
	}
	return false
}

// uintptrToNative reports whether the address v of a variable (or of one of
// its fields or elements) is converted to a uintptr in the arguments of a
// call to native code: syscall.Syscall(SYS_CLOCK_GETTIME, 1,
// uintptr(unsafe.Pointer(&ts)), 0), rule 4 of unsafe.Pointer. The kernel or
// the native function reads and writes the memory at that address with gc's
// layout: a cell is not that memory, and a write there breaks the VM. In
// real bytes (bytesAlloc), the uintptr is the address of the bytes of the
// variable, and the program reads what the call wrote.
func (c *compiler) uintptrToNative(v ssa.Value, depth int) bool {
	if depth > 8 {
		return false
	}
	for _, r := range *v.Referrers() {
		switch r := r.(type) {
		case *ssa.FieldAddr:
			if r.X == v && c.uintptrToNative(r, depth+1) {
				return true
			}
		case *ssa.IndexAddr:
			if r.X == v && c.uintptrToNative(r, depth+1) {
				return true
			}
		case *ssa.Convert:
			if r.X == v && isUnsafePointer(r.Type()) && c.uintptrArgOfNative(r) {
				return true
			}
		}
	}
	return false
}

// uintptrArgOfNative reports whether the unsafe.Pointer p is converted to a
// uintptr that is an argument of a call to native code (uintptrToNative).
func (c *compiler) uintptrArgOfNative(p ssa.Value) bool {
	for _, u := range *p.Referrers() {
		cv, ok := u.(*ssa.Convert)
		if !ok || cv.X != p || !isUintptr(cv.Type()) {
			continue
		}
		for _, w := range *cv.Referrers() {
			if call, ok := w.(ssa.CallInstruction); ok && c.nativeArg(call.Common(), cv) {
				return true
			}
		}
	}
	return false
}

// isUintptr reports whether t is uintptr (or a type defined on it).
func isUintptr(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uintptr
}

// viewedAsOther reports whether the address v of a value of type t is
// converted through unsafe.Pointer to a pointer to a plain type of another
// layout (bytesAlloc).
func (c *compiler) viewedAsOther(v ssa.Value, t types.Type, depth int) bool {
	if depth > 8 {
		return false
	}
	for _, r := range *v.Referrers() {
		switch r := r.(type) {
		case *ssa.Store:
			// the address kept in a variable (c := &T{} captured by a
			// closure is a heap cell): follow the loads of the variable
			if r.Val == v && c.loadsViewedAsOther(r.Addr, t, depth+1) {
				return true
			}
		case *ssa.FieldAddr:
			if r.X == v {
				st := t.Underlying().(*types.Struct)
				if c.viewedAsOther(r, st.Field(r.Field).Type(), depth+1) {
					return true
				}
			}
		case *ssa.IndexAddr:
			if r.X == v {
				if at, ok := t.Underlying().(*types.Array); ok && c.viewedAsOther(r, at.Elem(), depth+1) {
					return true
				}
			}
		case *ssa.Convert:
			if !isUnsafePointer(r.Type()) {
				continue
			}
			for _, u := range *r.Referrers() {
				cv, ok := u.(*ssa.Convert)
				if !ok {
					continue
				}
				pt, ok := cv.Type().Underlying().(*types.Pointer)
				if !ok {
					continue
				}
				to := pt.Elem()
				if (c.m.isPOD(to) || c.m.nativePlainType(to) != nil) && !sameLayout(t, to) && arrayOf(to, t) == 0 && arrayOf(t, to) == 0 {
					return true
				}
			}
		}
	}
	return false
}

// isByteViewOf reports whether v is a pointer of plain data in real bytes.
func isByteViewOf(v Value) bool { _, ok := v.R.(*byteView); return ok }

// loadsViewedAsOther reports whether a value loaded from the variable cell
// (a pointer to a t), in this function or in a closure that captures the
// variable, is viewed as another type (viewedAsOther).
func (c *compiler) loadsViewedAsOther(cell ssa.Value, t types.Type, depth int) bool {
	if depth > 8 {
		return false
	}
	if _, ok := cell.(*ssa.Alloc); !ok {
		if _, ok := cell.(*ssa.FreeVar); !ok {
			return false
		}
	}
	for _, r := range *cell.Referrers() {
		switch r := r.(type) {
		case *ssa.UnOp:
			if r.Op == token.MUL && r.X == cell && c.viewedAsOther(r, t, depth+1) {
				return true
			}
		case *ssa.MakeClosure:
			fn, _ := r.Fn.(*ssa.Function)
			for i, b := range r.Bindings {
				if b == cell && fn != nil && i < len(fn.FreeVars) && c.loadsViewedAsOther(fn.FreeVars[i], t, depth+1) {
					return true
				}
			}
		}
	}
	return false
}

// byteView is a pointer to plain data inside a Go byte array.
type byteView struct {
	b   []byte     // the whole array the VM holds
	off int        // offset of the value in b
	t   types.Type // the type the program sees
	ro  bool       // b is read-only (the bytes of a string outside the heap)
}

// isPOD reports whether values of type t are plain data: bytes with no
// pointer word, no string, no interface, no complex number, and no type
// that the VM keeps in native memory.
func (m *Machine) isPOD(t types.Type) bool {
	if m.isNativeNamed(t) {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&(types.IsString|types.IsComplex|types.IsUntyped) != 0:
			return false
		case u.Kind() == types.UnsafePointer:
			return false
		}
		return u.Info()&(types.IsInteger|types.IsFloat|types.IsBoolean) != 0
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if !m.isPOD(u.Field(i).Type()) {
				return false
			}
		}
		return true
	case *types.Array:
		return m.isPOD(u.Elem())
	}
	return false
}

// byteViewAt returns the view of a *to at offset off of the bytes of r,
// or the reason why there is none.
func (m *Machine) byteViewAt(r *uregion, off int, to types.Type) (Value, string) {
	if !m.isPOD(to) {
		if rt := m.nativePlainType(to); rt != nil {
			return m.nativeBytesView(r, off, rt) // nativeview.go
		}
		return Value{}, "needs plain data (integers, floats, booleans, structs and arrays of them)"
	}
	size := gcSizes.Sizeof(to)
	if off < 0 || int64(off)+size > int64(len(r.bytes)) {
		return Value{}, "reaches outside the memory known to the VM"
	}
	return Value{R: &byteView{b: r.bytes, off: off, t: to, ro: r.ro}}, ""
}

// byteViewOf returns the pointer p to a byte of a Go byte array seen as a
// *to (plain data). orig gives the array when the SSA shows it; else the
// recent regions are searched, else the view covers one byte.
func (m *Machine) byteViewOf(p Value, r *uregion, to types.Type) (Value, bool) {
	if bv, ok := p.R.(*byteView); ok {
		return bv.as(m, to), true
	}
	if _, ok := rawByte(p); !ok {
		return Value{}, false
	}
	if r == nil || r.bytes == nil || !r.holds(p) {
		r = m.findRegionPtr(p) // the ring of the goroutine first
	}
	if r == nil || r.bytes == nil {
		r = lone(p, nil)
	}
	off, _ := r.index(p)
	v, why := m.byteViewAt(r, off, to)
	if why != "" {
		unsupported("unsafe: viewing bytes as a %s %s", to, why)
	}
	return v, true
}

// as returns the view seen as a *to: the same bytes, another plain type.
// Every retyping of a view comes here; a type that is not plain data is
// refused by byteViewAt.
func (v *byteView) as(m *Machine, to types.Type) Value {
	if types.Identical(to.Underlying(), v.t.Underlying()) {
		return Value{R: v}
	}
	if isByteType(to) {
		// unsafe.Slice((*byte)(p), n) of the view finds its whole array
		if m != nil {
			m.addRegion(v.region())
		}
		return bytePtr(v.ptr(), v.ro)
	}
	nv, why := m.byteViewAt(&uregion{bytes: v.b, ro: v.ro}, v.off, to)
	if why != "" {
		unsupported("unsafe: viewing a %s as a %s %s", v.t, to, why)
	}
	return nv
}

// ptr is the Go pointer of the first byte of the view.
func (v *byteView) ptr() *byte {
	return (*byte)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(v.b)), v.off))
}

// addr is the address the program sees.
func (v *byteView) addr() uintptr { return uintptr(unsafe.Pointer(v.ptr())) }

// bytes returns the bytes of the value, for a load or a store of type t.
func (v *byteView) bytes(t types.Type) []byte {
	n := int(gcSizes.Sizeof(t))
	if v.off+n > len(v.b) {
		unsupported("unsafe: a %s at offset %d reaches outside a %d-byte object", t, v.off, len(v.b))
	}
	return v.b[v.off : v.off+n : v.off+n]
}

// load decodes the value of type t.
func (v *byteView) load(t types.Type) Value { return decodeBytes(v.bytes(t), t) }

// store encodes x, of type t, into the bytes.
func (v *byteView) store(x Value, t types.Type) {
	if v.ro {
		refuseROWrite()
	}
	encodeBytes(v.bytes(t), x, t)
}

// field is the address of field i of the struct the view points to.
func (v *byteView) field(i int) Value {
	st := v.t.Underlying().(*types.Struct)
	return Value{R: &byteView{b: v.b, off: v.off + int(fieldOffsets(st)[i]), t: st.Field(i).Type(), ro: v.ro}}
}

// index is the address of element i of the array the view points to.
func (v *byteView) index(i int64, flags uint8) Value {
	at := v.t.Underlying().(*types.Array)
	checkIndex(i, int(at.Len()), flags)
	return Value{R: &byteView{b: v.b, off: v.off + int(i*gcSizes.Sizeof(at.Elem())), t: at.Elem(), ro: v.ro}}
}

// slice is a[lo:hi:max] of the array a the view points to: a Go slice that
// shares the bytes, or a copy when they are read-only. A byte array gives a
// []byte; an array of another basic integer or float type (a local
// [4]uint16 also seen as bytes, bytesAlloc) gives a native slice of that
// type over the same bytes, as unsafe.Slice does (uregion.wordSlice).
func (v *byteView) slice(s *sliceSite, regs []Value) Value {
	at, ok := v.t.Underlying().(*types.Array)
	if !ok {
		unsupported("unsafe: slicing a %s laid over bytes needs byte-level memory", v.t)
	}
	n := int(at.Len())
	if isByteType(at.Elem()) {
		lo, hi, mx := s.bounds(regs, n, n)
		b := v.b[v.off+lo : v.off+hi : v.off+mx]
		if v.ro {
			return Value{R: append([]byte(nil), b...)}
		}
		return Value{R: b}
	}
	bt, ok := types.Unalias(at.Elem()).(*types.Basic)
	var rt reflect.Type
	if ok {
		rt, ok = basicTypes[bt.Kind()]
	}
	if !ok {
		unsupported("unsafe: slicing a %s laid over bytes needs byte-level memory", v.t)
	}
	lo, hi, mx := s.bounds(regs, n, n)
	size := int(rt.Size())
	b := v.b[v.off+lo*size : v.off+mx*size]
	if v.ro {
		b = append([]byte(nil), b...)
	}
	if mx == lo {
		return Value{R: reflect.MakeSlice(reflect.SliceOf(rt), 0, 0)}
	}
	return Value{R: reflect.SliceAt(rt, unsafe.Pointer(&b[0]), mx-lo).Slice(0, hi-lo)}
}

// region is the whole array of the view, as a region seen from the view.
func (v *byteView) region() *uregion {
	return &uregion{base: v.addr() - uintptr(v.off), bytes: v.b, ro: v.ro}
}

// fieldOffsets returns the gc offsets of the fields of st.
func fieldOffsets(st *types.Struct) []int64 {
	fs := make([]*types.Var, st.NumFields())
	for i := range fs {
		fs[i] = st.Field(i)
	}
	return gcSizes.Offsetsof(fs)
}

// decodeBytes reads a value of type t (plain data) from b, in the host
// order. Integers get the canonical form of the VM (sign or zero
// extended), booleans 0 or 1, floats the float64 bits.
func decodeBytes(b []byte, t types.Type) Value {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Info()&(types.IsString|types.IsComplex) != 0 || u.Kind() == types.UnsafePointer {
			break // not plain data: refused below, never read as an integer
		}
		size := gcSizes.Sizeof(t)
		switch {
		case u.Info()&types.IsBoolean != 0:
			return Value{N: b2i(b[0] != 0)}
		case u.Info()&types.IsFloat != 0:
			if size == 4 {
				return Value{N: fv(float64(math.Float32frombits(binary.NativeEndian.Uint32(b))))}
			}
			return Value{N: int64(binary.NativeEndian.Uint64(b))}
		case u.Info()&types.IsUnsigned != 0:
			switch size {
			case 1:
				return Value{N: int64(b[0])}
			case 2:
				return Value{N: int64(binary.NativeEndian.Uint16(b))}
			case 4:
				return Value{N: int64(binary.NativeEndian.Uint32(b))}
			}
			return Value{N: int64(binary.NativeEndian.Uint64(b))}
		}
		switch size {
		case 1:
			return Value{N: int64(int8(b[0]))}
		case 2:
			return Value{N: int64(int16(binary.NativeEndian.Uint16(b)))}
		case 4:
			return Value{N: int64(int32(binary.NativeEndian.Uint32(b)))}
		}
		return Value{N: int64(binary.NativeEndian.Uint64(b))}
	case *types.Struct:
		offs := fieldOffsets(u)
		fs := make([]Value, u.NumFields())
		for i := range fs {
			ft := u.Field(i).Type()
			fs[i] = decodeBytes(b[offs[i]:offs[i]+gcSizes.Sizeof(ft)], ft)
		}
		return Value{R: fs}
	case *types.Array:
		n, size := int(u.Len()), gcSizes.Sizeof(u.Elem())
		es := make([]Value, n)
		for i := range es {
			es[i] = decodeBytes(b[int64(i)*size:int64(i+1)*size], u.Elem())
		}
		return Value{R: es}
	}
	unsupported("unsafe: reading a %s from bytes", t)
	return Value{}
}

// encodeBytes writes x, of type t (plain data), into b in the host order.
func encodeBytes(b []byte, x Value, t types.Type) {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Info()&(types.IsString|types.IsComplex) != 0 || u.Kind() == types.UnsafePointer {
			unsupported("unsafe: writing a %s into bytes", t) // not plain data
		}
		size := gcSizes.Sizeof(t)
		if u.Info()&types.IsFloat != 0 && size == 4 {
			binary.NativeEndian.PutUint32(b, math.Float32bits(float32(f64(x))))
			return
		}
		switch size {
		case 1:
			b[0] = byte(x.N)
		case 2:
			binary.NativeEndian.PutUint16(b, uint16(x.N))
		case 4:
			binary.NativeEndian.PutUint32(b, uint32(x.N))
		default:
			binary.NativeEndian.PutUint64(b, uint64(x.N))
		}
	case *types.Struct:
		fs, _ := x.R.([]Value)
		offs := fieldOffsets(u)
		for i := 0; i < u.NumFields() && i < len(fs); i++ {
			ft := u.Field(i).Type()
			encodeBytes(b[offs[i]:offs[i]+gcSizes.Sizeof(ft)], fs[i], ft)
		}
	case *types.Array:
		es, _ := x.R.([]Value)
		size := gcSizes.Sizeof(u.Elem())
		for i := 0; i < int(u.Len()) && i < len(es); i++ {
			encodeBytes(b[int64(i)*size:int64(i+1)*size], es[i], u.Elem())
		}
	default:
		unsupported("unsafe: writing a %s into bytes", t)
	}
}
