package interp

import (
	"reflect"
	"runtime"
	"testing"
	"time"
)

// gcSettleGornesh runs the collector enough cycles for finalizers queued on
// dropped wrappers to have executed.
func gcSettleGornesh() {
	for k := 0; k < 4; k++ {
		runtime.GC()
		runtime.Gosched()
	}
	time.Sleep(20 * time.Millisecond)
	runtime.GC()
}

// TestGorneshFuncMetaReclaimedWhenHostDropsWrapper pins the weak funcval-keyed
// registry decision: a registry entry whose wrapper becomes unreachable is
// reclaimed by the finalizer armed at insertion, instead of being retained
// forever by the registry key (LEARNINGS 2026-08-30, weak funcval-keyed
// registry). The entry is registered exactly as registration paths do, with
// the wrapper kept alive only by the local variable.
func TestGorneshFuncMetaReclaimedWhenHostDropsWrapper(t *testing.T) {
	i := New(Options{})
	fn := reflect.MakeFunc(reflect.TypeOf(func() int { return 0 }), func([]reflect.Value) []reflect.Value { return nil })
	ref, ok := funcvalRefOf(fn)
	if !ok {
		t.Fatal("no funcval for the wrapper")
	}
	i.funcMu.Lock()
	i.insertFuncMetaEntryLocked(ref, interpretedFuncMeta{typ: fn.Type()}, nil)
	i.funcMu.Unlock()
	if got := funcMetaCountGornesh(i); got != 1 {
		t.Fatalf("funcMeta entries after registration = %d, want 1", got)
	}
	// Drop the wrapper and let the finalizer observe the drop.
	runtime.KeepAlive(fn)
	fn = reflect.Value{}
	gcSettleGornesh()
	if got := funcMetaCountGornesh(i); got != 0 {
		t.Fatalf("funcMeta entries after dropping the wrapper = %d, want 0", got)
	}
}

// TestGorneshFuncMetaRetainedWhileHostKeepsWrapper: a host-retained wrapper
// keeps its metadata across collections and the metadata stays resolvable.
func TestGorneshFuncMetaRetainedWhileHostKeepsWrapper(t *testing.T) {
	i := New(Options{})
	v, err := i.Eval(`func() func() int { c := 0; return func() int { c++; return c } }()`)
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := v.Interface().(func() int)
	if !ok {
		t.Fatalf("returned value is %T, want func() int", v.Interface())
	}
	before := funcMetaCountGornesh(i)
	if before == 0 {
		t.Fatal("no metadata registered for the returned closure")
	}
	gcSettleGornesh()
	if after := funcMetaCountGornesh(i); after < before {
		t.Fatalf("metadata reclaimed while the host still holds the wrapper: %d -> %d", before, after)
	}
	if got := fn(); got != 1 {
		t.Fatalf("retained wrapper call = %d, want 1", got)
	}
	if got := fn(); got != 2 {
		t.Fatalf("retained wrapper call = %d, want 2", got)
	}
	runtime.KeepAlive(fn)
}

// TestGorneshFuncMetaFinalizerGenerationGuard: a stale finalizer (generation
// mismatch) must not evict a newer registration that reused the address.
func TestGorneshFuncMetaFinalizerGenerationGuard(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`f := func() int { return 1 }`); err != nil {
		t.Fatal(err)
	}
	i.funcMu.RLock()
	var key uintptr
	var meta interpretedFuncMeta
	for k, m := range i.funcMeta {
		key, meta = k, m
		break
	}
	i.funcMu.RUnlock()
	if key == 0 {
		t.Fatal("no registry entry found for the declared function")
	}
	// Simulate a stale finalizer from a previous occupant of the address.
	i.evictFuncMetaAtFinalizer(key, meta.generation+1)
	if got := funcMetaCountGornesh(i); got == 0 {
		t.Fatal("stale finalizer evicted a live entry")
	}
	// A matching generation must evict.
	i.evictFuncMetaAtFinalizer(key, meta.generation)
	if got := funcMetaCountGornesh(i); got != 0 {
		t.Fatalf("funcMeta entries after matched-generation eviction = %d, want 0", got)
	}
}

// TestGorneshWeakRegistryBoundedAcrossSinkedClosures hands interpreted
// closures to a native sink through a declared function, so no root result
// cell pins them. The sink keeps only the latest callback; every earlier
// wrapper becomes unreachable and neither it nor its metadata may accumulate.
func TestGorneshWeakRegistryBoundedAcrossSinkedClosures(t *testing.T) {
	i := New(Options{})
	var sink func() int
	if err := i.Use(Exports{"sink/sink": {
		// Store keeps only the most recent callback.
		"Store": reflect.ValueOf(func(fn func() int) { sink = fn }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "sink"
func makeClosure() func() int { c := 0; return func() int { c++; return c } }
for k := 0; k < 300; k++ { sink.Store(makeClosure()) }
`); err != nil {
		t.Fatal(err)
	}
	if sink == nil {
		t.Fatal("sink never stored a callback")
	}
	if got := sink(); got != 1 {
		t.Fatalf("latest sinked callback = %d, want 1", got)
	}
	gcSettleGornesh()
	if got := funcMetaCountGornesh(i); got > 4 {
		t.Fatalf("funcMeta entries after 300 sinked closures = %d, want a bounded count", got)
	}
	runtime.KeepAlive(&sink)
}

// TestGorneshWeakRegistryKeepsRebindingForGlobals: wrappers reachable from
// package-level variables are never finalized and keep re-binding.
func TestGorneshWeakRegistryKeepsRebindingForGlobals(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
cb := func() int { return 42 }
res := cb()
`); err != nil {
		t.Fatal(err)
	}
	gcSettleGornesh()
	v, err := i.Eval(`cb()`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Interface(); got != 42 {
		t.Fatalf("global callback call = %v, want 42", got)
	}
	if got := funcMetaCountGornesh(i); got == 0 {
		t.Fatal("metadata for the global-reachable callback was reclaimed")
	}
}

// TestGorneshWeakRegistryPurgeStillWorks: PurgeRetainedFuncs remains the
// explicit reclamation path for wrappers the interpreter still pins (e.g. the
// last Eval result cell) or the host retains.
func TestGorneshWeakRegistryPurgeStillWorks(t *testing.T) {
	i := New(Options{})
	v, err := i.Eval(`func() func() int { c := 0; return func() int { c++; return c } }()`)
	if err != nil {
		t.Fatal(err)
	}
	fn := v.Interface().(func() int)
	before := funcMetaCountGornesh(i)
	if before == 0 {
		t.Fatal("no metadata registered")
	}
	if removed := i.PurgeRetainedFuncs(); removed == 0 {
		t.Fatal("purge removed nothing while the wrapper is only host-retained")
	}
	if after := funcMetaCountGornesh(i); after >= before {
		t.Fatalf("purge did not reduce the registry: %d -> %d", before, after)
	}
	if got := fn(); got != 1 {
		t.Fatalf("purged wrapper remains callable, got %d want 1", got)
	}
	runtime.KeepAlive(fn)
}
