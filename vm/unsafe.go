package vm

// Emulation of package unsafe on the VM value model (see docs/UNSAFE.md).
//
// Interpreted memory is not bytes: a variable is a Value cell, a struct or
// an array a []Value of cells, a []byte a []byte or a []Value. An
// unsafe.Pointer is the interpreted pointer itself (a *Value, or a *byte
// into a []byte or into the bytes of a string). What can be emulated:
//
//   - uintptr(p) is the real address of the cell, so printing, comparing and
//     hashing addresses work. The conversion records the memory reachable
//     from p (a "region": the cell and its static type, or the backing array
//     of a slice when p is &s[i]) so that unsafe.Pointer(uintptr(p) + off)
//     and unsafe.Add(p, off) resolve the offset, computed with gc's sizes,
//     to the cell of the field or element at that offset.
//   - Conversions between pointer types whose values have the same VM layout
//     (scalars of the same size, structs of such fields...).
//   - Views of the header of a string or a slice (unsafehdr.go):
//     reflect.StringHeader, reflect.SliceHeader, the []byte <-> string
//     zero-copy idioms, *(*uintptr)(unsafe.Pointer(&s)).
//   - unsafe.Slice and unsafe.String on memory whose extent the VM knows
//     (always checked: the result never reaches outside a real Go array).
//
// Everything that needs byte-level memory (viewing a [16]byte as a struct, a
// pointer to the middle of a scalar, offsets outside the recorded object)
// is reported as unsupported instead of corrupting the VM.

import (
	"errors"
	"fmt"
	"go/types"
	"math"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"

	"golang.org/x/tools/go/ssa"
)

// gcSizes are the sizes of gc, computed once per type: every pointer
// arithmetic on a struct (a region lookup, protobuf's pointer.Apply) needs
// the size and the field offsets of its type, and go/types computes them
// again at each call (it walked the whole message type, most of the time
// of a protobuf Marshal).
var gcSizes types.Sizes = &cachedSizes{s: types.SizesFor("gc", runtime.GOARCH)}

// The caches keep no type alive (the types of a plugin go away with it,
// TestLoadReleasesTypes): an entry is found by the address of the type (or
// of the first field) and checked by a weak pointer to it.
type cachedSizes struct {
	s       types.Sizes
	sizes   weakCache[int64]
	aligns  weakCache[int64]
	offsets weakCache[offsetsEntry]
}

// offsetsEntry has the offsets of a list of fields, with weak pointers to
// all of them: the instances of a generic struct share the fields whose
// type does not depend on a type parameter, so the first field does not
// name the list.
type offsetsEntry struct {
	fields []weak.Pointer[types.Var]
	offs   []int64
}

func (f offsetsEntry) same(fields []*types.Var) bool {
	for i, w := range f.fields {
		if w.Value() != fields[i] {
			return false
		}
	}
	return true
}

// weakCache maps objects, by address, to values.
type weakCache[V any] struct {
	m    sync.Map // uintptr -> *weakEntry[V]
	adds atomic.Int64
}

type weakEntry[V any] struct {
	w weak.Pointer[byte]
	n int // a second key (the number of fields)
	v V
}

func (c *weakCache[V]) load(p unsafe.Pointer, n int) (V, bool) {
	if e, ok := c.m.Load(uintptr(p)); ok {
		if e := e.(*weakEntry[V]); e.n == n && unsafe.Pointer(e.w.Value()) == p {
			return e.v, true
		}
	}
	var zero V
	return zero, false
}

func (c *weakCache[V]) store(p unsafe.Pointer, n int, v V) {
	c.m.Store(uintptr(p), &weakEntry[V]{w: weak.Make((*byte)(p)), n: n, v: v})
	if c.adds.Add(1)%4096 == 0 {
		c.m.Range(func(k, e any) bool {
			if e.(*weakEntry[V]).w.Value() == nil {
				c.m.CompareAndDelete(k, e)
			}
			return true
		})
	}
}

// typePtr returns the pointer behind t, or nil for a basic type (a global
// of go/types: cheap to compute, and not in the heap).
func typePtr(t types.Type) unsafe.Pointer {
	if _, ok := t.(*types.Basic); ok || t == nil {
		return nil
	}
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1]
}

func (c *cachedSizes) Sizeof(t types.Type) int64 {
	p := typePtr(t)
	if p == nil {
		return c.s.Sizeof(t)
	}
	if n, ok := c.sizes.load(p, 0); ok {
		return n
	}
	n := c.s.Sizeof(t)
	c.sizes.store(p, 0, n)
	return n
}

func (c *cachedSizes) Alignof(t types.Type) int64 {
	p := typePtr(t)
	if p == nil {
		return c.s.Alignof(t)
	}
	if n, ok := c.aligns.load(p, 0); ok {
		return n
	}
	n := c.s.Alignof(t)
	c.aligns.store(p, 0, n)
	return n
}

// Offsetsof returns the offsets of fields; the slice is shared and must
// not be changed. The entry is found by the last field and the count, and
// checked field by field.
func (c *cachedSizes) Offsetsof(fields []*types.Var) []int64 {
	if len(fields) == 0 {
		return nil
	}
	p := unsafe.Pointer(fields[len(fields)-1])
	if f, ok := c.offsets.load(p, len(fields)); ok && f.same(fields) {
		return f.offs
	}
	f := offsetsEntry{fields: make([]weak.Pointer[types.Var], len(fields)), offs: c.s.Offsetsof(fields)}
	for i, v := range fields {
		f.fields[i] = weak.Make(v)
	}
	c.offsets.store(p, len(fields), f)
	return f.offs
}

// uregion is memory reachable by unsafe arithmetic from a pointer.
type uregion struct {
	base  uintptr    // address of offset 0, as seen by the program
	elems []Value    // cells from offset 0 (one for a lone variable)
	bytes []byte     // or bytes from offset 0
	ro    bool       // the bytes are read-only (string data outside the heap)
	t     types.Type // type of each element of elems (nil: unknown)
}

// roByte is a pointer to a byte of a string that lives outside the Go heap
// (a string constant of the host, in read-only memory). It can be read,
// compared and printed, but the VM refuses a write through it: in gc the
// write stops the process with a fault that nothing can recover.
type roByte *byte

// bytePtr is the interpreted pointer to the byte p.
func bytePtr(p *byte, ro bool) Value {
	if ro {
		return Value{R: roByte(p)}
	}
	return Value{R: p}
}

// rawByte returns the Go pointer of a pointer to a byte, or of a view of
// plain data over bytes (byteview.go).
func rawByte(v Value) (*byte, bool) {
	switch p := v.R.(type) {
	case *byte:
		return p, true
	case roByte:
		return p, true
	case *byteView:
		return p.ptr(), true
	}
	return nil, false
}

// errROToNative refuses a pointer to read-only bytes given to native code.
var errROToNative = errors.New("unsafe: a pointer to the bytes of a string in read-only memory cannot go to native code (a write there stops the process)")

func refuseROWrite() {
	unsupported("unsafe: writing into the bytes of a string that lives in read-only memory")
}

// strReadOnly reports whether p, the data of a non-empty string, is
// outside the Go heap (a constant: writing there is a fault).
func strReadOnly(p *byte) bool { return !inHeap(p) }

// span is the size of the region in bytes (gc layout).
func (r *uregion) span() int64 {
	switch {
	case r.bytes != nil:
		return int64(len(r.bytes))
	case r.t == nil:
		return 1
	}
	return int64(len(r.elems)) * gcSizes.Sizeof(r.t)
}

// resolve returns the interpreted pointer at byte offset off of the region.
func (r *uregion) resolve(off int64) Value {
	v, why := r.lookup(off)
	if why != "" {
		unsupported("%s", why)
	}
	return v
}

// lookup is resolve, with the reason when there is no cell at offset off.
func (r *uregion) lookup(off int64) (Value, string) {
	v, _, why := r.lookupT(off)
	return v, why
}

// lookupT is lookup that also returns the type of the cell (nil when the
// region does not know it).
func (r *uregion) lookupT(off int64) (Value, types.Type, string) {
	if r.bytes != nil {
		if off < 0 || off >= int64(len(r.bytes)) {
			return Value{}, nil, fmt.Sprintf("unsafe: offset %d outside of a %d-byte object", off, len(r.bytes))
		}
		return bytePtr(&r.bytes[off], r.ro), nil, ""
	}
	if off == 0 {
		return Value{R: &r.elems[0]}, r.t, ""
	}
	if r.t == nil {
		return Value{}, nil, "unsafe: pointer arithmetic on a pointer of unknown type"
	}
	sz := gcSizes.Sizeof(r.t)
	idx := off / sz
	if off < 0 || idx >= int64(len(r.elems)) {
		return Value{}, nil, fmt.Sprintf("unsafe: offset %d outside of the object (%d bytes)", off, r.span())
	}
	return descend(&r.elems[idx], r.t, off-idx*sz)
}

// pointerAt returns the unsafe.Pointer at offset off of the region: the
// cell there, or a badAddr when the offset is inside a scalar or outside
// the region (gc allows such a pointer; only a dereference is refused).
// The type of the cell is nil when it is not known.
func (r *uregion) pointerAt(off int64) (Value, types.Type) {
	if v, t, why := r.lookupT(off); why == "" {
		return v, t
	}
	return Value{R: badAddr(r.base + uintptr(off))}, nil
}

// reached records the memory of the pointer that the cell p, of type t,
// holds, when the program reached p by address arithmetic: in gc the
// pointer read through p (protobuf's pointer.Elem() on a message field)
// can be offset again to reach the fields of its own struct.
func (m *Machine) reached(p Value, t types.Type) {
	if v, ok := p.R.(*hdrView); ok && v.m == nil {
		v.m = m // a slice field (descend): its view can record regions
	}
	if w, ok := p.R.(*hdrWord); ok && w.v.m == nil {
		w.v.m = m // a word of a header or of an interface (descend)
	}
	if c, ok := p.R.(*Value); ok && c != nil && t != nil {
		if pt, ok := t.Underlying().(*types.Pointer); ok && isAggregate(pt.Elem()) {
			m.rememberPointee(*c, t)
		}
	}
}

// index returns the index of the element p points to in the region, using
// the real Go addresses (not the addresses the program sees).
func (r *uregion) index(p Value) (int, bool) {
	if q, ok := rawByte(p); ok {
		if len(r.bytes) == 0 {
			return 0, false
		}
		d := uintptr(unsafe.Pointer(q)) - uintptr(unsafe.Pointer(&r.bytes[0]))
		if d < uintptr(len(r.bytes)) {
			return int(d), true
		}
		return 0, false
	}
	switch q := p.R.(type) {
	case *Value:
		if len(r.elems) == 0 {
			return 0, false
		}
		const sz = unsafe.Sizeof(Value{})
		d := uintptr(unsafe.Pointer(q)) - uintptr(unsafe.Pointer(&r.elems[0]))
		if d%sz == 0 && d/sz < uintptr(len(r.elems)) {
			return int(d / sz), true
		}
	}
	return 0, false
}

// offsetOf returns the offset (gc layout) of the element p points to.
func (r *uregion) offsetOf(p Value) (int64, bool) {
	i, ok := r.index(p)
	switch {
	case !ok:
		return 0, false
	case r.bytes != nil || i == 0:
		return int64(i), true
	case r.t == nil:
		return 0, false
	}
	return int64(i) * gcSizes.Sizeof(r.t), true
}

// at returns the region seen from the pointer p it holds. Its base is the
// real address of its first element, so that each element has one address
// in the region, base + its gc offset, whichever element p is: for an array
// stored as cells, &a[1] is base + 1 for a [N]byte, not the address of its
// cell, 24 bytes after a[0] (GOROOT test fixedbugs/issue36516.go).
func (r *uregion) at(p Value) *uregion {
	i, ok := r.index(p)
	if !ok {
		return nil
	}
	if r.bytes == nil && i > 0 && r.t == nil {
		return nil // no gc offset without the element type
	}
	real := uintptr(i)
	if r.bytes == nil {
		real *= unsafe.Sizeof(Value{})
	}
	c := *r
	c.base = addrOf(p) - real
	return &c
}

// addrIn returns the address the program sees for p in the region: base +
// the gc offset of p.
func (r *uregion) addrIn(p Value) (uintptr, bool) {
	off, ok := r.offsetOf(p)
	if !ok {
		return 0, false
	}
	return r.base + uintptr(off), true
}

// descend returns the cell at byte offset off of the value of type t held
// by cell, or the reason why there is none.
func descend(cell *Value, t types.Type, off int64) (Value, types.Type, string) {
	if off == 0 {
		if _, ok := t.Underlying().(*types.Slice); ok {
			// a slice field: a view of its header, so that its words can be
			// read (protobuf's pointer.Elem().IsNil() on a repeated field)
			// and that it is seen as a slice of another element type
			return Value{R: &hdrView{cell: cell, src: t, t: t, st: &hdrState{}}}, t, ""
		}
		return Value{R: cell}, t, ""
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		fs, ok := cell.R.([]Value)
		if !ok {
			break
		}
		vars := make([]*types.Var, u.NumFields())
		for i := range vars {
			vars[i] = u.Field(i)
		}
		offs := gcSizes.Offsetsof(vars)
		for i := len(offs) - 1; i >= 0; i-- {
			if offs[i] <= off {
				if off-offs[i] < gcSizes.Sizeof(vars[i].Type()) {
					return descend(&fs[i], vars[i].Type(), off-offs[i])
				}
				break
			}
		}
	case *types.Array:
		es, ok := cell.R.([]Value)
		if !ok {
			break
		}
		esz := gcSizes.Sizeof(u.Elem())
		if i := off / esz; i < int64(len(es)) {
			return descend(&es[i], u.Elem(), off-i*esz)
		}
	case *types.Interface:
		// the data word of an interface (ifaceview.go), reached by
		// uintptr(unsafe.Pointer(&i)) + 8; the type word is the cell
		// itself (off 0)
		if off == ptrSize {
			v := &hdrView{cell: cell, src: t, t: ifaceWordsType, st: &hdrState{}}
			return Value{R: &hdrWord{v: v, word: 1}}, nil, ""
		}
	case *types.Basic, *types.Slice:
		// the length or the capacity word of a string or slice header
		if isHdrSrc(t) && off%ptrSize == 0 && off < gcSizes.Sizeof(t) {
			v := &hdrView{cell: cell, src: t, t: t, st: &hdrState{}}
			return Value{R: &hdrWord{v: v, word: int(off / ptrSize)}}, nil, ""
		}
	}
	return Value{}, nil, fmt.Sprintf("unsafe: offset %d inside a %s value needs byte-level memory", off, t)
}

// addrOf is the address the program sees for an interpreted pointer.
func addrOf(v Value) uintptr {
	switch p := v.R.(type) {
	case *Value:
		if rv, ok := p.R.(reflect.Value); ok && rv.Kind() == reflect.Struct && rv.CanAddr() {
			// a variable of a native struct type (syscall.Timespec,
			// zeroValue): its memory is native, with gc's layout. Its
			// address is the address of that memory, as for the
			// addresses of its fields (fieldAddr), so that native code
			// that gets it as a uintptr (syscall.Syscall) writes the
			// variable, not the cell.
			return rv.UnsafeAddr()
		}
		return uintptr(unsafe.Pointer(p))
	case *byte:
		return uintptr(unsafe.Pointer(p))
	case roByte:
		return uintptr(unsafe.Pointer(p))
	case reflect.Value:
		return p.Pointer()
	case *hdrView:
		return uintptr(unsafe.Pointer(p.cell))
	case *hdrWord:
		return uintptr(unsafe.Pointer(p.v.cell)) + uintptr(p.word)*uintptr(ptrSize)
	case *byteView:
		return p.addr()
	case badAddr:
		return uintptr(p)
	case *MapV:
		// a map seen as an unsafe.Pointer: in gc a map is a pointer
		return uintptr(unsafe.Pointer(p))
	}
	return 0
}

// realMemory reports whether the pointer p points to memory with gc's
// layout, which native code may read and write at the address addrOf(p):
// a byte of a []byte or of a string, plain data in real bytes (byteView), a
// native value, a variable of a native struct type, or an address that is
// not in memory of the VM (badAddr, as in gc). A cell of the VM is not.
func realMemory(p Value) bool {
	switch q := p.R.(type) {
	case nil, *byte, roByte, *byteView, reflect.Value, badAddr:
		return true
	case *Value:
		rv, ok := q.R.(reflect.Value)
		return ok && rv.Kind() == reflect.Struct && rv.CanAddr()
	}
	return false
}

// isSyscallFunc reports whether the native function fn is a system call
// entry point of package syscall (Syscall, Syscall6, RawSyscall...): the
// kernel uses a uintptr argument of it as an address (unsafeToUintptr).
func isSyscallFunc(fn *ssa.Function) bool {
	return fn.Pkg != nil && fn.Pkg.Pkg.Path() == "syscall"
}

// badAddr is an unsafe.Pointer made from an address that is not in memory
// known to the VM (a constant, an address that native code returned, an
// address computed too far from its object). As in gc, the pointer can be
// kept, compared and printed; unlike gc, it can never be dereferenced or
// given to native code: the VM refuses (unsupported) instead of reading or
// writing memory it does not own.
type badAddr uintptr

func (b badAddr) refuse() {
	unsupported("unsafe: pointer %#x does not point to a value known to the VM", uintptr(b))
}

// uregions remembers the last regions converted to uintptr, newest first.
// As in gc, a uintptr does not keep its object alive: the regions in the
// heap are held by weak pointers (a finalizer of the object still runs:
// GOROOT test fixedbugs/issue24491b.go), and the others (globals, string
// constants, where weak.Make would stop the process) are never freed.
type uregions struct {
	mu   sync.Mutex
	ring [64]uref
	next int
}

// uref is a region in the ring.
type uref struct {
	strong *uregion // memory outside the heap, or nil
	base   uintptr
	t      types.Type
	n      int
	// span is the extent the program sees from base, lo and hi the real
	// addresses of the memory: the lookups compare them before they make
	// the weak pointer strong (most entries do not match).
	span   int64
	lo, hi uintptr
	// wb or we point to the last element of the capacity of the region, in
	// the heap, and back is its index from the first element. The subslices
	// s[i:] of one array share that element: the runtime keeps one special
	// per address of an object, and searches them linearly, so a weak
	// pointer to the first element of each subslice made slices.Insert in
	// a loop quadratic.
	wb   weak.Pointer[byte]
	we   weak.Pointer[Value]
	back int
}

// get sets r to the region; false when it is not used or freed. It does
// not allocate: uintptr conversions in a loop stay cheap.
func (e *uref) get(r *uregion) bool {
	if e.strong != nil {
		*r = *e.strong
		return true
	}
	// One call of Value: the object can be freed between two calls.
	if p := e.wb.Value(); p != nil {
		first := (*byte)(unsafe.Add(unsafe.Pointer(p), -e.back))
		*r = uregion{base: e.base, bytes: unsafe.Slice(first, e.n), t: e.t}
		return true
	}
	if p := e.we.Value(); p != nil {
		first := (*Value)(unsafe.Add(unsafe.Pointer(p), -e.back*int(unsafe.Sizeof(Value{}))))
		*r = uregion{base: e.base, elems: unsafe.Slice(first, e.n), t: e.t}
		return true
	}
	return false
}

// inHeap reports whether p points into the Go heap. runtime.AddCleanup
// accepts heap pointers, does nothing for globals and panics for other
// memory (string constants); the cleanup is stopped at once.
//
// This is not a documented behavior of AddCleanup, so inHeapOK checks it
// when package vm starts. When a Go release changes it, inHeap is false for
// every pointer (fail closed): the bytes of every string are then read-only
// for unsafe writes, and the VM keeps the regions of unsafe views alive
// (strong references) instead of weak pointers to them. It never calls
// weak.Make on memory that is not in the heap, and never lets a write reach
// a string constant.
func inHeap[T any](p *T) bool {
	return inHeapOK && cleanupInHeap(p)
}

// cleanupInHeap is the AddCleanup test of inHeap, without the start-up
// check.
func cleanupInHeap[T any](p *T) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	c := runtime.AddCleanup(p, func(int) {}, 0)
	c.Stop()
	return c != runtime.Cleanup{}
}

// inHeapOK reports whether cleanupInHeap tells the three kinds of memory
// apart on this runtime: a heap object, a global variable and a string
// constant.
var inHeapOK = probeInHeap(cleanupInHeap[byte])

// inHeapProbeGlobal is the global variable of the probe.
var inHeapProbeGlobal [16]byte

// probeInHeap reports whether test gives true for a heap pointer and false
// for a global and for the bytes of a string constant. Like the other
// start-up probes (funcpc.go), a panic only makes it fail.
func probeInHeap(test func(*byte) bool) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	heap := new([16]byte)
	const s = "vm: probe of the read-only memory of string constants"
	return test(&heap[0]) && !test(&inHeapProbeGlobal[0]) && !test(unsafe.StringData(s))
}

func (rs *uregions) add(r *uregion) {
	e := uref{base: r.base, t: r.t, span: r.span()}
	switch {
	case len(r.bytes) > 0:
		e.lo = uintptr(unsafe.Pointer(&r.bytes[0]))
		e.hi = e.lo + uintptr(len(r.bytes))
	case len(r.elems) > 0:
		e.lo = uintptr(unsafe.Pointer(&r.elems[0]))
		e.hi = e.lo + uintptr(len(r.elems))*unsafe.Sizeof(Value{})
	}
	switch {
	case len(r.bytes) > 0 && inHeap(lastOfCap(r.bytes)):
		e.n, e.back, e.wb = len(r.bytes), cap(r.bytes)-1, weak.Make(lastOfCap(r.bytes))
	case len(r.elems) > 0 && inHeap(lastOfCap(r.elems)):
		e.n, e.back, e.we = len(r.elems), cap(r.elems)-1, weak.Make(lastOfCap(r.elems))
	default:
		e.strong = r
	}
	rs.mu.Lock()
	rs.ring[rs.next%len(rs.ring)] = e
	rs.next++
	rs.mu.Unlock()
}

// lastOfCap returns the last element of the capacity of s (len(s) > 0).
func lastOfCap[T any](s []T) *T { return &s[:cap(s)][cap(s)-1] }

// find returns the newest region that holds the address addr, as the
// program sees addresses.
func (rs *uregions) find(addr uintptr) *uregion {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := 1; i <= len(rs.ring); i++ {
		var r uregion
		if e := rs.entry(i); e.covers(addr) && e.get(&r) {
			c := r
			return &c
		}
	}
	return nil
}

// covers reports whether the entry, if it is still alive, holds the
// address addr as the program sees it.
func (e *uref) covers(addr uintptr) bool {
	return addr >= e.base && int64(addr-e.base) < e.span
}

// findPtr returns the newest region whose memory holds the interpreted
// pointer p (real Go addresses: the answer is exact).
func (rs *uregions) findPtr(p Value) *uregion {
	if p.R == nil {
		return nil
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := 1; i <= len(rs.ring); i++ {
		var r uregion
		if e := rs.entry(i); e.holdsReal(p) && e.get(&r) && r.holds(p) {
			c := r
			return &c
		}
	}
	return nil
}

// pointerAt returns unsafe.Pointer(addr): the cell of the newest region
// that holds addr, and its type when known, else a badAddr. It does not
// allocate.
func (rs *uregions) pointerAt(addr uintptr) (Value, types.Type) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := 1; i <= len(rs.ring); i++ {
		var r uregion
		if e := rs.entry(i); e.covers(addr) && e.get(&r) {
			return r.pointerAt(int64(addr - r.base))
		}
	}
	return Value{R: badAddr(addr)}, nil
}

// holdsReal reports whether the memory of the entry may hold p, by real
// address (a strong entry, out of the heap, is always looked at).
func (e *uref) holdsReal(p Value) bool {
	if e.strong != nil {
		return true
	}
	var a uintptr
	switch q := p.R.(type) {
	case *Value:
		a = uintptr(unsafe.Pointer(q))
	default:
		b, ok := rawByte(p)
		if !ok {
			return true
		}
		a = uintptr(unsafe.Pointer(b))
	}
	return a >= e.lo && a < e.hi
}

// entry returns the i-th newest entry (1 is the newest).
func (rs *uregions) entry(i int) *uref {
	return &rs.ring[(rs.next-i+2*len(rs.ring))%len(rs.ring)]
}

// knows reports whether unsafe.Pointer(uintptr(p)) already gives back p,
// in a region that reaches at least need bytes from p, without
// allocating: then there is nothing to record. The region must be the
// newest one: the addresses the program sees for the fields of a struct
// (base + gc offset) can fall inside the cell of another object, and a
// newer region of that object would win the lookup of uintptr(p) + off.
// It also returns the address of p in that region (see uregion.at).
func (rs *uregions) knows(p Value, need int64) (uintptr, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := 1; i <= len(rs.ring); i++ {
		var r uregion
		if !rs.entry(i).get(&r) {
			continue
		}
		off, ok := r.offsetOf(p)
		if !ok {
			addr := addrOf(p)
			if addr < r.base || int64(addr-r.base) >= r.span() {
				return 0, false // a newer region: record p again
			}
			off = int64(addr - r.base)
		}
		v, why := r.lookup(off)
		if why == "" && v.R == p.R && r.span()-off >= need {
			return r.base + uintptr(off), true
		}
		return 0, false
	}
	return 0, false
}

// The ring of a Machine is shared by all the goroutines of the program.
// Under load, the other goroutines push a region out of it between the
// conversion that records it and the one that uses it: gjson's getBytes
// makes the data pointer of a string header the data pointer of a slice
// header, and the second step did not find the memory any more. So each
// goroutine also has a ring of its own (in its gstate, one per Machine):
// a region it recorded stays there until the goroutine itself records 64
// more. Lookups try the ring of the goroutine first, then the ring of the
// Machine (a uintptr that another goroutine converted).

// goRegions is the ring of one goroutine for one Machine.
type goRegions struct {
	m  *Machine
	rs uregions
}

// ring returns the ring of the goroutine for m; nil when it has none and
// create is false.
func (gs *gstate) ring(m *Machine, create bool) *uregions {
	for _, g := range gs.uregs {
		if g.m == m {
			return &g.rs
		}
	}
	if !create {
		return nil
	}
	g := &goRegions{m: m}
	gs.uregs = append(gs.uregs, g)
	return &g.rs
}

// goRing returns the ring of the calling goroutine for m: nil when the
// goroutine runs no interpreted code, or when it has no ring for m and
// create is false.
func (m *Machine) goRing(create bool) *uregions {
	if gs := lookupG(getg()); gs != nil {
		return gs.ring(m, create)
	}
	return nil
}

// addRegion records r in the ring of the goroutine and in the ring of m.
func (m *Machine) addRegion(r *uregion) {
	if g := m.goRing(true); g != nil {
		g.add(r)
	}
	m.uregions.add(r)
}

// findRegion is uregions.find, in the ring of the goroutine first.
func (m *Machine) findRegion(addr uintptr) *uregion {
	if g := m.goRing(false); g != nil {
		if r := g.find(addr); r != nil {
			return r
		}
	}
	return m.uregions.find(addr)
}

// findRegionPtr is uregions.findPtr, in the ring of the goroutine first.
func (m *Machine) findRegionPtr(p Value) *uregion {
	if g := m.goRing(false); g != nil {
		if r := g.findPtr(p); r != nil {
			return r
		}
	}
	return m.uregions.findPtr(p)
}

// regionPointerAt is uregions.pointerAt, in the ring of the goroutine
// first. The pointer the cell holds is recorded (reached).
func (m *Machine) regionPointerAt(addr uintptr) Value {
	if g := m.goRing(false); g != nil {
		if v, t := g.pointerAt(addr); !isBadAddr(v) {
			m.reached(v, t)
			return v
		}
	}
	v, t := m.uregions.pointerAt(addr)
	if isBadAddr(v) {
		// the address of a cell given by reflect.Value.UnsafeAddr
		if c, ct, ok := unsafeCellAt(m, addr); ok {
			v, t = Value{R: c}, ct
		}
	}
	m.reached(v, t)
	return v
}

func isBadAddr(v Value) bool {
	_, ok := v.R.(badAddr)
	return ok
}

// knowsRegion is uregions.knows for the calling goroutine: when it runs
// interpreted code, only its own ring counts (a region that is only in the
// ring of m may be pushed out by the other goroutines before it is used).
func (m *Machine) knowsRegion(p Value, need int64) (uintptr, bool) {
	if gs := lookupG(getg()); gs != nil {
		if g := gs.ring(m, false); g != nil {
			return g.knows(p, need)
		}
		return 0, false
	}
	return m.uregions.knows(p, need)
}

// remember records the region r, seen from the pointer p, for later
// conversions of addresses to pointers (nothing when they already work).
func (m *Machine) remember(r *uregion, p Value) {
	off, ok := r.offsetOf(p)
	if !ok {
		return
	}
	if _, ok := m.knowsRegion(p, r.span()-off); ok {
		return
	}
	if c := r.at(p); c != nil {
		m.addRegion(c)
	}
}

// backing returns the region of the whole backing array of the slice (or
// the array behind the pointer) x, with elements of type et; nil when x
// has no memory the VM can follow.
func backing(x Value, et types.Type) *uregion {
	switch s := x.R.(type) {
	case []Value:
		if cap(s) > 0 {
			return &uregion{elems: s[:cap(s)], t: et}
		}
	case []byte:
		if cap(s) > 0 {
			return &uregion{bytes: s[:cap(s)]}
		}
	case *Value:
		if arr, ok := s.R.([]Value); ok && len(arr) > 0 {
			return &uregion{elems: arr, t: et}
		}
	}
	return nil
}

// strRegion is the region of the bytes of the string s (nil when empty).
// They are read-only in gc. The VM does not check writes into a string
// built at run time (in the heap), as gc does not; the bytes of a string
// outside the heap (a constant of the host) are marked read-only.
func strRegion(s string) *uregion {
	if s == "" {
		return nil
	}
	p := unsafe.StringData(s)
	return &uregion{base: uintptr(unsafe.Pointer(p)), bytes: unsafe.Slice(p, len(s)), ro: strReadOnly(p)}
}

// isPtrLike reports whether t is a pointer type or unsafe.Pointer.
func isPtrLike(t types.Type) bool {
	if isPointer(t.Underlying()) {
		return true
	}
	b := basic(t)
	return b != nil && b.Kind() == types.UnsafePointer
}

// uorigin returns a function computing, at run time, the region that holds
// the pointer v, from its SSA origin: &x[i] (the whole backing array of the
// slice x, or the array *x), unsafe.SliceData(x) (x[:cap(x)]) or
// unsafe.StringData(s) (the bytes of s). Conversions between pointer types
// and unsafe.Pointer are looked through. It returns nil when the SSA does
// not show the origin.
func (c *compiler) uorigin(v ssa.Value) func(regs []Value) *uregion {
	for {
		cv, ok := v.(*ssa.Convert)
		if !ok || !isPtrLike(cv.X.Type()) || !isPtrLike(cv.Type()) {
			break
		}
		v = cv.X
	}
	switch o := v.(type) {
	case *ssa.IndexAddr:
		var et types.Type
		switch u := o.X.Type().Underlying().(type) {
		case *types.Slice:
			et = u.Elem()
		case *types.Pointer:
			et = u.Elem().Underlying().(*types.Array).Elem()
		}
		br := c.reg(o.X)
		return func(regs []Value) *uregion { return backing(regs[br], et) }
	case *ssa.Call:
		b, ok := o.Call.Value.(*ssa.Builtin)
		if !ok || len(o.Call.Args) != 1 {
			break
		}
		xr := c.reg(o.Call.Args[0])
		switch b.Name() {
		case "SliceData":
			et := sliceElem(o.Call.Args[0].Type())
			return func(regs []Value) *uregion { return backing(regs[xr], et) }
		case "StringData":
			return func(regs []Value) *uregion { return strRegion(valStr(regs[xr])) }
		}
	}
	return nil
}

// uprov returns a function computing, at run time, the region of the
// unsafe.Pointer value v seen from v: its SSA origin (uorigin), else a
// recent region that holds it, else the lone value of its static type
// (&x of type *T), else nil.
func (c *compiler) uprov(v ssa.Value) func(regs []Value) *uregion {
	pr, m := c.reg(v), c.m
	var t types.Type
	if cv, ok := v.(*ssa.Convert); ok {
		if pt, ok := cv.X.Type().Underlying().(*types.Pointer); ok {
			t = pt.Elem()
		}
	}
	orig := c.uorigin(v)
	return func(regs []Value) *uregion {
		p := regs[pr]
		if orig != nil {
			if r := orig(regs); r != nil {
				if r = r.at(p); r != nil {
					return r
				}
			}
		}
		if r := m.findRegionPtr(p); r != nil {
			if r = r.at(p); r != nil {
				return r
			}
		}
		return lone(p, t)
	}
}

// lone is the region of a pointer to a single value of type t (nil:
// unknown).
func lone(p Value, t types.Type) *uregion {
	r := &uregion{base: addrOf(p), t: t}
	switch c := p.R.(type) {
	case *Value:
		r.elems = unsafe.Slice(c, 1)
	case *byte:
		r.bytes = unsafe.Slice(c, 1)
	case roByte:
		r.bytes, r.ro = unsafe.Slice((*byte)(c), 1), true
	case *byteView:
		return c.region() // the view knows its whole array
	default:
		return nil
	}
	return r
}

// ptrArrayElem returns the element type when the unsafe.Pointer v is the
// conversion of a pointer to a non-empty array (unsafe.Pointer(&a), with a
// of type [N]T); nil otherwise.
func ptrArrayElem(v ssa.Value) types.Type {
	for {
		cv, ok := v.(*ssa.Convert)
		if !ok || !isPtrLike(cv.X.Type()) {
			return nil
		}
		if pt, ok := cv.X.Type().Underlying().(*types.Pointer); ok {
			if at, ok := pt.Elem().Underlying().(*types.Array); ok && at.Len() > 0 {
				return at.Elem()
			}
			return nil
		}
		v = cv.X
	}
}

// unsafeToUintptr compiles uintptr(p) for an unsafe.Pointer p.
func (c *compiler) unsafeToUintptr(dst int32, conv *ssa.Convert) {
	p := conv.X
	prov, pr, m := c.uprov(p), c.reg(p), c.m
	orig := c.uorigin(p)
	static := orig != nil
	arrElem := ptrArrayElem(p)
	toSyscall, what := false, ""
	for _, w := range *conv.Referrers() {
		call, ok := w.(ssa.CallInstruction)
		if !ok || !c.nativeArg(call.Common(), conv) || !isSyscallFunc(call.Common().Value.(*ssa.Function)) {
			continue
		}
		toSyscall = true
		what = p.Type().String()
		if pc, ok := p.(*ssa.Convert); ok {
			what = pc.X.Type().String() // the *T of unsafe.Pointer(&x)
		}
		what += " given to " + call.Common().Value.(*ssa.Function).String()
	}
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		if toSyscall && !realMemory(regs[pr]) {
			// syscall.Syscall(..., uintptr(unsafe.Pointer(p)), ...): the
			// kernel reads and writes the memory at the address with gc's
			// layout. A cell of the VM is not that memory: a write there
			// would break the VM, so refuse before the call (fail closed).
			// Other native functions take a uintptr as a number
			// (sync/atomic.StoreUintptr): a cell address is fine there, the
			// program gets it back with unsafe.Pointer(u).
			unsupported("unsafe: the address of a %s as a uintptr: the value is not in real memory (only plain data in a local variable, a []byte or a native value are)", what)
		}
		p := regs[pr]
		if arrElem != nil && !originHolds(orig, regs, p) {
			// A pointer to an array of cells: in gc, the array and its
			// first element have the same address. The address is the one
			// of &a[0] in the region of the array (the real address of
			// the cell of a[0], see uregion.at), so that uintptr(&a[i]) -
			// uintptr(&a) is the gc offset of a[i], and so that the data
			// word of a[0:0:0] is the address of the array (GOROOT test
			// slice3.go). The uintptr carries the array pointer, so that
			// unsafe.Pointer(u) gives back the array (uintptrToUnsafe).
			// When the array is itself an element of an array the SSA
			// shows (&g[1] of a [2][3]T), the usual path below gives its
			// gc offset in that array, as for &g[0]: uintptr(&g[1]) -
			// uintptr(&g[0]) is Sizeof(g[0]).
			if cell, ok := p.R.(*Value); ok {
				if arr, ok := cell.R.([]Value); ok && len(arr) > 0 {
					e0 := Value{R: &arr[0]}
					if _, ok := m.knowsRegion(e0, 0); !ok {
						m.remember((&uregion{elems: arr, t: arrElem}).at(e0), e0)
					}
					return Value{N: int64(addrOf(e0)), R: cell}
				}
			}
		}
		// Without an SSA origin, the region would come from the ring: when
		// the ring already gives back the pointer, there is nothing to do
		// (no allocation in a loop of conversions). The address is the one
		// of p in its region (uregion.at).
		if !static {
			if a, ok := m.knowsRegion(p, 0); ok {
				return Value{N: int64(a)}
			}
		}
		// a pointer of unknown type may have a hint (ptrHints)
		r := prov(regs)
		if r != nil && (r.t != nil || r.bytes != nil) {
			m.remember(r, p)
		} else if !m.fromHint(p) && r != nil {
			m.remember(r, p)
		}
		if r != nil {
			if a, ok := r.addrIn(p); ok {
				return Value{N: int64(a)}
			}
		}
		return Value{N: int64(addrOf(p))}
	}})
}

// toUnsafe compiles unsafe.Pointer(p) for a pointer p. The cell is shared;
// when the SSA shows where p points (&s[i], unsafe.SliceData...), the
// region is recorded, so that a header that gets this pointer as its data
// knows how far the memory goes.
func (c *compiler) toUnsafe(dst int32, p ssa.Value) {
	pr := c.reg(p)
	orig := c.uorigin(p)
	if orig == nil {
		if pt := p.Type().Underlying().(*types.Pointer); isAggregate(pt.Elem()) {
			// unsafe.Pointer(x) of a *S whose origin the SSA does not
			// show (a parameter): remember the struct, so that pointer
			// arithmetic with gc offsets and a view of its first field
			// work later (protoimpl.X.MessageStateOf(protoimpl.Pointer(x))).
			m, t := c.m, p.Type()
			c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
				m.rememberPointee(regs[pr], t)
				return regs[pr]
			}})
			return
		}
		c.emit(Instr{Op: OpMov, A: dst, B: pr})
		return
	}
	m := c.m
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		if r := orig(regs); r != nil {
			m.remember(r, regs[pr])
		}
		return regs[pr]
	}})
}

// originHolds reports whether the region of the SSA origin orig (nil: none)
// holds the pointer p: p is an element of an array or slice the SSA shows.
func originHolds(orig func(regs []Value) *uregion, regs []Value, p Value) bool {
	if orig == nil {
		return false
	}
	r := orig(regs)
	return r != nil && r.holds(p)
}

// arrayAt reports whether p is a pointer to an array of cells whose first
// element is at the address addr (the uintptr of a pointer to an array,
// see unsafeToUintptr).
func arrayAt(p any, addr uintptr) bool {
	if c, ok := p.(*Value); ok {
		if arr, ok := c.R.([]Value); ok && len(arr) > 0 {
			return uintptr(unsafe.Pointer(&arr[0])) == addr
		}
	}
	return false
}

// uintptrToUnsafe compiles unsafe.Pointer(u) for a uintptr u.
func (c *compiler) uintptrToUnsafe(dst, u int32) {
	m := c.m
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		addr := uintptr(regs[u].N)
		if addr == 0 {
			return Value{}
		}
		if p := regs[u].R; p != nil && (addrOf(Value{R: p}) == addr || arrayAt(p, addr)) {
			return Value{R: p}
		}
		return m.regionPointerAt(addr)
	}})
}

// unsafeAdd compiles unsafe.Add(p, n).
func (c *compiler) unsafeAdd(dst int32, p, n ssa.Value) {
	prov, pr, nr, m := c.uprov(p), c.reg(p), c.reg(n), c.m
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		off := regs[nr].N
		if off == 0 || regs[pr].R == nil {
			return regs[pr]
		}
		if b, ok := regs[pr].R.(badAddr); ok {
			return Value{R: b + badAddr(off)}
		}
		r := prov(regs)
		if r == nil {
			unsupported("unsafe.Add on a pointer the VM cannot follow")
		}
		at, ok := r.offsetOf(regs[pr])
		if !ok {
			at = int64(addrOf(regs[pr]) - r.base)
		}
		v, t := r.pointerAt(at + off)
		m.reached(v, t)
		return v
	}})
}

// unsafeData compiles unsafe.SliceData(x) and unsafe.StringData(x). The
// region of the data is recorded (see toUnsafe).
func (c *compiler) unsafeData(name string, dst int32, x ssa.Value) {
	xr, m := c.reg(x), c.m
	if name == "StringData" {
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
			s := valStr(regs[xr])
			if s == "" {
				return Value{}
			}
			r := strRegion(s)
			p := bytePtr(unsafe.StringData(s), r.ro) // read-only, as in gc
			m.remember(r, p)
			if !r.ro {
				m.extents.addBytes(unsafe.StringData(s), len(s))
			}
			return p
		}})
		return
	}
	et := sliceElem(x.Type())
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		var p Value
		switch s := regs[xr].R.(type) {
		case []byte:
			if cap(s) == 0 {
				return Value{}
			}
			p = Value{R: &s[:1][0]}
			m.extents.addBytes(&s[:1][0], cap(s))
		case []Value:
			if cap(s) == 0 {
				return Value{}
			}
			p = Value{R: &s[:1][0]}
			m.extents.addCells(&s[:1][0], cap(s), et)
		case reflect.Value:
			if s.Cap() == 0 {
				return Value{}
			}
			return Value{R: s.Slice(0, 1).Index(0).Addr()}
		case *zslice:
			return Value{R: &Value{}} // zero-size elements have no memory
		default:
			return Value{}
		}
		if r := backing(regs[xr], et); r != nil {
			m.remember(r, p)
		}
		return p
	}})
}

// unsafeSlice compiles unsafe.Slice(p, n) and unsafe.String(p, n). Go
// checks nothing here; the VM checks that the n elements are inside memory
// whose extent it knows (the SSA origin of p, a recent region, or the
// single element p points to). So the result can never reach memory
// outside a real Go array.
//
// When the program never uses the result (_ = unsafe.Slice(p, n)), only
// the checks of gc run: nothing reads the memory, so the VM does not need
// to know it, and a pointer that it does not know (badAddr) is not refused
// (GOROOT test unsafebuiltins.go).
func (c *compiler) unsafeSlice(name string, dst int32, call *ssa.Call, p, n ssa.Value, et types.Type) {
	orig, pr, nr, m := c.uorigin(p), c.reg(p), c.reg(n), c.m
	used := isUsed(call)
	str := name == "String"
	sz := gcSizes.Sizeof(et)
	unsignedLen := false
	if b := basic(n.Type()); b != nil && b.Info()&types.IsUnsigned != 0 {
		unsignedLen = true
	}
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		ptr, cnt := regs[pr], regs[nr].N
		if cnt < 0 || (unsignedLen && uint64(cnt) > math.MaxInt64) || (sz > 0 && cnt > math.MaxInt64/sz) {
			panic(runtimeError("unsafe." + name + ": len out of range"))
		}
		if ptr.R == nil {
			if cnt != 0 {
				panic(runtimeError("unsafe." + name + ": ptr is nil and len is not zero"))
			}
			if str {
				return Value{R: ""}
			}
			return Value{}
		}
		if b, ok := ptr.R.(badAddr); ok {
			if uint64(cnt)*uint64(sz) > uint64(-uintptr(b)) {
				panic(runtimeError("unsafe." + name + ": len out of range"))
			}
			if !used {
				return Value{}
			}
			b.refuse()
		}
		if !used {
			return Value{}
		}
		var r *uregion
		if orig != nil {
			r = orig(regs)
		}
		if r == nil || !r.holds(ptr) {
			r = m.findRegionPtr(ptr)
		}
		if r == nil {
			// a pointer given by SliceData or StringData that the ring forgot
			r = m.extents.region(ptr)
		}
		if r == nil {
			r = lone(ptr, nil)
		}
		if r == nil {
			unsupported("unsafe.%s on a pointer the VM cannot follow (%T)", name, ptr.R)
		}
		return r.slice(ptr, int(cnt), et, str, name)
	}})
}

// isUsed reports whether an instruction of the function reads the value
// v (a DebugRef does not count).
func isUsed(v ssa.Value) bool {
	refs := v.Referrers()
	if refs == nil {
		return true // unknown: as if used
	}
	for _, r := range *refs {
		if _, ok := r.(*ssa.DebugRef); !ok {
			return true
		}
	}
	return false
}

func (r *uregion) holds(p Value) bool { _, ok := r.index(p); return ok }

// slice returns the n elements of type et from p (which r holds), as a
// slice or, when str is set, as a string.
func (r *uregion) slice(p Value, n int, et types.Type, str bool, name string) Value {
	i, _ := r.index(p)
	if r.bytes != nil && !isByteType(et) {
		return r.wordSlice(i, n, et, str, name)
	}
	if r.bytes != nil {
		b := r.bytes[i:]
		if n > len(b) {
			unsupported("unsafe.%s: %d elements reach outside the memory known to the VM (%d bytes)", name, n, len(b))
		}
		if str {
			if n == 0 {
				return Value{R: ""}
			}
			return Value{R: unsafe.String(&b[0], n)}
		}
		if r.ro {
			// writable bytes of read-only memory: a copy
			return Value{R: append([]byte(nil), b[:n]...)}
		}
		return Value{R: b[:n:n]}
	}
	es := r.elems[i:]
	t := r.t
	if t != nil && len(es) == 1 && !sameLayout(t, et) {
		if a, ok := t.Underlying().(*types.Array); ok && sameLayout(a.Elem(), et) {
			// a pointer to an array used as a pointer to its first element
			if arr, ok := es[0].R.([]Value); ok {
				es, t = arr, a.Elem()
			}
		}
	}
	if t != nil && !sameLayout(t, et) {
		unsupported("unsafe: viewing a %s as a %s needs byte-level memory", t, et)
	}
	if n > len(es) {
		unsupported("unsafe.%s: %d elements reach outside the memory known to the VM (%d elements)", name, n, len(es))
	}
	if str {
		if !isByteType(et) {
			unsupported("unsafe.String of a %s", et)
		}
		return Value{R: string(valBytes(Value{R: es[:n]}))}
	}
	return Value{R: es[:n:n]}
}

// wordSlice returns unsafe.Slice(p, n) of a basic type et wider than a byte
// (integers and floats) when p points to byte i of the real bytes of
// r: a Go slice of et over the same bytes, as in gc
// (unsafe.Slice((*uint32)(unsafe.Pointer(&b[0])), len(b)/4)). The VM holds
// it as a native slice. A named type or a struct stays refused (its values
// need the VM layout), and so does a string.
func (r *uregion) wordSlice(i, n int, et types.Type, str bool, name string) Value {
	bt, ok := types.Unalias(et).(*types.Basic)
	if str || !ok || bt.Info()&(types.IsInteger|types.IsFloat) == 0 {
		unsupported("unsafe: viewing bytes as a %s needs byte-level memory", et)
	}
	rt, ok := basicTypes[bt.Kind()]
	if !ok {
		unsupported("unsafe: viewing bytes as a %s needs byte-level memory", et)
	}
	b := r.bytes[i:]
	if int64(n)*int64(rt.Size()) > int64(len(b)) {
		unsupported("unsafe.%s: %d elements reach outside the memory known to the VM (%d bytes)", name, n, len(b))
	}
	if n == 0 {
		return Value{R: reflect.MakeSlice(reflect.SliceOf(rt), 0, 0)}
	}
	if r.ro {
		// writable elements over read-only memory: a copy
		b = append([]byte(nil), b[:n*int(rt.Size())]...)
	}
	return Value{R: reflect.SliceAt(rt, unsafe.Pointer(&b[0]), n)}
}

// basicTypes are the native types of the basic kinds that wordSlice makes
// slices of.
var basicTypes = map[types.BasicKind]reflect.Type{
	types.Int:     reflect.TypeFor[int](),
	types.Int8:    reflect.TypeFor[int8](),
	types.Int16:   reflect.TypeFor[int16](),
	types.Int32:   reflect.TypeFor[int32](),
	types.Int64:   reflect.TypeFor[int64](),
	types.Uint:    reflect.TypeFor[uint](),
	types.Uint16:  reflect.TypeFor[uint16](),
	types.Uint32:  reflect.TypeFor[uint32](),
	types.Uint64:  reflect.TypeFor[uint64](),
	types.Uintptr: reflect.TypeFor[uintptr](),
	types.Float32: reflect.TypeFor[float32](),
	types.Float64: reflect.TypeFor[float64](),
}

// unsafeView compiles (*T)(p) for an unsafe.Pointer p. When the SSA shows
// p = unsafe.Pointer(q) with q of type *F, F and T must have the same VM
// layout, or F must be a string or a slice and T a view of its header
// (unsafehdr.go).
func (c *compiler) unsafeView(v *ssa.Convert) {
	dst, x := c.reg(v), c.reg(v.X)
	to := v.Type().Underlying().(*types.Pointer).Elem()
	src, ok := v.X.(*ssa.Convert)
	var from types.Type
	if ok {
		if pt, ok := src.X.Type().Underlying().(*types.Pointer); ok {
			from = pt.Elem()
		}
	}
	m := c.m
	switch {
	case from != nil && isIface(from) && !sameLayout(from, to) && isIfaceView(from, to):
		// the type and data words of an interface (ifaceview.go)
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return m.newView(regs[x], from, to) }})
	case from != nil && isHdrSrc(from) && !sameLayout(from, to) && isHdrView(from, to):
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return m.newView(regs[x], from, to) }})
	case from == nil:
		// The origin is unknown: a cell that holds a string or a []byte
		// is viewed as its header, a view of bytes or a pointer to a byte
		// as plain data over the bytes (viewOf, byteview.go).
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return m.viewOf(regs[x], to, true) }})
	case isByteType(from) && (m.isPOD(to) || m.nativePlainType(to) != nil) && !isByteType(to):
		if arrayOf(to, from) > 0 {
			// (*[N]byte)(p): over real bytes, or over cells
			c.elemsView(dst, v.X, from, to)
			break
		}
		// Plain data over the bytes of a Go []byte or of a string
		// (byteview.go), in the region the SSA shows. A byte held in a
		// cell keeps its cell when the layouts agree (an int8 over a byte
		// variable); other bytes held as cells are refused.
		orig, same := c.uorigin(v.X), sameLayout(from, to)
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
			var r *uregion
			if orig != nil {
				r = orig(regs)
			}
			if bv, ok := m.byteViewOf(regs[x], r, to); ok {
				return bv
			}
			switch regs[x].R.(type) {
			case nil, badAddr:
				// nil stays nil; only a dereference of a bad address is refused
				return regs[x]
			}
			if same {
				return m.viewOf(regs[x], to, false)
			}
			unsupported("unsafe: viewing a %s as a %s needs byte-level memory", from, to)
			return Value{}
		}})
	case sameLayout(from, to):
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return m.viewOf(regs[x], to, false) }})
	case arrayOf(to, from) > 0:
		c.elemsView(dst, v.X, from, to)
	case arrayOf(from, to) > 0:
		// a pointer to an array as a pointer to its first element
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
			if p, ok := regs[x].R.(*Value); ok {
				if es, ok := p.R.([]Value); ok {
					return Value{R: &es[0]}
				}
			}
			return m.viewOf(regs[x], to, false)
		}})
	case (hdrWords(from) != nil || isStrHdr(from)) && isHdrSrc(to):
		// a header built by the program, read as a string or a slice
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value { return m.fromHeader(regs[x], from, to) }})
	case firstPath(from, to) > 0:
		// a pointer to a struct as a pointer to its first field
		n := firstPath(from, to)
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
			if _, ok := regs[x].R.(*Value); ok {
				return m.firstOf(regs[x], from, n)
			}
			return m.viewOf(regs[x], to, false)
		}})
	default:
		c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
			switch p := regs[x].R.(type) {
			case *hdrView:
				return p.as(to)
			case *byteView:
				return p.as(m, to)
			}
			if v, ok := m.nativeView(regs[x], from, to); ok {
				return v // a native value of plain data (nativeview.go)
			}
			unsupported("unsafe: viewing a %s as a %s needs byte-level memory", from, to)
			return Value{}
		}})
	}
}

// arrayOf returns the length of a when a is an array type whose elements
// have the layout of e (0 otherwise).
func arrayOf(a, e types.Type) int64 {
	if u, ok := a.Underlying().(*types.Array); ok && sameLayout(u.Elem(), e) && !sameLayout(a, e) {
		return u.Len()
	}
	return 0
}

// elemsView compiles (*[n]E)(p) for an unsafe.Pointer p that comes from a
// *E: the array is made of the n cells from p (they are shared, as the
// memory in gc), which must be in memory whose extent the VM knows.
func (c *compiler) elemsView(dst int32, p ssa.Value, from, to types.Type) {
	orig, pr, m := c.uorigin(p), c.reg(p), c.m
	n := int(to.Underlying().(*types.Array).Len())
	c.emit(Instr{Op: OpFunc, A: dst, X: func(regs []Value) Value {
		ptr := regs[pr]
		if ptr.R == nil {
			return Value{}
		}
		var r *uregion
		if orig != nil {
			r = orig(regs)
		}
		if r == nil || !r.holds(ptr) {
			r = m.findRegionPtr(ptr)
		}
		if r == nil {
			r = lone(ptr, from)
		}
		if r != nil && r.bytes != nil && m.isPOD(to) {
			// (*[N]byte)(&b[i]) over a Go []byte: a view of the bytes.
			i, _ := r.index(ptr)
			v, why := m.byteViewAt(r, i, to)
			if why != "" {
				unsupported("unsafe: viewing a %s as a %s %s", from, to, why)
			}
			return v
		}
		if r == nil || r.bytes != nil || (r.t != nil && !sameLayout(r.t, from)) {
			unsupported("unsafe: viewing a %s as a %s needs byte-level memory", from, to)
		}
		i, _ := r.index(ptr)
		es := r.elems[i:]
		if len(es) < n {
			unsupported("unsafe: viewing a %s as a %s reaches outside the memory known to the VM (%d elements)", from, to, len(es))
		}
		return Value{R: &Value{R: es[:n:n]}}
	}})
}

func isPointer(t types.Type) bool { _, ok := t.(*types.Pointer); return ok }

// ptrOf is an interpreted pointer as an unsafe.Pointer for native code.
func ptrOf(v Value) unsafe.Pointer {
	switch p := v.R.(type) {
	case *Value:
		return unsafe.Pointer(p)
	case *byte:
		return unsafe.Pointer(p)
	case roByte:
		return unsafe.Pointer(p)
	case *byteView:
		return unsafe.Pointer(p.ptr())
	case reflect.Value:
		return p.UnsafePointer()
	case badAddr:
		p.refuse()
	}
	return nil
}

func isString(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// sameLayout reports whether values of types a and b have the same
// representation in the VM and in gc, so that a pointer to one can be used
// as a pointer to the other.
func sameLayout(a, b types.Type) bool {
	if types.Identical(a.Underlying(), b.Underlying()) {
		return true
	}
	switch ua := a.Underlying().(type) {
	case *types.Basic:
		ub, ok := b.Underlying().(*types.Basic)
		return ok && ua.Info()&(types.IsString|types.IsComplex) == 0 && ub.Info()&(types.IsString|types.IsComplex) == 0 &&
			gcSizes.Sizeof(ua) == gcSizes.Sizeof(ub) && ua.Kind() != types.Float32 && ub.Kind() != types.Float32
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature:
		switch b.Underlying().(type) {
		case *types.Pointer, *types.Map, *types.Chan, *types.Signature:
			return true
		}
		if ub, ok := b.Underlying().(*types.Basic); ok {
			return ub.Kind() == types.UnsafePointer
		}
	case *types.Slice:
		_, ok := b.Underlying().(*types.Slice)
		return ok && sameLayout(ua.Elem(), b.Underlying().(*types.Slice).Elem())
	case *types.Array:
		ub, ok := b.Underlying().(*types.Array)
		return ok && ua.Len() == ub.Len() && sameLayout(ua.Elem(), ub.Elem())
	case *types.Struct:
		ub, ok := b.Underlying().(*types.Struct)
		if !ok || ua.NumFields() != ub.NumFields() {
			return false
		}
		for i := 0; i < ua.NumFields(); i++ {
			if !sameLayout(ua.Field(i).Type(), ub.Field(i).Type()) {
				return false
			}
		}
		return true
	case *types.Interface:
		_, ok := b.Underlying().(*types.Interface)
		return ok
	}
	return false
}
