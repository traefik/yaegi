package vm

// Run-time check of the writes of a restricted plugin to the memory of the
// host that the standard library variables reach.
//
// Native packages run in the host process: their variables, and the
// objects these variables point to, are the host's. The load-time check
// (checkWrites in restrict.go) refuses the writes that name such a variable
// (time.Local = x, *time.UTC = x, delete(unicode.Scripts, k)). It cannot
// see a write through a copy of the pointer (l := time.Local; *l = x),
// through a pointer that a native function returns
// (*time.Now().Location() = x, the same object as *time.Local), or through
// the reflect of the plugin (reflect.ValueOf(io.EOF).Elem().Set(...)).
//
// hostMem is the set of these objects: at its first use, it walks every
// variable of the native registry (stdsyms.Symbols) and of the registered
// tables of the AOT copies and native packages (aotPkgs, natPkgs: their
// globals are shared by all plugins), except the ones a restricted machine
// has its own copy of (ownVars; but the host's *os.File of os.Stdout,
// Stderr and Stdin are walked, see sharedObjVars), and records the memory
// of the variable, of each object that a pointer reaches, of the arrays of
// the slices, and the maps. A restricted machine (m.restricted) checks the
// address before it writes to native memory, and refuses the write with a
// *RefusedError panic, which the plugin can recover:
//
//   - stores through a pointer, a field or an element (OpStore and the
//     fused stores, the elements of native slice views), atomic stores;
//   - map updates, delete and clear on a native map;
//   - append into the free capacity of a native slice, copy into it;
//   - the elements of a host []byte, which the VM keeps as a Go []byte
//     (hostBytes): stores, append, copy and clear;
//   - the reflect of the plugin (the twin): Set and the other Set methods,
//     SetMapIndex, Clear, Copy, on a value in host memory.
//
// The identity of the objects does not change: io.EOF stays the host's
// value, and err == io.EOF works with the errors of native code.
//
// Limits (docs/SANDBOX.md): only the writes of the VM itself are checked,
// not the writes of native code that the plugin calls. The set follows the
// exported fields and is made once, at the first check. With
// Options.UseUnsafe, the unsafe views (unsafehdr.go) are not checked.

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/traefik/yaegi/v2/stdsyms"
)

// hostSpan is the memory [lo, hi) of a host object.
type hostSpan struct{ lo, hi uintptr }

// hostMemSet is the memory of the host that the standard library variables
// reach (see the top of the file).
type hostMemSet struct {
	spans []hostSpan // sorted, merged
	// pages is an open addressing table of the pages (address >>
	// hostPageShift) that spans touch, stored as page+1 (0: an empty
	// slot); it has 1<<pageBits slots.
	pages    []uintptr
	pageBits uint
	maps     map[unsafe.Pointer]bool
	// keep holds the objects, so that their memory is never given to
	// another object (the plugin's own) while the set says it is the host's.
	keep []unsafe.Pointer
}

// hostMem returns the set, made at the first use.
var hostMem = sync.OnceValue(func() *hostMemSet {
	w := &hostWalk{
		set:  &hostMemSet{maps: map[unsafe.Pointer]bool{}},
		seen: map[hostSeen]bool{},
		ptrs: map[reflect.Type]bool{},
	}
	// own reports the variables that a restricted machine has its own copy
	// of, and whose object is its own too. The copies of os.Stdin, Stdout
	// and Stderr are duplicates of the host files (stdio.go); the host's
	// *os.File objects are walked all the same (sharedObjVars).
	own := func(name string) bool { return ownVars[name] && !sharedObjVars[name] }
	keys := make([]string, 0, len(stdsyms.Symbols))
	for k := range stdsyms.Symbols {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		pkg := k[:strings.LastIndexByte(k, '/')]
		for name, v := range stdsyms.Symbols[k] {
			if !v.CanAddr() || own(pkg+"."+name) {
				continue // a function, a type, a constant, or the plugin's own
			}
			w.object(v.Addr().UnsafePointer(), v)
		}
	}
	// The registered tables of the AOT copies (aot/, Options.AOT) and of
	// the native non-standard packages (cmd/aotgen -native): their
	// variables are host variables too, shared by every plugin of the
	// process (xml.HTMLEntity, sql.ErrNoRows of the AOT copies).
	regMu.RLock()
	var tables []*Package
	for _, p := range aotPkgs {
		tables = append(tables, p)
	}
	for _, p := range natPkgs {
		tables = append(tables, p)
	}
	regMu.RUnlock()
	slices.SortFunc(tables, func(a, b *Package) int { return strings.Compare(a.Path+a.AOTPath, b.Path+b.AOTPath) })
	for _, p := range tables {
		for name, v := range p.Vars {
			if v.Kind() != reflect.Pointer || v.IsNil() || own(p.Path+"."+name) {
				continue
			}
			w.object(v.UnsafePointer(), v.Elem())
		}
	}
	s := w.set
	slices.SortFunc(s.spans, func(a, b hostSpan) int {
		switch {
		case a.lo < b.lo:
			return -1
		case a.lo > b.lo:
			return 1
		}
		return 0
	})
	merged := s.spans[:0]
	for _, sp := range s.spans {
		if n := len(merged); n > 0 && sp.lo <= merged[n-1].hi {
			merged[n-1].hi = max(merged[n-1].hi, sp.hi)
			continue
		}
		merged = append(merged, sp)
	}
	s.spans = slices.Clip(merged)
	s.index()
	return s
})

// hostPageShift is the size of the pages of hostMemSet.pages: 8 KiB, the
// size of the small spans of the Go allocator.
const hostPageShift = 13

// index fills the page table of s from its spans.
func (s *hostMemSet) index() {
	var pages []uintptr
	for _, sp := range s.spans {
		for pg := sp.lo >> hostPageShift; pg <= (sp.hi-1)>>hostPageShift; pg++ {
			pages = append(pages, pg)
		}
	}
	slices.Sort(pages)
	pages = slices.Compact(pages)
	bits := uint(4)
	for 1<<bits < 2*len(pages) {
		bits++ // at most half full
	}
	s.pageBits = bits
	s.pages = make([]uintptr, 1<<bits)
	mask := uintptr(1)<<bits - 1
	for _, pg := range pages {
		h := hostPageHash(pg, bits)
		for s.pages[h] != 0 {
			h = (h + 1) & mask
		}
		s.pages[h] = pg + 1
	}
}

// hostPageHash is the slot of the page pg in a table of 1<<bits slots.
func hostPageHash(pg uintptr, bits uint) uintptr {
	return uintptr(uint64(pg) * 0x9E3779B97F4A7C15 >> (64 - bits))
}

// has reports whether the address p is in host memory. It is on the path
// of every store of a restricted machine to native memory: most addresses
// of the plugin are on a page without host objects, which the page table
// tells with one or two loads; the others take a binary search.
func (s *hostMemSet) has(p uintptr) bool {
	spans := s.spans
	if len(spans) == 0 || p < spans[0].lo || p >= spans[len(spans)-1].hi {
		return false
	}
	if !s.hasPage(p >> hostPageShift) {
		return false
	}
	// The first span whose hi is above p.
	lo, hi := 0, len(spans)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if spans[mid].hi <= p {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(spans) && spans[lo].lo <= p
}

// hasPage reports whether a span of s touches the page pg.
func (s *hostMemSet) hasPage(pg uintptr) bool {
	mask := uintptr(len(s.pages) - 1)
	for h := hostPageHash(pg, s.pageBits); ; h = (h + 1) & mask {
		e := s.pages[h]
		if e == 0 {
			return false
		}
		if e == pg+1 {
			return true
		}
	}
}

// hostByte is hostWriteAt for a byte store of the restricted machine of
// cx (an element of a Go []byte, bytes.go). It remembers the last page
// that has no host memory: the set does not change once made, so the
// stores of a loop into a buffer of the plugin cost a shift and a compare,
// which the callers do before they call it.
func (cx *ctx) hostByte(p unsafe.Pointer) {
	pg := uintptr(p) >> hostPageShift
	s := hostMem()
	if !s.hasPage(pg) {
		cx.freePage = pg
		return
	}
	if s.has(uintptr(p)) {
		panic(errHostWrite)
	}
}

// hostSeen is an object already walked, with the type it was walked as.
type hostSeen struct {
	p unsafe.Pointer
	t reflect.Type
}

// hostWalk records the objects that the variables reach.
type hostWalk struct {
	set  *hostMemSet
	seen map[hostSeen]bool
	ptrs map[reflect.Type]bool // hasPointers
}

// object records the object at p, whose value is v, and walks it.
func (w *hostWalk) object(p unsafe.Pointer, v reflect.Value) {
	k := hostSeen{p, v.Type()}
	if p == nil || w.seen[k] {
		return
	}
	w.seen[k] = true
	if size := v.Type().Size(); size > 0 {
		w.set.spans = append(w.set.spans, hostSpan{uintptr(p), uintptr(p) + size})
		w.set.keep = append(w.set.keep, p)
	}
	w.value(v)
}

// value walks the objects that v, a value in host memory, points to.
func (w *hostWalk) value(v reflect.Value) {
	if !w.hasPointers(v.Type()) {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			w.object(v.UnsafePointer(), v.Elem())
		}
	case reflect.Interface:
		if !v.IsNil() {
			w.value(v.Elem())
		}
	case reflect.Slice:
		if v.IsNil() || v.Cap() == 0 {
			return
		}
		p := v.UnsafePointer()
		k := hostSeen{p, v.Type()}
		if w.seen[k] {
			return
		}
		w.seen[k] = true
		if size := uintptr(v.Cap()) * v.Type().Elem().Size(); size > 0 {
			w.set.spans = append(w.set.spans, hostSpan{uintptr(p), uintptr(p) + size})
			w.set.keep = append(w.set.keep, p)
		}
		if w.hasPointers(v.Type().Elem()) {
			for i := range v.Len() {
				w.value(v.Index(i))
			}
		}
	case reflect.Map:
		if v.IsNil() || w.set.maps[v.UnsafePointer()] {
			return
		}
		w.set.maps[v.UnsafePointer()] = true
		w.set.keep = append(w.set.keep, v.UnsafePointer())
		it := v.MapRange()
		for it.Next() {
			w.value(it.Key())
			w.value(it.Value())
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			if t.Field(i).IsExported() {
				w.value(v.Field(i))
			}
		}
	case reflect.Array:
		if w.hasPointers(v.Type().Elem()) {
			for i := range v.Len() {
				w.value(v.Index(i))
			}
		}
	}
}

// hasPointers reports whether a value of type t can reach other memory
// that the walk records: a pointer, a slice, a map or an interface. A
// function, a channel and an unsafe.Pointer are not followed.
func (w *hostWalk) hasPointers(t reflect.Type) bool {
	if r, ok := w.ptrs[t]; ok {
		return r
	}
	w.ptrs[t] = false // a recursive type: the cycle goes through a pointer
	r := false
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map:
		r = true
	case reflect.Array:
		r = t.Len() > 0 && w.hasPointers(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() && w.hasPointers(f.Type) {
				r = true
				break
			}
		}
	}
	w.ptrs[t] = r
	return r
}

// errHostWrite is the panic of a refused write to host memory.
var errHostWrite = &RefusedError{What: "writing to memory of the host (an object of a standard library variable)"}

// hostWriteAt panics when the restricted machine m writes at the address p.
func (m *Machine) hostWriteAt(p unsafe.Pointer) {
	if m != nil && m.restricted && p != nil && hostMem().has(uintptr(p)) {
		panic(errHostWrite)
	}
}

// hostWrite panics when the restricted machine m sets the native value n
// (reflect Set and the other Set methods): its own memory, when n is
// addressable.
func (m *Machine) hostWrite(n reflect.Value) {
	if m != nil && m.restricted && n.IsValid() && n.CanAddr() {
		m.hostWriteAt(unsafe.Pointer(n.UnsafeAddr()))
	}
}

// hostWriteIn panics when the restricted machine m changes the content of
// the native map or slice n: the map itself, or the array of the slice.
func (m *Machine) hostWriteIn(n reflect.Value) {
	if m == nil || !m.restricted || !n.IsValid() {
		return
	}
	switch n.Kind() {
	case reflect.Map:
		if !n.IsNil() && hostMem().maps[n.UnsafePointer()] {
			panic(errHostWrite)
		}
	case reflect.Slice:
		if n.Cap() > 0 {
			m.hostWriteAt(n.UnsafePointer())
		}
	case reflect.Array:
		m.hostWrite(n)
	}
}

// hostBytes panics when the restricted machine m writes n bytes into b,
// a Go []byte of the VM (bytes.go). A []byte that the plugin reads from
// host memory (net.IPv4bcast, asn1.NullBytes, a []byte field) is a Go
// []byte too, with the array of the host: its elements are checked here,
// at each path that writes into a Go []byte (stores, append into the free
// capacity, copy, clear, the reflect of the plugin). n is the number of
// bytes written: 0 writes nothing (copy of an empty slice). execCopy gives
// n = 0 for a source that seqLen does not know (a native slice view, a
// reflect.Value): this is not a forgotten kind, copyBytes copies nothing
// from such a source either, so nothing is written.
func (m *Machine) hostBytes(b []byte, n int) {
	if m != nil && m.restricted && n > 0 && cap(b) > 0 {
		m.hostWriteAt(unsafe.Pointer(unsafe.SliceData(b)))
	}
}

// hostBytesAppend is hostBytes for append(b, n bytes): the append writes
// into the array of b only when the n bytes fit in its free capacity.
func (m *Machine) hostBytesAppend(b []byte, n int) {
	if m != nil && m.restricted && n > 0 && n <= cap(b)-len(b) {
		m.hostWriteAt(unsafe.Pointer(unsafe.SliceData(b)))
	}
}

// hostWritePtr is hostWrite for the value that the native pointer p points
// to.
func (m *Machine) hostWritePtr(p reflect.Value) {
	if m != nil && m.restricted && p.Kind() == reflect.Pointer && !p.IsNil() {
		m.hostWriteAt(p.UnsafePointer())
	}
}

// hostAppend panics when an append to the native slice s by the restricted
// machine m would write into the free capacity of a host array.
func (m *Machine) hostAppend(s any) {
	if m == nil || !m.restricted {
		return
	}
	if rv, ok := natViewOf(s); ok && rv.Cap() > rv.Len() {
		m.hostWriteAt(rv.UnsafePointer())
	}
}

// writer returns the machine that writes through v: the machine of its
// reflect, or the machine of the goroutine for a native value. nil (no
// check) for a goroutine that runs no interpreted code.
func (v RV) writer() *Machine {
	if v.t != nil && v.t.vr != nil && v.t.vr.m != nil {
		return v.t.vr.m
	}
	return curMachine()
}
