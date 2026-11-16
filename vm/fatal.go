package vm

// Fatal errors of the Go runtime as panics of the plugin
// (Options.PanicOnFatal).
//
// Some errors of a Go program are not panics but fatal errors: the runtime
// prints them and ends the process, and recover cannot stop them
// (runtime.fatal, "used when user code is expected to be at fault for the
// failure, such as racing map writes"). In a plugin, they end the host. The
// VM can see some of them before they happen, on memory of the plugin, and
// raise a *FatalError panic instead, which the plugin can recover and which
// goes to GoPanic like any other panic of a plugin goroutine:
//
//   - Unlock of an unlocked sync.Mutex, Unlock of an RWMutex that is not
//     locked for writing, RUnlock of an RWMutex that is not locked for
//     reading. The VM replaces these three methods for the calls of
//     interpreted code (lookupMethod): the replacement reads the state of
//     the lock first. A lock of the host (an object of a standard library
//     variable, hostmem.go) keeps the Go behaviour.
//   - Concurrent writes to a map of the plugin, and a read or a range of
//     such a map during a write. Each write of the VM to an interpreted map
//     (MapV: an index assignment, delete, clear) marks the map for its
//     duration, and each read (an index expression, the start and each step
//     of a range) counts itself in the map for its duration. A write that
//     finds another write or a read panics instead of writing, and a read
//     that finds a write panics instead of reading. The atomics are
//     sequentially consistent, so of a write and a read that overlap, at
//     least one panics: the runtime never sees them overlap, and never ends
//     the process. Only these instructions are checked: the reads and writes
//     of native code and of the reflect of the plugin (json.Marshal of the
//     map, reflect.Value.SetMapIndex) are not, and can still reach the check
//     of the runtime.
//   - (*sync.Cond).Wait when the lock of the Cond (a sync.Mutex, a
//     sync.RWMutex or its RLocker) is not held: Wait unlocks it first. Unlock
//     of the RLocker of an RWMutex that is not locked for reading.
//   - A misuse of runtime.SetFinalizer (storage.go): a first argument that
//     is nil, not a pointer, a nil pointer or a pointer inside an object, a
//     second argument that is not a func or does not take the first one, a
//     second finalizer on the same object.
//   - A nil func given to native code that starts it later on a goroutine
//     of its own: time.AfterFunc, context.AfterFunc,
//     (*http.Server).RegisterOnShutdown ("go of nil func value" when it
//     runs) and (*sync.WaitGroup).Go (a nil dereference on a new goroutine,
//     which ends the process too). The call panics at once, on the
//     goroutine of the plugin. A func that is not nil and panics later is a
//     panic of a plugin goroutine (GoPanic), as before.
//
// What stays fatal: the same errors in native code (an AOT copy, the
// standard library, a map or a lock of the host), out of memory, a stack
// overflow in native code, a deadlock of all goroutines. In particular, two
// goroutines of a plugin that write the same native map (the http.Header of
// a request: h.Set, h.Add, h.Del run in native code) end the host with
// "concurrent map writes", as they end a compiled Go program. This is a
// known host risk, kept on purpose: the behaviour is the one of Go and of
// yaegi, the writes happen in native code that the VM does not see, and a
// check would only catch two plugin goroutines (not a plugin goroutine
// against the host) for a cost on every write.
//
// This changes the behaviour of the program (Go ends the process), so it is
// off by default. A host that runs plugins it does not trust turns it on
// (internal/pluginhost does).

import (
	"context"
	"go/types"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unsafe" // go:linkname
)

// FatalError is the panic that replaces a fatal error of the Go runtime in
// a plugin with Options.PanicOnFatal. Msg is the message of the runtime
// ("concurrent map writes", "sync: unlock of unlocked mutex"), or, for an
// error that Go reports as a panic on a goroutine that nothing recovers, a
// short description of it.
type FatalError struct{ Msg string }

func (e *FatalError) Error() string {
	return "fatal error: " + e.Msg + " (a panic of the plugin with Options.PanicOnFatal)"
}

var (
	errMapWrites     = &FatalError{"concurrent map writes"}
	errMapReadWrite  = &FatalError{"concurrent map read and map write"}
	errMapIterWrite  = &FatalError{"concurrent map iteration and map write"}
	errUnlockMutex   = &FatalError{"sync: unlock of unlocked mutex"}
	errUnlockRWMutex = &FatalError{"sync: Unlock of unlocked RWMutex"}
	errRUnlockRW     = &FatalError{"sync: RUnlock of unlocked RWMutex"}
	errGoNilFunc     = &FatalError{"go of nil func value"}
	errWaitGroupGo   = &FatalError{"sync: WaitGroup.Go of a nil func (a nil dereference on a new goroutine)"}
)

// beginWrite marks mp for a write of the VM, or panics when another write
// or a read runs.
func (mp *MapV) beginWrite() {
	if !mp.writing.CompareAndSwap(0, 1) {
		panic(errMapWrites)
	}
	if mp.readers.Load() != 0 {
		mp.writing.Store(0)
		panic(errMapReadWrite)
	}
}

// beginRead counts a read of the VM in mp, or panics with err (a read or a
// range) when a write runs. endRead ends it.
func (mp *MapV) beginRead(err *FatalError) {
	mp.readers.Add(1)
	if mp.writing.Load() != 0 {
		mp.readers.Add(-1)
		panic(err)
	}
}

func (mp *MapV) endRead() { mp.readers.Add(-1) }

// guardedGet is mp[k] with the read count.
func (mp *MapV) guardedGet(k any) (Value, bool) {
	mp.beginRead(errMapReadWrite)
	defer mp.endRead() // a key of an unhashable dynamic type panics
	v, ok := mp.M[k]
	return v, ok
}

// guardedMapNext is mapNext with the read count of the map of it.
func (m *Machine) guardedMapNext(it *mapIter) Value {
	mp := it.m
	mp.beginRead(errMapIterWrite)
	defer mp.endRead()
	return m.mapNext(it)
}

// guardedSet is mp[k] = v (kv: the original key) with the write mark.
func (mp *MapV) guardedSet(k any, v, kv Value) {
	mp.beginWrite()
	defer mp.writing.Store(0)
	mp.M[k] = v
	mp.setKey(k, kv)
}

// guardedDelete is delete(mp, k) with the write mark.
func (mp *MapV) guardedDelete(k any) {
	mp.beginWrite()
	defer mp.writing.Store(0)
	delete(mp.M, k)
	mp.deleteKey(k)
}

// guardedClear is clear(mp) with the write mark.
func (mp *MapV) guardedClear() {
	mp.beginWrite()
	defer mp.writing.Store(0)
	mp.clear()
}

// lockLayout holds the offsets of the state words of sync.Mutex and
// sync.RWMutex, read with reflect from the types of the host's Go release.
// ok is false when the layout is not the expected one (a new Go release):
// the replacements then keep the Go behaviour, and TestFatalLockLayout
// fails.
type lockLayout struct {
	mutexState  uintptr // int32: bit 0 is mutexLocked
	readerCount uintptr // int32 of RWMutex: < 0 while locked for writing
	ok          bool
}

const rwmutexMaxReaders = 1 << 30 // sync/rwmutex.go

var locks = sync.OnceValue(func() lockLayout {
	var l lockLayout
	mt := reflect.TypeFor[sync.Mutex]()
	off, ok := int32Field(mt, "state")
	if !ok {
		// Go 1.24 and later: sync.Mutex{_ noCopy; mu isync.Mutex}.
		if f, found := mt.FieldByName("mu"); found && f.Type.Kind() == reflect.Struct {
			if o, found := int32Field(f.Type, "state"); found {
				off, ok = f.Offset+o, true
			}
		}
	}
	rt := reflect.TypeFor[sync.RWMutex]()
	f, found := rt.FieldByName("readerCount")
	if !ok || !found {
		return l
	}
	rc, rcOK := f.Offset, f.Type.Kind() == reflect.Int32
	if f.Type.Kind() == reflect.Struct { // atomic.Int32{_ noCopy; v int32}
		var o uintptr
		o, rcOK = int32Field(f.Type, "v")
		rc += o
	}
	if !rcOK {
		return l
	}
	return lockLayout{mutexState: off, readerCount: rc, ok: true}
})

// int32Field returns the offset of the int32 field name of the struct t.
func int32Field(t reflect.Type, name string) (uintptr, bool) {
	f, ok := t.FieldByName(name)
	if !ok || f.Type.Kind() != reflect.Int32 {
		return 0, false
	}
	return f.Offset, true
}

// lockWord returns the int32 at offset off of the lock p.
func lockWord(p unsafe.Pointer, off uintptr) int32 {
	return atomic.LoadInt32((*int32)(unsafe.Add(p, off)))
}

// pluginLock reports whether the lock at p belongs to the plugin, not to
// the host (an object of a standard library variable).
func pluginLock(p unsafe.Pointer) bool { return !hostMem().has(uintptr(p)) }

// fatalUnlock is (*sync.Mutex).Unlock with PanicOnFatal.
func fatalUnlock(mu *sync.Mutex) {
	l := locks()
	if p := unsafe.Pointer(mu); l.ok && lockWord(p, l.mutexState)&1 == 0 && pluginLock(p) {
		panic(errUnlockMutex)
	}
	mu.Unlock()
}

// fatalRWUnlock is (*sync.RWMutex).Unlock with PanicOnFatal: not locked for
// writing when the reader count is not negative.
func fatalRWUnlock(rw *sync.RWMutex) {
	l := locks()
	if p := unsafe.Pointer(rw); l.ok && lockWord(p, l.readerCount) >= 0 && pluginLock(p) {
		panic(errUnlockRWMutex)
	}
	rw.Unlock()
}

// fatalRUnlock is (*sync.RWMutex).RUnlock with PanicOnFatal: rUnlockSlow
// throws when no reader holds the lock (the count is 0, or -maxReaders
// while a writer waits for the readers).
func fatalRUnlock(rw *sync.RWMutex) {
	l := locks()
	if p := unsafe.Pointer(rw); l.ok {
		if r := lockWord(p, l.readerCount); (r == 0 || r == -rwmutexMaxReaders) && pluginLock(p) {
			panic(errRUnlockRW)
		}
	}
	rw.RUnlock()
}

// rlockerType is the type of RWMutex.RLocker: *sync.rlocker, a pointer to
// the RWMutex. Its Unlock is RUnlock.
var rlockerType = reflect.TypeOf(new(sync.RWMutex).RLocker())

// fatalUnlockOf panics like the Unlock of the lock l would throw: l is a
// *sync.Mutex, a *sync.RWMutex or an RLocker. Another Locker is not checked.
func fatalUnlockOf(l sync.Locker) {
	ll := locks()
	if !ll.ok || l == nil {
		return
	}
	switch l := l.(type) {
	case *sync.Mutex:
		if p := unsafe.Pointer(l); lockWord(p, ll.mutexState)&1 == 0 && pluginLock(p) {
			panic(errUnlockMutex)
		}
	case *sync.RWMutex:
		if p := unsafe.Pointer(l); lockWord(p, ll.readerCount) >= 0 && pluginLock(p) {
			panic(errUnlockRWMutex)
		}
	default:
		if reflect.TypeOf(l) != rlockerType {
			return
		}
		p := reflect.ValueOf(l).UnsafePointer()
		if r := lockWord(p, ll.readerCount); (r == 0 || r == -rwmutexMaxReaders) && pluginLock(p) {
			panic(errRUnlockRW)
		}
	}
}

// fatalCondWait is (*sync.Cond).Wait with PanicOnFatal: Wait unlocks c.L
// first, so a lock that is not held is a fatal error. The panic comes
// before Wait, so c stays as it was.
func fatalCondWait(c *sync.Cond) {
	fatalUnlockOf(c.L)
	c.Wait()
}

// fatalWaitGroupGo is (*sync.WaitGroup).Go with PanicOnFatal: the new
// goroutine calls f, and a nil f is a panic there that ends the process.
func fatalWaitGroupGo(wg *sync.WaitGroup, f func()) {
	if f == nil {
		panic(errWaitGroupGo)
	}
	wg.Go(f)
}

// fatalOnShutdown is (*http.Server).RegisterOnShutdown with PanicOnFatal:
// Shutdown starts f with a go statement, and a nil f is a fatal error then.
func fatalOnShutdown(s *http.Server, f func()) {
	if f == nil {
		panic(errGoNilFunc)
	}
	s.RegisterOnShutdown(f)
}

// fatalMethods are the methods that PanicOnFatal replaces (methodFunc: the
// receiver comes first).
var fatalMethods = sync.OnceValue(func() map[methodKey]reflect.Value {
	mu := reflect.TypeFor[*sync.Mutex]()
	rw := reflect.TypeFor[*sync.RWMutex]()
	rlUnlock := reflect.MakeFunc(reflect.FuncOf([]reflect.Type{rlockerType}, nil, false), func(a []reflect.Value) []reflect.Value {
		fatalRUnlock((*sync.RWMutex)(a[0].UnsafePointer()))
		return nil
	})
	// Short keys: gofmt 1.26 and 1.27 align long keys differently.
	cond := reflect.TypeFor[*sync.Cond]()
	wg := reflect.TypeFor[*sync.WaitGroup]()
	srv := reflect.TypeFor[*http.Server]()
	return map[methodKey]reflect.Value{
		{mu, "Unlock"}:              reflect.ValueOf(fatalUnlock),
		{rw, "Unlock"}:              reflect.ValueOf(fatalRWUnlock),
		{rw, "RUnlock"}:             reflect.ValueOf(fatalRUnlock),
		{rlockerType, "Unlock"}:     rlUnlock,
		{cond, "Wait"}:              reflect.ValueOf(fatalCondWait),
		{wg, "Go"}:                  reflect.ValueOf(fatalWaitGroupGo),
		{srv, "RegisterOnShutdown"}: reflect.ValueOf(fatalOnShutdown),
	}
})

// fatalFuncs are the functions that PanicOnFatal replaces: they start a
// func later on a goroutine of their own, and a nil func is a fatal error
// there ("go of nil func value").
var fatalFuncs = map[string]reflect.Value{
	"time.AfterFunc": reflect.ValueOf(func(d time.Duration, f func()) *time.Timer {
		if f == nil {
			panic(errGoNilFunc)
		}
		return time.AfterFunc(d, f)
	}),
	"context.AfterFunc": reflect.ValueOf(func(ctx context.Context, f func()) func() bool {
		if f == nil {
			panic(errGoNilFunc)
		}
		return context.AfterFunc(ctx, f)
	}),
}

// fatalFunc returns the replacement of the native function key
// ("time.AfterFunc") when m has PanicOnFatal.
func (m *Machine) fatalFunc(key string) (reflect.Value, bool) {
	if m == nil || !m.panicOnFatal {
		return reflect.Value{}, false
	}
	fn, ok := fatalFuncs[key]
	return fn, ok
}

// fatalMethod returns the replacement of the method name of the native type
// t when m has PanicOnFatal.
func (m *Machine) fatalMethod(t reflect.Type, name string) (reflect.Value, bool) {
	if m == nil || !m.panicOnFatal {
		return reflect.Value{}, false
	}
	fn, ok := fatalMethods()[methodKey{t, name}]
	return fn, ok
}

// runtime.SetFinalizer with PanicOnFatal (storage.go). The runtime throws
// on each misuse; checkFinalizer panics with the same message before the
// native call. The VM sets the finalizer of an interpreted pointer on its
// cell (a *Value): a pointer to a field or an element is a cell inside an
// object ([]Value), which the runtime refuses ("pointer not at beginning of
// allocated block") unless it is the first one. A native pointer (a field
// of a native struct, an element of a native array, the AOT mode) can be
// inside its object too. objectStart tells them apart as the runtime does,
// with runtime.findObject and the span of the object.

//go:linkname runtimeFindObject runtime.findObject
func runtimeFindObject(p, refBase, refOff uintptr) (base uintptr, s unsafe.Pointer, objIndex uintptr)

const (
	wordSize = unsafe.Sizeof(uintptr(0))
	// mallocHeaderSize is gc.MallocHeaderSize (internal/runtime/gc): 8 on
	// every platform, not one word (8-byte alignment on 32-bit platforms).
	mallocHeaderSize = 8
	// minSizeForMallocHeader is gc.MinSizeForMallocHeader: an object with
	// pointers and of a larger size class has a malloc header (512 bytes
	// on 64-bit platforms, 128 on 32-bit platforms).
	minSizeForMallocHeader = wordSize * 8 * wordSize
	// maxTinySize is the limit of the tiny allocator (runtime/malloc.go):
	// the runtime allows a finalizer on a pointer inside an object when the
	// type of the pointer has no pointers and is smaller.
	maxTinySize = 16
	// spanScanWords is how many words of a runtime.mspan the probe reads:
	// the struct is longer than that on every platform.
	spanScanWords = 20
)

// spanLayout holds the offsets of the fields elemsize and spanclass in
// runtime.mspan.
type spanLayout struct {
	elemOff, classOff uintptr
	ok                bool
}

// spans is the layout found by the start-up probe; spanLayoutOK is false
// when the probe fails (TestRuntimeCouplings). Then objectStart
// only knows that a pointer to the base of its object is the start, and
// PanicOnFatal refuses every other pointer: the VM never calls
// runtime.SetFinalizer with a pointer that it cannot prove to be the start
// of its object.
var (
	spans        = probeSpanLayout()
	spanLayoutOK = spans.ok
)

// objectStart reports whether p is where its object starts, as
// runtime.SetFinalizer computes it: the base of the object, after the
// malloc header when the object has one (a small size class with pointers,
// larger than minSizeForMallocHeader). Only the object of p and its span
// are read (findObject of another address can end the process with "found
// bad pointer in Go heap"). inHeap is false when p is not in the heap (the
// runtime ignores a pointer to a global, and throws on other memory).
func objectStart(p uintptr) (start, inHeap bool) { return spans.start(p) }

func (l spanLayout) start(p uintptr) (start, inHeap bool) {
	base, s, _ := runtimeFindObject(p, 0, 0)
	if base == 0 {
		return false, false
	}
	if p == base {
		// The base of an object with a malloc header is the header: no
		// pointer of the program points there.
		return true, true
	}
	if !l.ok || p != base+mallocHeaderSize {
		return false, true
	}
	return l.hasHeader(s), true
}

// hasHeader reports whether the objects of the span s have a malloc header
// (runtime.SetFinalizer: !noscan, a size class, !heapBitsInSpan).
func (l spanLayout) hasHeader(s unsafe.Pointer) bool {
	elemsize := *(*uintptr)(unsafe.Add(s, l.elemOff))
	class := *(*uint8)(unsafe.Add(s, l.classOff))
	return class&1 == 0 && class>>1 != 0 && elemsize > minSizeForMallocHeader
}

// probeSpanLayout finds the offsets of elemsize and spanclass in
// runtime.mspan with objects of known sizes, then checks objectStart on
// them, with and without a malloc header: each field must be at exactly
// one offset of the first words of the span.
func probeSpanLayout() spanLayout {
	const w = int(wordSize)
	type probe struct {
		p      unsafe.Pointer
		size   uintptr // a size class or a number of pages: the elemsize
		noscan bool
		large  bool
		header bool
	}
	scanSmall := new([4]*int)                          // 16 or 32 bytes
	scanLimit := new([8 * w]*int)                      // minSizeForMallocHeader: no header
	scanAbove := new([8*w + 1]*int)                    // the next size class: a header
	scan2K := new([(2048 - mallocHeaderSize) / w]*int) // with the header: 2048 bytes
	scan4K := new([(4096 - mallocHeaderSize) / w]*int) // with the header: 4096 bytes
	noscan2K := new([2048]byte)                        // no pointers, no header
	noscan64 := new([64]byte)                          // not tiny
	scanLarge := new([131072 / w]*int)                 // a large object: no header
	noscanLarge := new([131072]byte)                   // a large object without pointers
	probeSink = []any{scanSmall, scanLimit, scanAbove, scan2K, scan4K, noscan2K, noscan64, scanLarge, noscanLarge}
	defer func() { probeSink = nil }()
	aboveSize := uintptr(0) // the size class of scanAbove is not known here
	probes := []probe{
		{unsafe.Pointer(scanSmall), unsafe.Sizeof(*scanSmall), false, false, false},
		{unsafe.Pointer(scanLimit), unsafe.Sizeof(*scanLimit), false, false, false},
		{unsafe.Pointer(scanAbove), aboveSize, false, false, true},
		{unsafe.Pointer(scan2K), 2048, false, false, true},
		{unsafe.Pointer(scan4K), 4096, false, false, true},
		{unsafe.Pointer(noscan2K), 2048, true, false, false},
		{unsafe.Pointer(noscan64), 64, true, false, false},
		{unsafe.Pointer(scanLarge), 131072, false, true, false},
		{unsafe.Pointer(noscanLarge), 131072, true, true, false},
	}
	spans := make([]unsafe.Pointer, len(probes))
	for i, pr := range probes {
		base, s, _ := runtimeFindObject(uintptr(pr.p), 0, 0)
		want := base
		if pr.header {
			want += mallocHeaderSize
		}
		if base == 0 || s == nil || uintptr(pr.p) != want {
			return spanLayout{}
		}
		spans[i] = s
	}
	var elemOff, classOff uintptr
	found := 0
	for o := uintptr(0); o < spanScanWords*wordSize; o += wordSize {
		match := true
		for i, pr := range probes {
			if pr.size != 0 && *(*uintptr)(unsafe.Add(spans[i], o)) != pr.size {
				match = false
				break
			}
		}
		if match {
			elemOff, found = o, found+1
		}
	}
	if found != 1 {
		return spanLayout{}
	}
	found = 0
	for o := uintptr(0); o < spanScanWords*wordSize; o++ {
		class := func(i int) uint8 { return *(*uint8)(unsafe.Add(spans[i], o)) }
		match := true
		for i, pr := range probes {
			c := class(i)
			switch {
			case pr.large:
				match = match && c == b2u8(pr.noscan)
			default:
				match = match && c>>1 != 0 && c&1 == b2u8(pr.noscan)
			}
		}
		// Same size, with and without pointers: the same size class.
		match = match && class(3)^class(5) == 1
		// Other sizes: other size classes.
		match = match && class(0)>>1 != class(3)>>1 && class(3)>>1 != class(4)>>1 && class(1)>>1 != class(2)>>1
		if match {
			classOff, found = o, found+1
		}
	}
	if found != 1 {
		return spanLayout{}
	}
	l := spanLayout{elemOff, classOff, true}
	for _, pr := range probes {
		p := uintptr(pr.p)
		if start, inHeap := l.start(p); !start || !inHeap {
			return spanLayout{}
		}
		// The second word, and the second word after the header.
		for _, q := range []uintptr{p + wordSize, p + mallocHeaderSize} {
			if start, inHeap := l.start(q); start || !inHeap {
				return spanLayout{}
			}
		}
	}
	return l
}

func b2u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

var probeSink []any

// hasPointers reports whether a value of type t holds pointers, as the
// runtime type tells it (abi.Type.Pointers).
func hasPointers(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Array:
		return t.Len() > 0 && hasPointers(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if hasPointers(t.Field(i).Type) {
				return true
			}
		}
		return false
	case reflect.Pointer, reflect.UnsafePointer, reflect.Map, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Slice, reflect.String:
		return true
	}
	return false
}

// valueRType is the type of the cells: runtime.SetFinalizer gets a *Value.
var valueRType = reflect.TypeFor[Value]()

// checkFinalizerStart panics with the fatal error of runtime.SetFinalizer
// when p, a pointer to a value of type elem, is not the start of its
// object, with PanicOnFatal. A pointer into an object that the runtime
// allows (elem without pointers and smaller than maxTinySize: the object
// can come from the tiny allocator) passes.
func (m *Machine) checkFinalizerStart(p unsafe.Pointer, elem reflect.Type) {
	if !m.panicOnFatal || p == nil {
		return
	}
	if elem.Size() < maxTinySize && !hasPointers(elem) {
		return
	}
	if start, inHeap := objectStart(uintptr(p)); inHeap && !start {
		panic(&FatalError{"runtime.SetFinalizer: pointer not at beginning of allocated block"})
	}
}

// finalized holds the cells that have a finalizer of the VM (the address:
// an object with a finalizer is not freed before the finalizer runs, and
// the finalizer removes it), for "finalizer already set".
var finalized sync.Map // uintptr -> struct{}

// finalizeMu makes the check of finalized, runtime.SetFinalizer and the
// update of finalized one step (the hook in storage.go).
var finalizeMu sync.Mutex

// checkFinalizer panics with the fatal error of runtime.SetFinalizer(obj,
// fin) when the call would throw, with PanicOnFatal. iv and fiv are the
// interfaces of obj and fin (nil for a nil interface).
func (m *Machine) checkFinalizer(iv, fiv *Iface) {
	if !m.panicOnFatal {
		return
	}
	fail := func(msg string) { panic(&FatalError{"runtime.SetFinalizer: " + msg}) }
	if iv == nil {
		fail("first argument is nil")
	}
	ot := iv.T.T
	switch {
	case ot != nil:
		if _, ok := ot.Underlying().(*types.Pointer); !ok {
			fail("first argument is " + finTypeString(ot) + ", not pointer")
		}
	case iv.T.Native != nil && iv.T.Native.Kind() != reflect.Pointer:
		fail("first argument is " + iv.T.Native.String() + ", not pointer")
	}
	switch p := iv.V.R.(type) {
	case nil:
		fail("pointer not in allocated block")
	case *Value:
		if p == nil {
			fail("pointer not in allocated block")
		}
		m.checkFinalizerStart(unsafe.Pointer(p), valueRType)
	case *byte:
		if p == nil {
			fail("pointer not in allocated block")
		}
	case reflect.Value:
		if p.Kind() == reflect.Pointer && p.IsNil() {
			fail("pointer not in allocated block")
		}
		if p.Kind() == reflect.Pointer {
			m.checkFinalizerStart(p.UnsafePointer(), p.Type().Elem())
		}
	}
	if fiv == nil {
		return
	}
	if ft := fiv.T.T; ft != nil {
		sig, ok := ft.Underlying().(*types.Signature)
		if !ok {
			fail("second argument is " + finTypeString(ft) + ", not a function")
		}
		if ot != nil {
			pass := "cannot pass " + finTypeString(ot) + " to finalizer " + finTypeString(ft)
			if sig.Variadic() {
				fail(pass + " because dotdotdot")
			}
			if sig.Params().Len() != 1 || !types.AssignableTo(ot, sig.Params().At(0).Type()) {
				fail(pass)
			}
		}
	} else if fiv.T.Native != nil && fiv.T.Native.Kind() != reflect.Func {
		fail("second argument is " + fiv.T.Native.String() + ", not a function")
	}
	// "finalizer already set" is checked by the hook (storage.go), under
	// finalizeMu with the native call.
}

// finTypeString writes t as the runtime does: package names, not paths.
func finTypeString(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string { return p.Name() })
}
