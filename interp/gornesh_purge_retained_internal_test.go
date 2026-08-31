package interp

import (
	"reflect"
	"runtime"
	"testing"
)

func TestGorneshPurgeRetainedFuncsFreesDroppedEvals(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func makePurgeClosureGornesh() func() int {
	n := 0
	return func() int { n++; return n }
}`); err != nil {
		t.Fatalf("define makePurgeClosureGornesh: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	kept := make([]func() int, 0, 3)
	for round := 0; round < 3; round++ {
		value, err := i.Eval(`makePurgeClosureGornesh()`)
		if err != nil {
			t.Fatalf("closure Eval round %d: %v", round, err)
		}
		callback := value.Interface().(func() int)
		if got := callback(); got != 1 {
			t.Fatalf("closure round %d returned %d, want 1", round, got)
		}
		kept = append(kept, callback)
	}
	grown := funcMetaCountGornesh(i)
	if grown <= baseline {
		t.Fatalf("metadata after %d closure Evals = %d, want more than baseline %d", len(kept), grown, baseline)
	}
	kept = nil
	waitForFuncSweepGornesh(i)
	removed := i.PurgeRetainedFuncs()
	if removed == 0 {
		t.Fatal("purge removed no metadata for dropped closure Evals")
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after purge = %d, want baseline %d", got, baseline)
	}
	if again := i.PurgeRetainedFuncs(); again != 0 {
		t.Fatalf("second purge removed %d entries, want 0", again)
	}
}

func TestGorneshPurgeRetainedCallbackStaysCallableAndStale(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var saved func()
	i := New(Options{})
	if err := i.Use(Exports{"purgereflect/purgereflect": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"RoundTrip":  reflect.ValueOf(func(f func()) { f() }),
		"GetWrapper": reflect.ValueOf(func() func() { return saved }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "purgereflect"
var purgeStaleGlobalGornesh int
func makePurgeStaleClosureGornesh() func() {
	return func() { purgeStaleGlobalGornesh++ }
}`); err != nil {
		t.Fatalf("define purge stale closure: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	value, err := i.Eval(`makePurgeStaleClosureGornesh()`)
	if err != nil {
		t.Fatalf("create stale closure: %v", err)
	}
	callback := value.Interface().(func())
	saved = callback
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("metadata after retained closure = %d, want more than baseline %d", got, baseline)
	}
	if removed := i.PurgeRetainedFuncs(); removed == 0 {
		t.Fatal("purge removed no metadata for the host-retained closure")
	}
	purged := funcMetaCountGornesh(i)

	// A purged value remains callable while its original root is current.
	callback()
	value, err = i.Eval(`purgeStaleGlobalGornesh`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("purged callback write: value=%v err=%v", value, err)
	}
	// lookupInterpretedFunc must miss it: no metadata may be re-registered
	// when the purged value crosses an Eval boundary again.
	if _, _, ok := i.lookupInterpretedFunc(reflect.ValueOf(callback)); ok {
		t.Fatal("purged callback metadata is still discoverable")
	}
	if _, err := i.Eval(`purgereflect.RoundTrip(purgereflect.GetWrapper())`); err != nil {
		t.Fatalf("round-trip purged callback: %v", err)
	}
	waitForFuncSweepGornesh(i)
	if got := funcMetaCountGornesh(i); got != purged {
		t.Fatalf("metadata after purged callback round trip = %d, want %d", got, purged)
	}
	value, err = i.Eval(`purgeStaleGlobalGornesh`)
	if err != nil || value.Interface() != 2 {
		t.Fatalf("purged callback round-trip write: value=%v err=%v", value, err)
	}

	// Documented stale-root contract: after a cancel/detach the purged value
	// still executes, but its writes land in the abandoned root and stay
	// invisible through Globals().
	cancelBlockedFuncMetaEvalGornesh(t, i, `purgereflect.Block()`, entered, release)
	if _, err := i.Eval(`purgeStaleGlobalGornesh = 100`); err != nil {
		t.Fatalf("reset global on detached root: %v", err)
	}
	callback()
	value, err = i.Eval(`purgeStaleGlobalGornesh`)
	if err != nil || value.Interface() != 100 {
		t.Fatalf("purged callback wrote through the detached root into the live one: value=%v err=%v", value, err)
	}
	for name, global := range i.Globals() {
		if name == "main/purgeStaleGlobalGornesh" && global.Interface() != 100 {
			t.Fatalf("Globals() saw a detached-root write: %v", global.Interface())
		}
	}
}

func TestGorneshPurgeRetainedFuncsKeepsGlobalsReachable(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"purgereflect/purgereflect": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"RoundTrip": reflect.ValueOf(func(f func() int) int { return f() }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "purgereflect"
var purgeKeptGlobalGornesh int
var purgeKeptClosureGornesh func() int
func installPurgeKeptClosureGornesh() func() int {
	n := 0
	purgeKeptClosureGornesh = func() int { n++; purgeKeptGlobalGornesh = n; return n }
	return purgeKeptClosureGornesh
}
func declaredPurgeFuncGornesh() int { return 7 }`); err != nil {
		t.Fatalf("define globals-reachable closure: %v", err)
	}
	defineBaseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`installPurgeKeptClosureGornesh()`); err != nil {
		t.Fatalf("install globals-reachable closure: %v", err)
	}
	installed := funcMetaCountGornesh(i)
	if installed <= defineBaseline {
		t.Fatalf("metadata after installing globals-reachable closure = %d, want more than %d", installed, defineBaseline)
	}
	if removed := i.PurgeRetainedFuncs(); removed != 0 {
		t.Fatalf("purge removed %d entries while the closure is reachable from a package-level variable", removed)
	}
	if got := funcMetaCountGornesh(i); got != installed {
		t.Fatalf("metadata after globals-reachable purge = %d, want %d", got, installed)
	}
	value, err := i.Eval(`declaredPurgeFuncGornesh()`)
	if err != nil || value.Interface() != 7 {
		t.Fatalf("declared top-level func after purge: value=%v err=%v", value, err)
	}
	// The kept closure still re-binds when passed through a host function
	// argument into a later Eval.
	value, err = i.Eval(`purgereflect.RoundTrip(purgeKeptClosureGornesh); purgeKeptGlobalGornesh`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("kept closure re-bind: value=%v err=%v", value, err)
	}

	// After a detach a globals-reachable closure keeps re-binding to the live
	// root, unlike a purged one.
	cancelBlockedFuncMetaEvalGornesh(t, i, `purgereflect.Block()`, entered, release)
	value, err = i.Eval(`purgereflect.RoundTrip(purgeKeptClosureGornesh); purgeKeptGlobalGornesh`)
	if err != nil || value.Interface() != 2 {
		t.Fatalf("kept closure re-bind after detach: value=%v err=%v", value, err)
	}
	if _, err := i.Eval(`purgeKeptClosureGornesh = nil`); err != nil {
		t.Fatalf("clear kept closure: %v", err)
	}
	waitForFuncSweepGornesh(i)
	i.PurgeRetainedFuncs()
	if got := funcMetaCountGornesh(i); got != defineBaseline {
		t.Fatalf("metadata after dropping kept closure = %d, want baseline %d", got, defineBaseline)
	}
}

func TestGorneshPurgeRetainedFuncsSkipsPendingChannelSends(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var purgePendingChannelGornesh = make(chan func() int, 2)
var purgePendingSequenceGornesh int
func sendPurgePendingCallbackGornesh() int {
	purgePendingSequenceGornesh++
	want := purgePendingSequenceGornesh
	purgePendingChannelGornesh <- func() int { return want }
	return want
}`); err != nil {
		t.Fatalf("define pending channel closure: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`sendPurgePendingCallbackGornesh(); sendPurgePendingCallbackGornesh()`); err != nil {
		t.Fatalf("send pending callbacks: %v", err)
	}
	retained := funcMetaCountGornesh(i)
	if retained <= baseline {
		t.Fatalf("metadata with pending channel sends = %d, want more than baseline %d", retained, baseline)
	}
	if removed := i.PurgeRetainedFuncs(); removed != 0 {
		t.Fatalf("purge removed %d entries while channel sends are undelivered, want 0", removed)
	}
	if got := funcMetaCountGornesh(i); got != retained {
		t.Fatalf("metadata after skipped purge = %d, want %d", got, retained)
	}
	value, err := i.Eval(`(<-purgePendingChannelGornesh)() + (<-purgePendingChannelGornesh)()`)
	if err != nil || value.Interface() != 3 {
		t.Fatalf("receive pending callbacks: value=%v err=%v", value, err)
	}
	waitForFuncSweepGornesh(i)
	if removed := i.PurgeRetainedFuncs(); removed != 0 {
		t.Fatalf("purge removed %d entries after the channel drained, want 0", removed)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after drained channel purge = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshPurgeRetainedFuncsConcurrentPurgeLoop(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"purgereflect/purgereflect": {
		"RoundTrip": reflect.ValueOf(func(f func() int) int { return f() }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "purgereflect"
var purgeRaceChannelGornesh = make(chan func() int, 1)
var purgeRaceGlobalGornesh int
func makePurgeRaceClosureGornesh() func() int {
	n := 0
	return func() int { n++; purgeRaceGlobalGornesh = n; return n }
}
func sendPurgeRaceCallbackGornesh() int {
	n := purgeRaceGlobalGornesh + 1
	purgeRaceChannelGornesh <- func() int { return n }
	return n
}`); err != nil {
		t.Fatalf("define race churn sources: %v", err)
	}
	baseline := funcMetaCountGornesh(i)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			i.PurgeRetainedFuncs()
			runtime.Gosched()
		}
	}()

	var callback func() int
	for iteration := 0; iteration < 30; iteration++ {
		value, err := i.Eval(`makePurgeRaceClosureGornesh()`)
		if err != nil {
			t.Fatalf("race closure Eval iteration %d: %v", iteration, err)
		}
		callback = value.Interface().(func() int)
		if got := callback(); got != 1 {
			t.Fatalf("race callback iteration %d returned %d, want 1", iteration, got)
		}
		if _, err := i.Eval(`sendPurgeRaceCallbackGornesh()`); err != nil {
			t.Fatalf("race send iteration %d: %v", iteration, err)
		}
		if _, err := i.Eval(`(<-purgeRaceChannelGornesh)()`); err != nil {
			t.Fatalf("race receive iteration %d: %v", iteration, err)
		}
		if _, err := i.Eval(`purgereflect.RoundTrip(makePurgeRaceClosureGornesh())`); err != nil {
			t.Fatalf("race round trip iteration %d: %v", iteration, err)
		}
		_ = i.Globals()
		_ = i.Symbols("main")
	}
	close(stop)
	<-done
	callback = nil
	if _, err := i.Eval(`purgeRaceGlobalGornesh = 0`); err != nil {
		t.Fatalf("reset race global: %v", err)
	}
	waitForFuncSweepGornesh(i)
	i.PurgeRetainedFuncs()
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after concurrent churn and final purge = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshPurgeRetainedFuncsCleansDirectFuncsAliases(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var wrapper func() int
	i := New(Options{})
	if err := i.Use(Exports{"purgereflect/purgereflect": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"RoundTrip":  reflect.ValueOf(func(f func() int) int { return f() }),
		"GetWrapper": reflect.ValueOf(func() func() int { return wrapper }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "purgereflect"
var purgeAliasCountGornesh int
func makePurgeAliasClosureGornesh() func() int {
	n := 40
	return func() int { n++; purgeAliasCountGornesh = n; return n }
}`); err != nil {
		t.Fatalf("define alias closure: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	value, err := i.Eval(`makePurgeAliasClosureGornesh()`)
	if err != nil {
		t.Fatalf("create alias closure: %v", err)
	}
	wrapper = value.Interface().(func() int)
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("metadata after retained closure = %d, want more than baseline %d", got, baseline)
	}
	i.funcMu.RLock()
	directCount := len(i.directFuncs)
	i.funcMu.RUnlock()
	if directCount != 0 {
		t.Fatalf("directFuncs before any cross-root activation = %d entries, want 0", directCount)
	}

	// A forced root detach makes the next Eval a new root generation; passing
	// the wrapper through host functions then creates bound aliases and
	// directFuncs activation cache entries sharing the wrapper's group.
	cancelBlockedFuncMetaEvalGornesh(t, i, `purgereflect.Block()`, entered, release)
	if _, err := i.Eval(`purgereflect.RoundTrip(purgereflect.GetWrapper()); purgereflect.GetWrapper()`); err != nil {
		t.Fatalf("round trip wrapper across roots: %v", err)
	}
	value, err = i.Eval(`purgeAliasCountGornesh`)
	if err != nil || value.Interface() != 41 {
		t.Fatalf("cross-root bound wrapper call: value=%v err=%v", value, err)
	}
	i.funcMu.RLock()
	directCount = len(i.directFuncs)
	i.funcMu.RUnlock()
	if directCount == 0 {
		t.Fatal("cross-root activation created no directFuncs entries")
	}
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("metadata with aliases = %d, want more than baseline %d", got, baseline)
	}

	wrapper = nil
	waitForFuncSweepGornesh(i)
	removed := i.PurgeRetainedFuncs()
	if removed == 0 {
		t.Fatal("purge removed no metadata for the dropped wrapper and its aliases")
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after alias purge = %d, want baseline %d", got, baseline)
	}
	i.funcMu.RLock()
	directCount = len(i.directFuncs)
	i.funcMu.RUnlock()
	if directCount != 0 {
		t.Fatalf("directFuncs after purge = %d entries, want 0", directCount)
	}
	if again := i.PurgeRetainedFuncs(); again != 0 {
		t.Fatalf("second purge removed %d entries, want 0", again)
	}
}
