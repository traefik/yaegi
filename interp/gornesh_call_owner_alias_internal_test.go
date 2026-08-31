package interp

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/traefik/yaegi/stdlib"
)

func TestGorneshCallOwnerBindingPreservesCrossArgumentAliases(t *testing.T) {
	type callback = func()
	var deferredSame bool
	samePointer := func(first, second *callback) bool { return first == second }
	sameMap := func(first, second map[string]callback) bool {
		return reflect.ValueOf(first).Pointer() == reflect.ValueOf(second).Pointer()
	}
	sameSlice := func(first, second []callback) bool {
		return reflect.ValueOf(first).Pointer() == reflect.ValueOf(second).Pointer()
	}
	recordDeferred := func(
		firstPointer, secondPointer *callback,
		firstMap, secondMap map[string]callback,
		firstSlice, secondSlice []callback,
	) {
		deferredSame = samePointer(firstPointer, secondPointer) &&
			sameMap(firstMap, secondMap) && sameSlice(firstSlice, secondSlice)
	}

	i := New(Options{})
	if err := i.Use(Exports{"alias/alias": {
		"SamePointer":    reflect.ValueOf(samePointer),
		"SameMap":        reflect.ValueOf(sameMap),
		"SameSlice":      reflect.ValueOf(sameSlice),
		"RecordDeferred": reflect.ValueOf(recordDeferred),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "alias"
func callOwnerAliasGornesh() bool {
	callback := func() {}
	pointer := &callback
	mapping := map[string]func(){"callback": callback}
	slice := []func(){callback}
	defer alias.RecordDeferred(pointer, pointer, mapping, mapping, slice, slice)
	return alias.SamePointer(pointer, pointer) &&
		alias.SameMap(mapping, mapping) && alias.SameSlice(slice, slice)
}

callOwnerAliasGornesh()`)
	if err != nil {
		t.Fatalf("evaluate call-owner aliases: %v", err)
	}
	if !value.Bool() {
		t.Fatal("native call received distinct copies of an aliased argument")
	}
	if !deferredSame {
		t.Fatal("deferred native call received distinct copies of an aliased argument")
	}
}

func TestGorneshCallOwnerBindingLeavesNativeOnlyCyclicValuesUnchanged(t *testing.T) {
	nativeCallback := func() {}
	sameCallbackPointer := func(first, second *func()) bool { return first == second }
	sameCyclicMap := func(first, second map[string]interface{}) bool {
		return reflect.ValueOf(first).Pointer() == reflect.ValueOf(second).Pointer()
	}
	i := New(Options{})
	if err := i.Use(Exports{"alias/alias": {
		"NativeCallback":      reflect.ValueOf(nativeCallback),
		"SameCallbackPointer": reflect.ValueOf(sameCallbackPointer),
		"SameCyclicMap":       reflect.ValueOf(sameCyclicMap),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "alias"
native := alias.NativeCallback
pointer := &native
cyclic := map[string]interface{}{}
cyclic["self"] = cyclic
cyclic["callback"] = alias.NativeCallback
alias.SameCallbackPointer(pointer, pointer) && alias.SameCyclicMap(cyclic, cyclic)`)
	if err != nil {
		t.Fatalf("evaluate native-only cyclic aliases: %v", err)
	}
	if !value.Bool() {
		t.Fatal("native-only callback values were needlessly copied")
	}
}

func TestGorneshCallOwnerBindingPreservesOverlappingSliceMutation(t *testing.T) {
	called := false
	mutate := func(first, second []func()) bool {
		first[1] = func() { called = true }
		second[0]()
		return called
	}
	i := New(Options{})
	if err := i.Use(Exports{"alias/alias": {
		"Mutate": reflect.ValueOf(mutate),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "alias"
callbacks := []func(){func(){}, func(){}, func(){}}
alias.Mutate(callbacks[:2], callbacks[1:])`)
	if err != nil {
		t.Fatalf("evaluate overlapping callback slices: %v", err)
	}
	if !value.Bool() || !called {
		t.Fatal("native mutation did not preserve overlapping slice aliasing")
	}
}

func TestGorneshCallOwnerBindingPreservesOverlappingSlicesForInterpretedTarget(t *testing.T) {
	i := New(Options{})
	value, err := i.Eval(`
func callOwnerMutateOverlappingGornesh(first, second []func()) bool {
	called := false
	first[1] = func() { called = true }
	second[0]()
	return called
}
callbacks := []func(){func(){}, func(){}, func(){}}
callOwnerMutateOverlappingGornesh(callbacks[:2], callbacks[1:])`)
	if err != nil {
		t.Fatalf("evaluate interpreted overlapping callback slices: %v", err)
	}
	if !value.Bool() {
		t.Fatal("interpreted call did not preserve overlapping slice aliasing")
	}
}

func TestGorneshCallOwnerBindingLetsNativeSortMutateOriginalSlice(t *testing.T) {
	i := New(Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "sort"
type callOwnerSortItemGornesh struct {
	Key int
	Callback func()
}
items := []callOwnerSortItemGornesh{
	{Key: 2, Callback: func(){}},
	{Key: 1, Callback: func(){}},
}
sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
items[0].Key`)
	if err != nil {
		t.Fatalf("evaluate native sort mutation: %v", err)
	}
	if got := value.Interface().(int); got != 1 {
		t.Fatalf("first key after native sort = %d, want 1", got)
	}
}

// Nested callbacks in mutable containers cannot be replaced without changing
// Go container identity. They therefore stay bound to their creation owner;
// cancellation fences them instead of attaching them to a later Eval root.
func TestGorneshCallOwnerBindingFencesCanceledNestedMutableCallback(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"alias/alias": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
			close(finished)
		}),
		"CallMap": reflect.ValueOf(func(callbacks map[string]func()) { callbacks["run"]() }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "alias"
var callOwnerNestedRunsGornesh int
var callOwnerNestedCallbacksGornesh map[string]func()`); err != nil {
		t.Fatalf("define nested callback globals: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `
callOwnerNestedCallbacksGornesh = map[string]func(){
	"run": func() { callOwnerNestedRunsGornesh++ },
}
alias.Block()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("blocking native call was not entered")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled nested callback Eval returned nil error")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("blocking native call did not finish")
	}
	waitForFuncSweepGornesh(i)

	value, err := i.Eval(`
callOwnerNestedRunsGornesh = 100
alias.CallMap(callOwnerNestedCallbacksGornesh)
callOwnerNestedRunsGornesh`)
	if err != nil {
		t.Fatalf("invoke canceled nested callback after detach: %v", err)
	}
	if got := value.Interface().(int); got != 100 {
		t.Fatalf("canceled nested callback attached to later root: runs=%d, want 100", got)
	}
}
