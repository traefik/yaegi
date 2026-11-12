package vm

// Views of the header of a string or a slice (see docs/UNSAFE.md).
//
// In gc a string is a header {data, len} and a slice a header {data, len,
// cap}. Programs read and write these words through unsafe pointers:
// (*reflect.StringHeader)(unsafe.Pointer(&s)), *(*uintptr)(unsafe.Pointer(&b)),
// a local struct{ p unsafe.Pointer; n int }, or the zero-copy idioms
// *(*string)(unsafe.Pointer(&b)) and *(*[]byte)(unsafe.Pointer(&s)).
//
// In the VM the variable is a cell that holds a Go string, a []byte, a
// []Value... A view is a pointer (*hdrView) to that cell with the type the
// program sees. Loads and stores through the view, or through the address
// of one of its words (*hdrWord), compute the words from the value in the
// cell, and a write builds a new value from the new words and puts it in
// the cell. The new value is always made by slicing a real Go slice or
// string that the VM holds (the data pointer must come from memory whose
// extent the VM knows, unsafe.go): a header can never reach memory outside
// the backing array of its data. A header that reaches further, or whose
// data address the VM does not know, is refused (unsupported).
//
// A header can be invalid for a while, for example after hdr.Len = n and
// before hdr.Cap = n, or after hdr.Data = p of an empty string (a Go
// string of length 0 has no data pointer to keep). Such a header is kept
// in the view (pending) and the cell keeps its last valid value, until a
// later write makes the header valid again.

import (
	"go/types"
	"reflect"
	"unsafe"
)

// hdr is the header of a string or a slice, with the memory its data
// pointer reaches.
type hdr struct {
	addr     uintptr       // data address, as the program sees it
	ptr      Value         // data pointer (*byte, *Value, reflect.Value), Value{} when there is no element
	bytes    []byte        // memory from ptr to the end of its array (bytes),
	elems    []Value       // or (cells)
	et       types.Type    // type of elems (nil: unknown, then elems has one cell)
	nat      reflect.Value // native slice the header comes from (data not changed)
	cell0    *Value        // data pointer of a slice of cells of capacity 0 (no element)
	known    bool          // the memory from ptr is known (bytes, elems or nat)
	ro       bool          // bytes is read-only memory (roByte)
	len, cap int
}

func (h hdr) triple() [3]uintptr { return [3]uintptr{h.addr, uintptr(h.len), uintptr(h.cap)} }

// setData copies the data part of d into h (len and cap do not change).
func (h *hdr) setData(d hdr) {
	h.addr, h.ptr, h.bytes, h.elems, h.et, h.nat, h.known, h.ro, h.cell0 = d.addr, d.ptr, d.bytes, d.elems, d.et, d.nat, d.known, d.ro, d.cell0
}

var byteSlice = types.NewSlice(types.Typ[types.Byte])

// emptySubstring is str[i:i] for the string str that src holds.
//
// In gc, the data pointer of an empty substring is the data pointer of
// str (the compiler does not move it past the end, see slicecap.go of the
// GOROOT tests). A Go string of length 0 put in an interface loses its data
// pointer (runtime.convTstring gives the zero value), and Value.R is an
// interface: so the result keeps the data address in N, where only the
// header of the string reads it back (hdrOf). It is a number, like a
// uintptr: it keeps nothing alive. An empty string that has an address
// already (src.N) keeps it.
func emptySubstring(src Value, str string) Value {
	if len(str) == 0 {
		return Value{N: src.N, R: ""}
	}
	return Value{N: int64(uintptr(unsafe.Pointer(unsafe.StringData(str)))), R: ""}
}

// hdrOf returns the header of the value v of type t (a string or a slice).
func hdrOf(v Value, t types.Type) hdr {
	switch s := v.R.(type) {
	case nil:
		return hdr{known: true}
	case string:
		h := hdr{len: len(s), cap: len(s), known: true}
		p := unsafe.StringData(s)
		h.addr = uintptr(unsafe.Pointer(p))
		if len(s) == 0 && v.N != 0 {
			h.addr = uintptr(v.N) // an empty substring (emptySubstring)
		}
		if len(s) > 0 {
			h.ro = strReadOnly(p)
			h.ptr, h.bytes = bytePtr(p, h.ro), unsafe.Slice(p, len(s))
		}
		return h
	case []byte:
		h := hdr{len: len(s), cap: cap(s), known: true}
		h.addr = uintptr(unsafe.Pointer(unsafe.SliceData(s)))
		if cap(s) > 0 {
			h.bytes = s[:cap(s)]
			h.ptr = Value{R: &h.bytes[0]}
		}
		return h
	case []Value:
		h := hdr{len: len(s), cap: cap(s), known: true, et: sliceElem(t)}
		h.addr = uintptr(unsafe.Pointer(unsafe.SliceData(s)))
		if cap(s) > 0 {
			h.elems = s[:cap(s)]
			h.ptr = Value{R: &h.elems[0]}
		} else if s != nil {
			// never past the end of the array: a slice of capacity 0
			// keeps the data pointer of the slice it was cut from
			h.cell0 = unsafe.SliceData(s)
		}
		return h
	case *zslice:
		return hdr{addr: uintptr(unsafe.Pointer(unsafe.SliceData(s.s))), len: len(s.s), cap: cap(s.s)}
	case reflect.Value:
		switch s.Kind() {
		case reflect.String:
			return hdrOf(Value{R: s.String()}, t)
		case reflect.Slice:
			h := hdr{addr: s.Pointer(), len: s.Len(), cap: s.Cap(), nat: s, known: true}
			if h.cap > 0 {
				h.ptr = Value{R: s.Slice(0, 1).Index(0).Addr()}
			}
			return h
		}
	}
	unsupported("unsafe: header of a %T", v.R)
	return hdr{}
}

// dataIn returns the data part of a header whose data pointer is p, in
// the region r.
func dataIn(r *uregion, p Value) hdr {
	h := hdr{addr: addrOf(p), ptr: p, known: true}
	if i, ok := r.index(p); ok {
		if r.bytes != nil {
			h.bytes, h.ro = r.bytes[i:], r.ro
		} else {
			h.elems, h.et = r.elems[i:], r.t
		}
		return h
	}
	// a field or an element inside one element of the region: this cell
	// only, of unknown type
	switch q := p.R.(type) {
	case *Value:
		h.elems = unsafe.Slice(q, 1)
	case *byte:
		h.bytes = unsafe.Slice(q, 1)
	case roByte:
		h.bytes, h.ro = unsafe.Slice((*byte)(q), 1), true
	default:
		h.known = false
	}
	return h
}

// dataAt returns the data part of a header whose data is the address u.
func (m *Machine) dataAt(u uintptr) hdr {
	if u == 0 {
		return hdr{known: true}
	}
	r := m.findRegion(u)
	if r == nil {
		unsupported("unsafe: the data address %#x of a string or slice header does not come from memory known to the VM", u)
	}
	h := dataIn(r, r.resolve(int64(u-r.base)))
	h.addr = u
	return h
}

// dataPtr returns the data part of a header whose data is the pointer p.
func (m *Machine) dataPtr(p Value) hdr {
	switch p.R.(type) {
	case nil:
		return hdr{known: true}
	case *Value, *byte, roByte:
	default:
		unsupported("unsafe: a %T as the data of a string or slice header", p.R)
	}
	if r := m.findRegionPtr(p); r != nil {
		return dataIn(r, p)
	}
	return dataIn(lone(p, nil), p)
}

// refuse reports a header that reaches outside known memory.
func refuse(h hdr, n int) {
	unsupported("unsafe: a string or slice header of length %d and capacity %d reaches outside the memory known to the VM (%d elements)", h.len, h.cap, n)
}

// extent is the number of elements reachable from the data pointer.
func (h *hdr) extent() int {
	switch {
	case h.nat.IsValid():
		return h.nat.Cap()
	case h.bytes != nil:
		return len(h.bytes)
	}
	return len(h.elems)
}

// makeHdr builds the value of type t (a string or a slice) that has the
// header h. It reports false when h is not a valid header yet (no data
// with a non-zero length, a length above the capacity) and refuses
// (unsupported) a header that reaches outside the memory of its data.
func makeHdr(h hdr, t types.Type) (Value, bool) {
	if h.len < 0 || h.cap < 0 {
		unsupported("unsafe: negative length in a string or slice header")
	}
	if h.ptr.R == nil {
		switch {
		case h.len == 0 && h.cap == 0 && isString(t):
			return Value{R: ""}, true
		case h.len == 0 && h.cap == 0:
			return Value{}, true
		case h.addr == 0:
			return Value{}, false // data not written yet
		}
		refuse(h, 0)
	}
	if !h.known {
		unsupported("unsafe: the data of a string or slice header does not come from memory known to the VM")
	}
	n := h.extent()
	if isString(t) {
		if h.len > n {
			refuse(h, n)
		}
		switch {
		case h.len == 0:
			return Value{R: ""}, true
		case h.nat.IsValid():
			if h.nat.Type().Elem().Kind() != reflect.Uint8 {
				unsupported("unsafe: viewing a %s as a string needs byte-level memory", h.nat.Type())
			}
			return Value{R: string(h.nat.Slice(0, h.len).Bytes())}, true
		case h.bytes != nil:
			return Value{R: unsafe.String(&h.bytes[0], h.len)}, true
		}
		if h.et != nil && !isByteType(h.et) {
			unsupported("unsafe: viewing a []%s as a string needs byte-level memory", h.et)
		}
		// The bytes are cells: the string is a copy.
		return Value{R: string(valBytes(Value{R: h.elems[:h.len]}))}, true
	}
	if h.len > n || h.cap > n {
		refuse(h, n)
	}
	if h.len > h.cap {
		return Value{}, false // capacity not written yet
	}
	et := sliceElem(t)
	switch {
	case h.nat.IsValid():
		ne := h.nat.Type().Elem()
		if isByteType(et) != (ne.Kind() == reflect.Uint8) || gcSizes.Sizeof(et) != int64(ne.Size()) {
			unsupported("unsafe: viewing a %s as a %s needs byte-level memory", h.nat.Type(), t)
		}
		return Value{R: h.nat.Slice3(0, h.len, h.cap)}, true
	case h.bytes != nil:
		if !isByteType(et) {
			unsupported("unsafe: viewing bytes as a %s needs byte-level memory", t)
		}
		if h.ro {
			// writable bytes of read-only memory: a copy
			b := make([]byte, h.len, h.cap)
			copy(b, h.bytes)
			return Value{R: b}, true
		}
		return Value{R: h.bytes[:h.len:h.cap]}, true
	}
	if h.et != nil && !sameLayout(h.et, et) {
		if es, ok := rewrap(h.elems[:h.cap], h.et, et); ok {
			return Value{R: es[:h.len]}, true
		}
		unsupported("unsafe: viewing a []%s as a %s needs byte-level memory", h.et, t)
	}
	return Value{R: h.elems[:h.len:h.cap]}, true
}

// oneField returns the type of the only field of the struct type t, or nil.
func oneField(t types.Type) types.Type {
	if st, ok := t.Underlying().(*types.Struct); ok && st.NumFields() == 1 {
		return st.Field(0).Type()
	}
	return nil
}

// rewrap returns a copy of the elements es of type from as elements of type
// to, when one is a struct with one field that has the layout of the other:
// protobuf reads a []*T as a []pointer (struct{ p unsafe.Pointer }) and
// stores the slice it appended back (impl.pointer.PointerSlice and
// AppendPointerSlice). In gc both have the same memory; in the VM a struct
// is a cell of fields, so the elements are copied: a later write to an
// element of one slice is not seen in the other (docs/UNSAFE.md).
func rewrap(es []Value, from, to types.Type) ([]Value, bool) {
	like := func(a, b types.Type) bool { return b != nil && (sameLayout(a, b) || sameLayout(b, a)) }
	switch {
	case like(from, oneField(to)):
		out := make([]Value, len(es))
		for i, e := range es {
			out[i] = Value{R: []Value{e}}
		}
		return out, true
	case like(to, oneField(from)):
		out := make([]Value, len(es))
		for i, e := range es {
			fs, ok := e.R.([]Value)
			if !ok || len(fs) != 1 {
				return nil, false
			}
			out[i] = fs[0]
		}
		return out, true
	}
	return nil, false
}

// hdrView is a pointer to a string or slice variable (cell) seen as a
// pointer to t: a header-like struct, a word, or another string or slice
// type.
type hdrView struct {
	m    *Machine // nil: a view made by unsafe arithmetic (descend), which does not record regions
	cell *Value
	src  types.Type // type of the variable: a string or a slice
	t    types.Type // type the program sees
	st   *hdrState  // shared by the views of the same pointer
}

// hdrState is the header written through a view that is not valid yet.
type hdrState struct {
	pend *hdr
	seen [3]uintptr // header of the cell when pend was set

	// the words of an interface written through a view that are not
	// complete yet (ifaceview.go), and the interface of the cell then
	ipend *[2]Value
	iseen *Iface
}

// hdrWord points to word (0: data, 1: len, 2: cap) of a header view.
type hdrWord struct {
	v    *hdrView
	word int
}

// isHdrSrc reports whether t has a header: a string or a slice.
func isHdrSrc(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsString != 0
	case *types.Slice:
		return true
	}
	return false
}

// isWord reports whether t is a type gc stores in one word that a header
// view can use: an integer of the size of a pointer, or a pointer.
func isWord(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Uintptr, types.Int, types.Uint, types.UnsafePointer:
			return true
		case types.Int64, types.Uint64:
			return ptrSize == 8
		}
	case *types.Pointer:
		return true
	}
	return false
}

// hdrWords returns the types of the words of t when t is one word, or a
// struct or an array of one to three words; nil otherwise.
func hdrWords(t types.Type) []types.Type {
	if isWord(t) {
		return []types.Type{t}
	}
	var ws []types.Type
	switch u := t.Underlying().(type) {
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			ws = append(ws, u.Field(i).Type())
		}
	case *types.Array:
		for i := int64(0); i < u.Len() && i < 4; i++ {
			ws = append(ws, u.Elem())
		}
	}
	if len(ws) == 0 || len(ws) > 3 {
		return nil
	}
	for _, w := range ws {
		if !isWord(w) {
			return nil
		}
	}
	return ws
}

// isStrHdr reports whether t is a struct of a string and one word: gc lays
// it out as the header of a slice (data, len, cap), and cespare/xxhash reads
// sliceHeader{s, len(s)} as a []byte to hash a string without a copy.
func isStrHdr(t types.Type) bool {
	st, ok := t.Underlying().(*types.Struct)
	return ok && st.NumFields() == 2 && isString(st.Field(0).Type()) && isWord(st.Field(1).Type()) && !isPtrLike(st.Field(1).Type())
}

// isHdrView reports whether a pointer to a src (string or slice) can be
// seen as a pointer to t.
func isHdrView(src, t types.Type) bool {
	return isHdrSrc(src) && (isHdrSrc(t) || hdrWords(t) != nil)
}

// newView returns the view as a *to of the pointer p to a from (a string
// or a slice).
func (m *Machine) newView(p Value, from, to types.Type) Value {
	switch q := p.R.(type) {
	case nil:
		return Value{}
	case *hdrView:
		return q.as(to)
	case *Value:
		return Value{R: &hdrView{m: m, cell: q, src: from, t: to, st: &hdrState{}}}
	}
	unsupported("unsafe: header view of a %T", p.R)
	return Value{}
}

// viewOf returns the unsafe.Pointer p seen as a *to. A view stays a view
// (or gives back its variable); a view of bytes, or a pointer to a byte,
// becomes a view of the same bytes as a *to when to is plain data, and is
// refused otherwise (byteview.go). With detect, a cell that holds a string
// or a []byte becomes a view when to is not its type.
func (m *Machine) viewOf(p Value, to types.Type, detect bool) Value {
	if v, ok := m.nativeView(p, nil, to); ok {
		// a native value seen as another type (nativeview.go); before,
		// p kept the native type and a store through it panicked in
		// reflect
		return v
	}
	switch q := p.R.(type) {
	case *hdrView:
		return q.as(to)
	case *byteView:
		return q.as(m, to)
	case *byte, roByte:
		// Another one-byte type (an int8, a bool) over a byte of a Go
		// byte array, or a wider type when a recent region holds the
		// byte: a view, so that a load gives the canonical form of the
		// VM (an int8 is sign extended). A byte seen as a byte stays a
		// pointer.
		if !isByteType(to) {
			if v, ok := m.byteViewOf(p, nil, to); ok {
				return v
			}
		}
	case *Value:
		if !detect {
			break
		}
		var src types.Type
		switch q.R.(type) {
		case string:
			src = types.Typ[types.String]
		case []byte:
			src = byteSlice
		case *Iface:
			// the words of an interface whose static type the SSA does
			// not show: written words must make an interface of any type
			if isIfaceView(emptyIface, to) {
				return m.newView(p, emptyIface, to)
			}
			return p
		case []Value, nil:
			// a struct or an array whose type a recent region knows (the
			// SSA does not show it): a view of its first field goes down
			// to that field, and the first field seen as the struct goes
			// up, as in gc (impl.Export.MessageStateOf). A slice (nil or
			// not) with a hint is viewed as its header.
			return m.resolveAt(p, to)
		default:
			return p
		}
		if !sameLayout(src, to) && isHdrView(src, to) {
			return m.newView(p, src, to)
		}
	}
	return p
}

// as returns the view seen as a *to.
func (v *hdrView) as(to types.Type) Value {
	switch {
	case types.Identical(to.Underlying(), v.src.Underlying()):
		return Value{R: v.cell}
	case isIface(v.src):
		if isIfaceView(v.src, to) {
			return Value{R: &hdrView{m: v.m, cell: v.cell, src: v.src, t: to, st: v.st}}
		}
	case sameLayout(v.t, to) || isHdrView(v.src, to):
		return Value{R: &hdrView{m: v.m, cell: v.cell, src: v.src, t: to, st: v.st}}
	}
	unsupported("unsafe: viewing a %s as a %s needs byte-level memory", v.src, to)
	return Value{}
}

// cur returns the current header: the pending one, or the header of the
// value in the cell.
func (v *hdrView) cur() hdr {
	h := hdrOf(*v.cell, v.src)
	if p := v.st.pend; p != nil {
		if h.triple() == v.st.seen {
			return *p
		}
		v.st.pend = nil // the variable was assigned since
	}
	return h
}

// Words of a header written by a store (set).
const (
	wroteData = 1 << iota
	wroteLen
	wroteCap
	wroteAll = wroteData | wroteLen | wroteCap
)

// set makes h the header of the variable; wrote has the words the store
// wrote.
func (v *hdrView) set(h hdr, wrote int) {
	if _, ok := v.cell.R.(reflect.Value); ok {
		unsupported("unsafe: writing the header of a native variable")
	}
	if isString(v.src) {
		h.cap = h.len
	}
	// When the data moves first (hdr.Data += 2 before hdr.Len -= 2 and
	// hdr.Cap -= 2), the old length and capacity reach outside the memory
	// of the new data for a while. Such a header is kept like the other
	// invalid headers: reading it through the view still refuses it
	// (makeHdr), and the variable keeps its last valid value, so no memory
	// outside the array is reached. A length or a capacity that the store
	// itself wrote outside the memory is refused at once.
	var val Value
	ok := false
	n := h.extent()
	if late := h.ptr.R != nil && h.known && (h.len > n || h.cap > n) &&
		!(wrote&wroteLen != 0 && h.len > n) && !(wrote&wroteCap != 0 && h.cap > n); !late {
		val, ok = makeHdr(h, v.src)
	}
	if ok {
		*v.cell = val
		// The cell gives back the same header, and its memory reaches as
		// far (a slice of capacity 0 or an empty string has none): no need
		// to keep h.
		if n := hdrOf(val, v.src); n.triple() == h.triple() && n.extent() == h.extent() {
			v.st.pend = nil
			return
		}
	}
	v.st.pend = &h
	v.st.seen = hdrOf(*v.cell, v.src).triple()
}

// word returns word w of the header h, of type t.
func (v *hdrView) word(h hdr, w int, t types.Type) Value {
	switch w {
	case 0:
		// Remember the memory: unsafe.Pointer(hdr.Data), or a header that
		// gets this data, finds it again.
		a, known := uintptr(0), false
		if v.m != nil {
			a, known = v.m.knowsRegion(h.ptr, int64(len(h.bytes))+int64(len(h.elems))*elemSize(h.et))
		}
		switch {
		case v.m == nil:
		case known:
			// The array is a region the program knows: the address is
			// its base plus the gc offset of the data (uregion.at), as
			// for uintptr(unsafe.Pointer(&a[i])). For cells, it is not
			// the real address of the cell.
			h.addr = a
		case h.cell0 != nil:
			// A slice of cells of capacity 0 has no element, but its data
			// pointer is the one of the slice it was cut from (gc does
			// not move it past the end): its address in the region of
			// that array, when the program knows it (GOROOT test
			// slicecap.go).
			if r := v.m.findRegionPtr(Value{R: h.cell0}); r != nil {
				if a, ok := r.addrIn(Value{R: h.cell0}); ok {
					h.addr = a
				}
			}
		case len(h.bytes) > 0:
			v.m.addRegion(&uregion{base: h.addr, bytes: h.bytes, ro: h.ro})
		case len(h.elems) > 0:
			v.m.addRegion(&uregion{base: h.addr, elems: h.elems, t: h.et})
		}
		if isPtrLike(t) {
			if h.ptr.R == nil && h.addr != 0 {
				// an empty substring or a slice of capacity 0: its data
				// address, with no element to point to (badAddr)
				return Value{R: badAddr(h.addr)}
			}
			return h.ptr
		}
		return Value{N: int64(h.addr)}
	case 1:
		return Value{N: int64(h.len)}
	case 2:
		if !isString(v.src) {
			return Value{N: int64(h.cap)}
		}
	}
	unsupported("unsafe: word %d of the header of a %s", w, v.src)
	return Value{}
}

// elemSize is the gc size of t (1 when t is unknown).
func elemSize(t types.Type) int64 {
	if t == nil {
		return 1
	}
	return gcSizes.Sizeof(t)
}

// setWord writes the value x of type t as word w of the header h.
func (v *hdrView) setWord(h *hdr, w int, x Value, t types.Type) {
	switch w {
	case 0:
		if isPtrLike(t) {
			if x.R != h.ptr.R {
				h.setData(v.m.dataPtr(x))
			}
		} else if u := uintptr(x.N); u != h.addr {
			h.setData(v.m.dataAt(u))
		}
	case 1:
		h.len = int(x.N)
	case 2:
		if isString(v.src) {
			unsupported("unsafe: word %d of the header of a %s", w, v.src)
		}
		h.cap = int(x.N)
	default:
		unsupported("unsafe: word %d of the header of a %s", w, v.src)
	}
}

// load reads the view as a value of type t.
func (v *hdrView) load(t types.Type) Value {
	if types.Identical(t.Underlying(), v.src.Underlying()) {
		return *v.cell
	}
	if isIface(v.src) {
		return v.ifaceLoad(t)
	}
	h := v.cur()
	if isHdrSrc(t) {
		if isString(v.src) {
			h.cap = h.len
		}
		val, ok := makeHdr(h, t)
		if !ok {
			unsupported("unsafe: reading an incomplete string or slice header as a %s", t)
		}
		if v.m != nil && h.et != nil && !isString(t) && h.len > 0 && oneField(sliceElem(t)) != nil {
			// a []*T read as a []pointer: offsets from the elements reach
			// the fields of each T
			v.m.hintElems(h.elems[:h.len], h.et)
		}
		return val
	}
	if isWord(t) {
		return v.word(h, 0, t)
	}
	ws := hdrWords(t)
	out := make([]Value, len(ws))
	for i, wt := range ws {
		out[i] = v.word(h, i, wt)
	}
	return v.m.fromWords(out, t)
}

// fromWords returns the struct or array of type t made of the words ws. A
// struct of a native named type (reflect.SliceHeader in a plugin) lives in
// native memory (Machine.zero).
func (m *Machine) fromWords(ws []Value, t types.Type) Value {
	if m == nil {
		return Value{R: ws}
	}
	z, ok := m.zero(t).R.(reflect.Value)
	if !ok {
		return Value{R: ws}
	}
	rv := reflect.New(z.Type()).Elem()
	for i, wt := range hdrWords(t) {
		rv.Field(i).Set(m.toNative(ws[i], wt))
	}
	return Value{R: rv}
}

// words returns the words of x, a value of type t (hdrWords).
func (m *Machine) words(x Value, t types.Type) []Value {
	ws := hdrWords(t)
	if isWord(t) {
		return []Value{x}
	}
	switch r := x.R.(type) {
	case []Value:
		return r
	case reflect.Value:
		out := make([]Value, len(ws))
		for i, wt := range ws {
			if r.Kind() == reflect.Array {
				out[i] = m.fromNative(r.Index(i), wt)
			} else {
				out[i] = m.fromNative(r.Field(i), wt)
			}
		}
		return out
	}
	unsupported("unsafe: header words of a %T", x.R)
	return nil
}

// store writes x, of type t, through the view.
func (v *hdrView) store(x Value, t types.Type) {
	if types.Identical(t.Underlying(), v.src.Underlying()) {
		*v.cell = x
		v.st.pend, v.st.ipend = nil, nil
		return
	}
	if isIface(v.src) {
		v.ifaceStore(x, t)
		return
	}
	if isHdrSrc(t) {
		h := hdrOf(x, t)
		if isString(t) && !isString(v.src) {
			h.cap = h.len
		}
		v.set(h, wroteAll)
		return
	}
	h := v.cur()
	wrote := wroteData
	if !isWord(t) {
		fs := v.m.words(x, t)
		wrote = 0
		for i, wt := range hdrWords(t) {
			v.setWord(&h, i, fs[i], wt)
			wrote |= 1 << i
		}
	} else {
		v.setWord(&h, 0, x, t)
	}
	v.set(h, wrote)
}

// wordAddr returns the address of word i of the view (an array of words).
func (v *hdrView) wordAddr(i int64) Value {
	n := len(hdrWords(v.t))
	if isIface(v.src) {
		n = len(ifaceWords(v.t))
	}
	if i < 0 || i >= int64(n) {
		panic(runtimeError("index out of range"))
	}
	return Value{R: &hdrWord{v: v, word: int(i)}}
}

func (w *hdrWord) load(t types.Type) Value {
	if isIface(w.v.src) {
		return w.v.m.ifaceWord(w.v.ifaceCur()[ifaceSlot(w.v.t, w.word)], t)
	}
	return w.v.word(w.v.cur(), w.word, t)
}

func (w *hdrWord) store(x Value, t types.Type) {
	if isIface(w.v.src) {
		ws := w.v.ifaceCur()
		ifaceSetWord(&ws, ifaceSlot(w.v.t, w.word), x, t)
		w.v.ifaceSet(ws)
		return
	}
	h := w.v.cur()
	w.v.setWord(&h, w.word, x, t)
	w.v.set(h, 1<<w.word)
}

// fromHeader reads the header-like value that p points to (of type from)
// as a value of type to (a string or a slice). The result is a new
// variable: a later write through it does not change the header.
func (m *Machine) fromHeader(p Value, from, to types.Type) Value {
	switch q := p.R.(type) {
	case nil:
		return Value{}
	case *hdrView:
		return q.as(to)
	case *Value:
		if isStrHdr(from) {
			// {string, cap}: the data and the length of the string, then
			// the capacity, which must stay inside the string (makeHdr).
			fs, ok := q.R.([]Value)
			if !ok {
				unsupported("unsafe: header words of a %T", q.R)
			}
			h := hdrOf(fs[0], types.Typ[types.String])
			h.cap = int(fs[1].N)
			if isString(to) {
				h.cap = h.len
			}
			val, ok := makeHdr(h, to)
			if !ok {
				unsupported("unsafe: reading an incomplete header as a %s", to)
			}
			return Value{R: &val}
		}
		ws := hdrWords(from)
		fs := m.words(*q, from)
		v := &hdrView{m: m, src: to}
		h := hdr{known: true}
		for i, wt := range ws {
			if i == 2 && isString(to) {
				break
			}
			v.setWord(&h, i, fs[i], wt)
		}
		if len(ws) < 3 {
			h.cap = h.len
		}
		val, ok := makeHdr(h, to)
		if !ok {
			unsupported("unsafe: reading an incomplete header as a %s", to)
		}
		return Value{R: &val}
	}
	unsupported("unsafe: header view of a %T", p.R)
	return Value{}
}
