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
// allocated block") unless it is the first one. cellStart tells them apart
// with runtime.findObject.

//go:linkname runtimeFindObject runtime.findObject
func runtimeFindObject(p, refBase, refOff uintptr) (base uintptr, s unsafe.Pointer, objIndex uintptr)

const wordSize = unsafe.Sizeof(uintptr(0))

// valueType is the address of the type of Value in the runtime (the first
// word of an interface that holds a Value).
var valueType = func() uintptr {
	var v any = Value{}
	return uintptr((*[2]unsafe.Pointer)(unsafe.Pointer(&v))[0])
}()

// cellStart reports whether the cell c is where its object starts, as
// SetFinalizer computes it: the base of the object, after the malloc header
// when the object has one. A Value has pointers, so a large []Value (more
// than 512 bytes, runtime/mbitmap.go) starts with a header of one word that
// holds the type of its elements: Value. Only the object of c is read
// (findObject of another address can end the process with "found bad
// pointer in Go heap"). ok is false when c is not in the heap.
func cellStart(c *Value) (start, ok bool) {
	p := uintptr(unsafe.Pointer(c))
	base, _, _ := runtimeFindObject(p, 0, 0)
	if base == 0 {
		return false, false
	}
	if p == base {
		return true, true
	}
	if p != base+wordSize {
		return false, true
	}
	// The word before c is base, in the object of c.
	header := atomic.LoadUintptr((*uintptr)(unsafe.Add(unsafe.Pointer(c), -int(wordSize))))
	return header == valueType, true
}

// cellStartOK is the start-up probe of cellStart (TestRuntimeCouplings):
// without it, the check of interior pointers is off and SetFinalizer keeps
// the Go behaviour.
var cellStartOK = func() bool {
	one := new(Value)
	small := make([]Value, 4)
	header := make([]Value, 64)  // 1536 bytes: a header
	large := make([]Value, 2000) // 48000 bytes: a large object, no header
	pair := new(struct {
		x uintptr
		v Value
	})
	// In the heap, not on the stack (escape analysis).
	cellProbeSink = []any{one, small, header, large, pair}
	defer func() { cellProbeSink = nil }()
	for _, c := range []struct {
		v    *Value
		want bool
	}{
		{one, true}, {&small[0], true}, {&small[1], false}, {&header[0], true}, {&header[1], false},
		{&large[0], true}, {&large[1], false}, {&pair.v, false},
	} {
		if got, ok := cellStart(c.v); !ok || got != c.want {
			return false
		}
	}
	return true
}()

var cellProbeSink []any

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
	cell, isCell := iv.V.R.(*Value)
	switch p := iv.V.R.(type) {
	case nil:
		fail("pointer not in allocated block")
	case *Value:
		if p == nil {
			fail("pointer not in allocated block")
		}
	case *byte:
		if p == nil {
			fail("pointer not in allocated block")
		}
	case reflect.Value:
		if p.Kind() == reflect.Pointer && p.IsNil() {
			fail("pointer not in allocated block")
		}
	}
	if isCell && cellStartOK {
		if start, ok := cellStart(cell); ok && !start {
			fail("pointer not at beginning of allocated block")
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
