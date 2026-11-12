package vm

// Views of the two words of an interface (see docs/UNSAFE.md).
//
// In gc an interface value is two words: the type (or the itab) and the
// data, a pointer for the pointer-shaped types and a pointer to a boxed copy
// for the others. Libraries read these words through unsafe:
// protobuf (protoreflect.ifaceHeader, impl.pointerOfIface), zerolog
// (isNilValue: (*[2]uintptr)(unsafe.Pointer(&i))[1] == 0), reflect2.
//
// In the VM an interface variable is a cell that holds an *Iface. A view of
// its words is a hdrView whose source type is the interface: the words are
// computed from the *Iface each time, and a write builds a new *Iface.
//
//   - The type word is a token: a cell unique to the dynamic type (typeToks).
//     It is comparable as in gc (typeOf(x) == typeOf(y) when x and y have
//     the same dynamic type), and it is the only thing a write accepts as a
//     type word: an interface can only be built again from the words of an
//     interface, never from a type word the program made (protobuf's
//     getIface stores back the words that valueOfIface read).
//   - The data word is the pointer itself for a pointer-shaped dynamic type
//     (pointer, map, channel, func, unsafe.Pointer), and a pointer to a copy
//     of the value for the other types (a new copy at each read).
//
// Read as a uintptr, a word is the address behind it (0 for nil), so
// zerolog sees a nil pointer in an interface.

import (
	"go/types"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

// ptrHints keeps the type of the struct and array cells that the program
// reached through unsafe, for as long as the cells live. The ring of
// regions is a small cache: protobuf's proto.Equal walks a tree of
// messages, and the regions of the outer message are pushed out of the
// ring before the walk comes back to it; a []*T seen as a []pointer
// (rewrap) has too many elements to record a region for each at once. When
// the ring does not know a cell any more, its hint gives the region again
// (fromHint, resolveAt). The cells are held by weak pointers: a hint keeps
// no cell alive. They are found by address, checked by the weak pointer (a
// new cell at the address of a dead one has no hint).
type ptrHints struct {
	m    sync.Map // uintptr (address of the cell) -> *hint
	adds atomic.Int64
}

// hint is the struct (or array) type t of the cell top, at the address of
// the cell c: top itself (top is zero) or its first field.
type hint struct {
	c   weak.Pointer[Value]
	t   types.Type
	top weak.Pointer[Value]
}

// add records that the cell c holds a t, or is the first field (n levels
// down) of top, a t, when top is not nil.
func (h *ptrHints) add(c *Value, t types.Type, top *Value) {
	k := uintptr(unsafe.Pointer(c))
	if v, ok := h.m.Load(k); ok {
		if e := v.(*hint); e.c.Value() == c && e.t == t && (top == nil) == (e.top == weak.Pointer[Value]{}) && (top == nil || e.top.Value() == top) {
			return
		}
	}
	e := &hint{c: weak.Make(c), t: t}
	if top != nil {
		e.top = weak.Make(top)
	}
	h.m.Store(k, e)
	if h.adds.Add(1)%4096 == 0 {
		// drop the hints of the cells that are gone
		h.m.Range(func(k, v any) bool {
			if v.(*hint).c.Value() == nil {
				h.m.CompareAndDelete(k, v)
			}
			return true
		})
	}
}

// get returns the type and the top cell recorded for the cell c (top is c
// itself when c holds the t); t is nil when there is no hint.
func (h *ptrHints) get(c *Value) (t types.Type, top *Value) {
	v, ok := h.m.Load(uintptr(unsafe.Pointer(c)))
	if !ok {
		return nil, nil
	}
	e := v.(*hint)
	if e.c.Value() != c {
		return nil, nil // a dead cell had this address
	}
	if e.top == (weak.Pointer[Value]{}) {
		return e.t, c
	}
	if top = e.top.Value(); top == nil {
		return nil, nil
	}
	return e.t, top
}

// hintElems records the pointees of the pointers es of type t (a pointer to
// a struct or an array).
func (m *Machine) hintElems(es []Value, t types.Type) {
	pt, ok := t.Underlying().(*types.Pointer)
	if !ok || !isAggregate(pt.Elem()) {
		return
	}
	for _, e := range es {
		if c, ok := e.R.(*Value); ok && c != nil {
			if _, ok := c.R.([]Value); ok {
				m.hints.add(c, pt.Elem(), nil)
			}
		}
	}
}

// fromHint records the region of p (an unsafe.Pointer converted to a
// uintptr) when a hint knows the type there, and reports whether it did.
func (m *Machine) fromHint(p Value) bool {
	c, ok := p.R.(*Value)
	if !ok || c == nil {
		return false
	}
	t, top := m.hints.get(c)
	if t == nil {
		return false
	}
	m.addRegion(&uregion{base: addrOf(p), elems: unsafe.Slice(top, 1), t: t})
	return true
}

// typeTok is the content of the cell of a type word.
type typeTok struct{ rt *RType }

// isIface reports whether t is an interface type.
func isIface(t types.Type) bool { _, ok := t.Underlying().(*types.Interface); return ok }

// ifaceWords returns the positions (struct fields or array elements) of the
// two words of t when t is laid out as an interface value: two words
// (unsafe.Pointer, uintptr or pointers) and fields of size zero
// (protoreflect.ifaceHeader has a _ [0]any). nil otherwise.
func ifaceWords(t types.Type) []int {
	var ws []int
	switch u := t.Underlying().(type) {
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			ft := u.Field(i).Type()
			switch {
			case gcSizes.Sizeof(ft) == 0:
			case isWord(ft):
				ws = append(ws, i)
			default:
				return nil
			}
		}
	case *types.Array:
		if u.Len() == 2 && isWord(u.Elem()) {
			ws = []int{0, 1}
		}
	}
	if len(ws) != 2 {
		return nil
	}
	return ws
}

// isIfaceView reports whether a pointer to a src (an interface) can be
// seen as a pointer to t: the two words, or the first word alone.
func isIfaceView(src, t types.Type) bool {
	return isIface(src) && (ifaceWords(t) != nil || isWord(t))
}

// typeToken returns the type word of the dynamic type rt.
func (m *Machine) typeToken(rt *RType) Value {
	var key any = rt
	if rt.T == nil {
		key = rt.Native
	}
	if c, ok := m.typeToks.Load(key); ok {
		return Value{R: c.(*Value)}
	}
	c, _ := m.typeToks.LoadOrStore(key, &Value{R: typeTok{rt}})
	return Value{R: c.(*Value)}
}

// typeWordOf returns the type word of the values of the type that vt
// describes: the data word of a reflect.Type, as in gc.
func (m *Machine) typeWordOf(vt *vtype) Value {
	if vt.t != nil {
		return m.typeToken(m.rtypeOf(vt.t))
	}
	return m.typeToken(m.nativeRType(vt.nat))
}

// typeOfWord returns the reflect.Type of the program for the type whose
// type word is w (the data word of a reflect.Type), or nil.
func (m *Machine) typeOfWord(w Value) *Iface {
	c, ok := w.R.(*Value)
	if !ok || c == nil {
		return nil
	}
	tok, ok := c.R.(typeTok)
	if !ok {
		return nil
	}
	if tok.rt.T != nil {
		return m.vr.vtypeOf(tok.rt.T).ifc
	}
	// no go/types type: the native descriptor (as twinToVM)
	return &Iface{T: m.vr.typeRT, V: Value{R: natT(tok.rt.Native)}}
}

// ptrShaped reports whether gc stores a value of the dynamic type rt
// directly in the data word of an interface.
func ptrShaped(rt *RType) bool {
	if rt.T == nil {
		switch rt.Native.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
			return true
		}
		return false
	}
	switch u := rt.T.Underlying().(type) {
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature:
		return true
	case *types.Basic:
		return u.Kind() == types.UnsafePointer
	}
	return false
}

// ifaceWordsOf returns the type and data words of the interface value iv.
func (m *Machine) ifaceWordsOf(iv *Iface) [2]Value {
	if iv == nil {
		return [2]Value{}
	}
	tw := m.typeToken(iv.T)
	if vt, ok := iv.V.R.(*vtype); ok && m.vr != nil && iv.T == m.vr.typeRT {
		// a reflect.Type: in gc its data word is the *rtype, which is the
		// type word of the values of that type; reflect2 caches its types
		// by both (TypeOf and Type2)
		return [2]Value{tw, m.typeWordOf(vt)}
	}
	if ptrShaped(iv.T) {
		m.rememberPointee(iv.V, iv.T.T)
		return [2]Value{tw, iv.V}
	}
	if rv, ok := iv.V.R.(reflect.Value); ok && iv.T.Native != nil {
		p := reflect.New(rv.Type())
		p.Elem().Set(rv)
		return [2]Value{tw, {R: p}}
	}
	c := deepCopy(iv.V, iv.T.T)
	if _, ok := iv.T.T.Underlying().(*types.Slice); ok {
		// a slice: a view of its header, as for a slice field (a cell of a
		// slice looks like a cell of a struct; reflect2 reads the header,
		// (*sliceHeader)(PtrOf(v)).Data)
		return [2]Value{tw, {R: &hdrView{m: m, cell: &c, src: iv.T.T, t: iv.T.T, st: &hdrState{}}}}
	}
	// the copy of a struct or an array: offsets from the data word reach
	// its fields (reflect2's UnsafeStructField.UnsafeGet on PtrOf(v))
	m.rememberPointee(Value{R: &c}, types.NewPointer(iv.T.T))
	return [2]Value{tw, {R: &c}}
}

// rememberPointee records the memory of the pointer p, of type t, when t is
// a pointer to a struct or an array that lives in cells: a later
// unsafe.Pointer(uintptr(p) + off) finds the field at gc offset off, as
// protobuf does with the offsets of reflect.StructField
// (impl.pointer.Apply on the data word of a message).
func (m *Machine) rememberPointee(p Value, t types.Type) {
	pt, ok := t.(*types.Pointer)
	if !ok {
		if t == nil {
			return
		}
		pt, ok = t.Underlying().(*types.Pointer)
		if !ok {
			return
		}
	}
	if _, ok := pt.Elem().Underlying().(*types.Slice); ok {
		// a *[]T: a cell of a slice looks like a cell of a struct, the
		// hint tells (*sliceHeader)(p) to view its header (resolveAt)
		if c, ok := p.R.(*Value); ok && c != nil {
			m.hints.add(c, pt.Elem(), nil)
		}
		return
	}
	if !isAggregate(pt.Elem()) {
		return
	}
	if c, ok := p.R.(*Value); ok && c != nil {
		if _, ok := c.R.([]Value); ok && !m.knowsType(p, pt.Elem()) {
			if t, _ := m.hints.get(c); t != nil && firstPath(t, pt.Elem()) >= 0 {
				// the struct that p starts, recorded before (a *messageState
				// that is the first field of a message)
				m.fromHint(p)
				return
			}
			// not remember: a region without a type may already know p
			m.addRegion(lone(p, pt.Elem()))
			m.hints.add(c, pt.Elem(), nil)
		}
	}
}

// knowsType reports whether a recent region holds the cell p with the
// element type t, or starts at p with a type whose first field (n levels
// down) is a t: the region of a *Msg recorded at its first field already
// covers the *messageState there, and a new region would hide the *Msg from
// resolveAt.
func (m *Machine) knowsType(p Value, t types.Type) bool {
	addr := addrOf(p)
	if r := m.findRegionPtr(p); r != nil && r.t != nil && types.Identical(r.t, t) {
		// the region must give p at its own address: the region of a
		// struct recorded at its first field (firstOf) holds the struct
		// cell at the address of the field
		if off, ok := r.offsetOf(p); ok && r.base+uintptr(off) == addr {
			return true
		}
	}
	r := m.findRegion(addr)
	return r != nil && r.t != nil && r.base == addr && firstPath(r.t, t) > 0
}

// isAggregate reports whether t is a struct or an array type.
func isAggregate(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	}
	return false
}

// firstPath returns how many times to go down to the first field (or
// element) from a value of type t to reach a part of type to, which gc
// places at the same address; -1 when there is none.
func firstPath(t, to types.Type) int {
	for n := 0; n < 16; n++ {
		if sameLayout(t, to) {
			return n
		}
		switch u := t.Underlying().(type) {
		case *types.Struct:
			if u.NumFields() == 0 {
				return -1
			}
			t = u.Field(0).Type()
		case *types.Array:
			if u.Len() == 0 {
				return -1
			}
			t = u.Elem()
		default:
			return -1
		}
	}
	return -1
}

// resolveAt returns the pointer p (a cell) seen as a *to when a recent
// region of a known type starts at the address of p: the struct there, or
// its first field, or the first field of that, whichever has the layout of
// to. In gc they all have the address of p; in the VM they are distinct
// cells. Down: a *Msg seen as a *messageState (its first field). Up: the
// pointer of the messageState seen as a *Msg again (protobuf's
// reflect.NewAt(t, ms.pointer())). Without such a region, p is unchanged.
func (m *Machine) resolveAt(p Value, to types.Type) Value {
	if c, ok := p.R.(*Value); !ok || c == nil {
		return p
	}
	c := p.R.(*Value)
	addr := addrOf(p)
	var t types.Type
	var top *Value
	if r := m.findRegion(addr); r != nil && r.t != nil && r.base == addr && len(r.elems) > 0 {
		t, top = r.t, &r.elems[0]
	} else if r := m.findRegionPtr(p); r != nil && r.t != nil && r.holds(p) {
		// an element of an array after the first one (reflect2's
		// arrayAt(Data, i, size) on the elements of a slice)
		t, top = r.t, c
	} else if t, top = m.hints.get(c); t == nil {
		return p
	}
	if n := firstPath(t, to); n >= 0 {
		return m.firstOf(Value{R: top}, t, n)
	}
	if n, ht := firstHdr(t, to); n >= 0 {
		// a slice or a string (or the first field that is one) seen as
		// its header: (*sliceHeader)(ptr) in reflect2
		return m.newView(m.firstOf(Value{R: top}, t, n), ht, to)
	}
	return p
}

// firstHdr returns how many times to go down to the first field from a
// value of type t to reach a slice or a string that a *to can view as its
// header, and that type; -1 when there is none.
func firstHdr(t, to types.Type) (int, types.Type) {
	for n := 0; n < 16; n++ {
		if isHdrSrc(t) && isHdrView(t, to) {
			return n, t
		}
		switch u := t.Underlying().(type) {
		case *types.Struct:
			if u.NumFields() == 0 {
				return -1, nil
			}
			t = u.Field(0).Type()
		case *types.Array:
			if u.Len() == 0 {
				return -1, nil
			}
			t = u.Elem()
		default:
			return -1, nil
		}
	}
	return -1, nil
}

// firstOf returns the pointer to the part of type to at the address of p, a
// pointer to a t in cells: p itself, or its first field (n levels down). In
// gc a struct and its first field have the same address; in the VM they are
// two cells, so the struct is recorded at the address of the field too: an
// offset from the field pointer (protobuf's messageState.pointer().Apply)
// reaches the other fields.
func (m *Machine) firstOf(p Value, t types.Type, n int) Value {
	cell, ok := p.R.(*Value)
	if !ok || n <= 0 {
		return p
	}
	top, ft := cell, t
	for i := 0; i < n; i++ {
		fs, ok := cell.R.([]Value)
		if !ok || len(fs) == 0 {
			unsupported("unsafe: viewing a %s as its first field needs its cells", t)
		}
		cell = &fs[0]
		switch u := ft.Underlying().(type) {
		case *types.Struct:
			ft = u.Field(0).Type()
		case *types.Array:
			ft = u.Elem()
		}
	}
	q := Value{R: cell}
	m.reached(q, ft)
	addr := addrOf(q)
	if r := m.findRegion(addr); r == nil || r.base != addr || r.t == nil || !types.Identical(r.t, t) || len(r.elems) == 0 || &r.elems[0] != top {
		m.addRegion(&uregion{base: addr, elems: unsafe.Slice(top, 1), t: t})
		m.hints.add(cell, t, top)
	}
	return q
}

// ifaceFrom builds the interface value of type src from its two words. It
// reports false when the words are not complete yet (a type and no data
// for a type that is not pointer-shaped, or data and no type).
func (m *Machine) ifaceFrom(ws [2]Value, src types.Type) (Value, bool) {
	if ws[0].R == nil {
		return Value{}, ws[1].R == nil
	}
	cell, _ := ws[0].R.(*Value)
	var tok typeTok
	ok := false
	if cell != nil {
		tok, ok = cell.R.(typeTok)
	}
	if !ok {
		unsupported("unsafe: the type word of an interface must come from an interface value")
	}
	rt := tok.rt
	if it, _ := src.Underlying().(*types.Interface); it != nil && !it.Empty() {
		if rt.T == nil || !types.Implements(rt.T, it) {
			unsupported("unsafe: an interface word of type %s does not implement %s", rt, src)
		}
	}
	if m.vr != nil && rt == m.vr.typeRT {
		// a reflect.Type made of the words of one (ifaceWordsOf)
		if ws[1].R == nil {
			return Value{}, false // data not written yet
		}
		ifc := m.typeOfWord(ws[1])
		if ifc == nil {
			unsupported("unsafe: the data word of a reflect.Type must come from a reflect.Type")
		}
		return Value{R: ifc}, true
	}
	if ptrShaped(rt) {
		return Value{R: &Iface{T: rt, V: ws[1]}}, true
	}
	var v Value
	switch p := ws[1].R.(type) {
	case nil:
		return Value{}, false // data not written yet
	case *Value:
		if rt.T == nil {
			unsupported("unsafe: the data word of a native %s must come from native memory", rt)
		}
		v = deepCopy(*p, rt.T)
	case *byteView:
		if rt.T == nil {
			unsupported("unsafe: the data word of a native %s must come from native memory", rt)
		}
		v = p.load(rt.T)
	case reflect.Value:
		if p.Kind() != reflect.Pointer || p.IsNil() {
			unsupported("unsafe: the data word of an interface of type %s", rt)
		}
		if rt.T == nil {
			if p.Type().Elem() != rt.Native {
				unsupported("unsafe: the data word of an interface of type %s points to a %s", rt, p.Type().Elem())
			}
			e := reflect.New(rt.Native).Elem()
			e.Set(p.Elem())
			v = Value{R: e}
		} else {
			v = deepCopy(m.fromNative(p.Elem(), rt.T), rt.T) // a copy, as from a cell
		}
	default:
		unsupported("unsafe: the data word of an interface of type %s", rt)
	}
	return Value{R: &Iface{T: rt, V: v}}, true
}

// ifaceWord returns the word w (a pointer-like or an integer value) as a
// value of type t.
func (m *Machine) ifaceWord(w Value, t types.Type) Value {
	if pt, ok := t.Underlying().(*types.Pointer); ok && m != nil && isIntWord(pt.Elem()) {
		if c, ok := m.funcCode(w); ok {
			return c
		}
	}
	if _, ok := t.Underlying().(*types.Pointer); ok {
		if c, ok := w.R.(*Value); ok && c != nil {
			if _, ok := c.R.(typeTok); ok {
				// (*eface)(unsafe.Pointer(&v)).typ.kind: there is no
				// runtime type behind a type word
				unsupported("unsafe: the type word of an interface seen as a %s: the VM has no runtime type to read", t)
			}
		}
	}
	if isPtrLike(t) {
		return w
	}
	return Value{N: int64(refAddr(w.R))}
}

// funcCode returns, for w, the data word of a func in an interface, a
// pointer to the first word of gc's funcval: the code pointer. A program
// that reads it (**(**uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(&f)) +
// 8)), the funcPC of GOROOT test fixedbugs/issue17381.go) gets the address
// that reflect.ValueOf(f).Pointer() gives: for an interpreted function the
// address that runtime.FuncForPC knows (funcpc.go), for a native one its
// real code pointer. The word is a copy, made at each read, as the data
// word of the other values that are not pointers: a write to it changes
// nothing. ok is false when w is not a func.
func (m *Machine) funcCode(w Value) (Value, bool) {
	var pc uintptr
	switch f := w.R.(type) {
	case *Closure:
		switch {
		case f == nil:
			return Value{}, false
		case f.Fn != nil:
			pc = uintptr(m.funcPC(f.Fn))
		case f.Direct != nil:
			pc = uintptr(m.funcPC(f.Direct))
		default:
			pc = uintptr(unsafe.Pointer(f)) // a VM intrinsic: no code to name
		}
	case reflect.Value:
		if f.Kind() != reflect.Func || f.IsNil() {
			return Value{}, false
		}
		pc = f.Pointer()
	default:
		return Value{}, false
	}
	return Value{R: &Value{N: int64(pc)}}, true
}

// ifaceWordsType is the type of the view of the two words of an interface
// that address arithmetic reaches (descend).
var ifaceWordsType = types.NewArray(types.Typ[types.UnsafePointer], 2)

// isIntWord reports whether t is an integer of the size of a pointer.
func isIntWord(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsInteger != 0 && gcSizes.Sizeof(t) == ptrSize
}

// ifaceSlot returns the word (0: type, 1: data) at field or element i of
// the type t of the view.
func ifaceSlot(t types.Type, i int) int {
	for k, f := range ifaceWords(t) {
		if f == i {
			return k
		}
	}
	unsupported("unsafe: field %d of an interface seen as a %s", i, t)
	return 0
}

// ifaceCur returns the words of the view: the pending ones, or the words of
// the interface in the cell.
func (v *hdrView) ifaceCur() [2]Value {
	iv, _ := v.cell.R.(*Iface)
	if p := v.st.ipend; p != nil {
		if iv == v.st.iseen {
			return *p
		}
		v.st.ipend = nil // the variable was assigned since
	}
	return v.m.ifaceWordsOf(iv)
}

// ifaceSet makes ws the words of the variable: an interface built from
// them, or pending words until they are complete.
func (v *hdrView) ifaceSet(ws [2]Value) {
	if val, ok := v.m.ifaceFrom(ws, v.src); ok {
		*v.cell = val
		v.st.ipend = nil
		return
	}
	v.st.ipend = &ws
	v.st.iseen, _ = v.cell.R.(*Iface)
}

// ifaceSetWord writes x, of type t, as word k of ws. An integer must be 0
// or the current word: the VM never makes a pointer from an integer.
func ifaceSetWord(ws *[2]Value, k int, x Value, t types.Type) {
	switch {
	case isPtrLike(t):
		ws[k] = x
	case x.N == 0:
		ws[k] = Value{}
	case uintptr(x.N) != refAddr(ws[k].R):
		unsupported("unsafe: an interface word written as the integer %#x, which is not its current value", uintptr(x.N))
	}
}

// ifaceLoad reads the view as a value of type t.
func (v *hdrView) ifaceLoad(t types.Type) Value {
	ws := v.ifaceCur()
	if isWord(t) {
		return v.m.ifaceWord(ws[0], t)
	}
	pos := ifaceWords(t)
	switch u := t.Underlying().(type) {
	case *types.Struct:
		fs := make([]Value, u.NumFields())
		for i := range fs {
			fs[i] = v.m.zero(u.Field(i).Type())
		}
		for k, i := range pos {
			fs[i] = v.m.ifaceWord(ws[k], u.Field(i).Type())
		}
		return Value{R: fs}
	case *types.Array:
		return Value{R: []Value{v.m.ifaceWord(ws[0], u.Elem()), v.m.ifaceWord(ws[1], u.Elem())}}
	}
	unsupported("unsafe: reading an interface as a %s", t)
	return Value{}
}

// ifaceStore writes x, of type t, through the view.
func (v *hdrView) ifaceStore(x Value, t types.Type) {
	ws := v.ifaceCur()
	if isWord(t) {
		ifaceSetWord(&ws, 0, x, t)
		v.ifaceSet(ws)
		return
	}
	fs, ok := x.R.([]Value)
	if !ok {
		unsupported("unsafe: writing a %T as the words of an interface", x.R)
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		for k, i := range ifaceWords(t) {
			ifaceSetWord(&ws, k, fs[i], u.Field(i).Type())
		}
	case *types.Array:
		for k := range ws {
			ifaceSetWord(&ws, k, fs[k], u.Elem())
		}
	}
	v.ifaceSet(ws)
}
