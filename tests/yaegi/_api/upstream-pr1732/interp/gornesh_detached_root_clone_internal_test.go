package interp

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"
	"unsafe"

	stdlibunsafe "github.com/traefik/yaegi/stdlib/unsafe"
)

type DetachedNativeMethodBoxGornesh struct {
	Entered func()
	Block   func()
	Done    func()
	Value   int
}

type (
	DetachedNamedMapGornesh     map[string]int
	DetachedNamedPointerGornesh *int
)

type detachedChannelOrderPayloadGornesh struct {
	Marker   int
	Value    map[string]int
	Callback func()
}

type detachedChannelCycleNodeGornesh struct {
	Next *detachedChannelCycleNodeGornesh
}

type detachedNativeReturnBoxGornesh struct {
	Pointer *int
}

type zeroOffsetAliasBoxGornesh struct {
	X int
}

type EqualExtentInnerGornesh struct {
	X int
}

type equalExtentOuterGornesh struct {
	EqualExtentInnerGornesh
}

func (b *DetachedNativeMethodBoxGornesh) BlockAndMutate() {
	b.Entered()
	b.Block()
	b.Value++
	b.Done()
}

func forceDetachedRootCloneGornesh(t *testing.T, i *Interpreter) {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	if err := i.Use(Exports{"detachedclone/detachedclone": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
			close(finished)
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `import "detachedclone"; detachedclone.Block()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("detached-root blocker was not entered")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled detached-root Eval returned nil error")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("detached-root blocker did not finish")
	}
	waitForFuncSweepGornesh(i)
}

func TestGorneshDetachedRootRemapsGlobalCellPointer(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedCellValueGornesh = 1
var detachedCellPointerGornesh = &detachedCellValueGornesh`); err != nil {
		t.Fatalf("define global pointer: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`detachedCellValueGornesh = 2; *detachedCellPointerGornesh`)
	if err != nil || value.Interface() != 2 {
		t.Fatalf("pointer after detached root: value=%v err=%v", value, err)
	}
	value, err = i.Eval(`*detachedCellPointerGornesh = 3; detachedCellValueGornesh`)
	if err != nil || value.Interface() != 3 {
		t.Fatalf("global after pointer mutation: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootClonesZeroOffsetAliasIndependentOfSlotOrder(t *testing.T) {
	i := New(Options{})
	old := i.frame
	box := &zeroOffsetAliasBoxGornesh{X: 1}
	alias := &box.X
	old.data = []reflect.Value{
		reflect.New(reflect.TypeOf(alias)).Elem(),
		reflect.New(reflect.TypeOf(box)).Elem(),
	}
	old.data[0].Set(reflect.ValueOf(alias))
	old.data[1].Set(reflect.ValueOf(box))
	i.registerOwnedValue(reflect.ValueOf(box), old)
	i.registerOwnedAddress(reflect.ValueOf(alias), old)

	next := old.cloneDetached(make(chan struct{}))
	nextAlias := next.data[0].Interface().(*int)
	nextBox := next.data[1].Interface().(*zeroOffsetAliasBoxGornesh)
	*nextAlias = 7
	if nextBox.X != 7 || nextAlias != &nextBox.X {
		t.Fatalf("zero-offset alias split: alias=%p box field=%p values=%d/%d", nextAlias, &nextBox.X, *nextAlias, nextBox.X)
	}
}

func TestGorneshDetachedRootClonesEqualExtentEmbeddedAliasIndependentOfSlotOrder(t *testing.T) {
	i := New(Options{})
	old := i.frame
	outer := &equalExtentOuterGornesh{EqualExtentInnerGornesh: EqualExtentInnerGornesh{X: 1}}
	inner := &outer.EqualExtentInnerGornesh
	old.data = []reflect.Value{
		reflect.New(reflect.TypeOf(inner)).Elem(),
		reflect.New(reflect.TypeOf(outer)).Elem(),
	}
	old.data[0].Set(reflect.ValueOf(inner))
	old.data[1].Set(reflect.ValueOf(outer))
	i.registerOwnedValue(reflect.ValueOf(outer), old)
	i.registerOwnedAddress(reflect.ValueOf(inner), old)

	next := old.cloneDetached(make(chan struct{}))
	nextInner := next.data[0].Interface().(*EqualExtentInnerGornesh)
	nextOuter := next.data[1].Interface().(*equalExtentOuterGornesh)
	nextInner.X = 7
	if nextOuter.X != 7 || nextInner != &nextOuter.EqualExtentInnerGornesh {
		t.Fatalf("equal-extent embedded alias split: inner=%p outer field=%p values=%d/%d", nextInner, &nextOuter.EqualExtentInnerGornesh, nextInner.X, nextOuter.X)
	}
}

func TestGorneshDetachedRootClonesRawPointerBeforeProvisionalInteriorCell(t *testing.T) {
	i := New(Options{})
	old := i.frame
	pointer := new(int)
	*pointer = 1
	raw := unsafe.Pointer(pointer)
	scratch := reflect.NewAt(reflect.TypeOf(0), raw).Elem()
	old.data = []reflect.Value{
		reflect.New(reflect.TypeOf(raw)).Elem(),
		scratch,
		reflect.New(reflect.TypeOf(pointer)).Elem(),
	}
	old.data[0].Set(reflect.ValueOf(raw))
	old.data[2].Set(reflect.ValueOf(pointer))
	i.registerOwnedValue(reflect.ValueOf(pointer), old)

	next := old.cloneDetached(make(chan struct{}))
	nextRaw := next.data[0].Interface().(unsafe.Pointer)
	nextPointer := next.data[2].Interface().(*int)
	if nextRaw != unsafe.Pointer(nextPointer) {
		t.Fatalf("raw pointer used provisional cell mapping: raw=%p typed=%p", nextRaw, nextPointer)
	}
	*(*int)(nextRaw) = 7
	if *nextPointer != 7 {
		t.Fatalf("raw and typed pointer clones split: raw=%d typed=%d", *(*int)(nextRaw), *nextPointer)
	}
}

func TestGorneshPanicFreezesEvaluatedValueHeader(t *testing.T) {
	i := New(Options{})
	value, err := i.Eval(`
var recoveredScalarHeaderGornesh int
var recoveredMapHeaderGornesh map[string]int
func recoverFrozenScalarHeaderGornesh() {
	value := 1
	defer func() { recoveredScalarHeaderGornesh = recover().(int) }()
	defer func() { value = 2 }()
	panic(value)
}
func recoverFrozenMapHeaderGornesh() {
	value := map[string]int{"original": 1}
	defer func() { recoveredMapHeaderGornesh = recover().(map[string]int) }()
	defer func() { value = map[string]int{"replacement": 2} }()
	panic(value)
}
recoverFrozenScalarHeaderGornesh()
recoverFrozenMapHeaderGornesh()
recoveredScalarHeaderGornesh == 1 && recoveredMapHeaderGornesh["original"] == 1 && len(recoveredMapHeaderGornesh) == 1`)
	if err != nil || value.Interface() != true {
		t.Fatalf("panic value header was not frozen: value=%v err=%v", value, err)
	}
	value, err = i.Eval(`
recoveredNilPanicGornesh := false
func() {
	defer func() { recoveredNilPanicGornesh = recover() != nil }()
	panic(nil)
}()
recoveredNilPanicGornesh`)
	if err != nil || value.Interface() != true {
		t.Fatalf("interpreted panic(nil) recovery: value=%v err=%v", value, err)
	}
	if _, err := i.Eval(`panic(nil)`); err == nil {
		t.Fatal("API-boundary panic(nil) returned nil error")
	}
}

func TestGorneshBufferedChannelSendFreezesEvaluatedMapHeader(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var frozenSendChannelGornesh = make(chan map[string]int, 1)
var frozenSendOriginalGornesh map[string]int
func sendFrozenMapHeaderGornesh() {
	value := map[string]int{"original": 1}
	frozenSendOriginalGornesh = value
	frozenSendChannelGornesh <- value
	value = map[string]int{"replacement": 2}
}
sendFrozenMapHeaderGornesh()`); err != nil {
		t.Fatalf("send frozen map header: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`received := <-frozenSendChannelGornesh; received["mutated"] = 3; frozenSendOriginalGornesh["mutated"]`)
	if err != nil || value.Interface() != 3 {
		t.Fatalf("buffered send map header was retargeted: value=%v err=%v", value, err)
	}
	value, err = i.Eval(`frozenSendOriginalGornesh["mutated"]`)
	if err != nil || value.Interface() != 3 {
		t.Fatalf("buffered send lost original backing mutations: value=%v err=%v", value, err)
	}
}

func TestGorneshUnsafePointerIsAnOwnedAllocationIdentityEdge(t *testing.T) {
	t.Run("repeated detach", func(t *testing.T) {
		i := New(Options{})
		root := i.frame
		pointer := new(int)
		*pointer = 1
		raw := unsafe.Pointer(pointer)
		root.data = []reflect.Value{reflect.New(reflect.TypeOf(raw)).Elem()}
		root.data[0].Set(reflect.ValueOf(raw))
		i.registerOwnedValue(reflect.ValueOf(pointer), root)

		first := root.cloneDetached(make(chan struct{}))
		firstRaw := first.data[0].Interface().(unsafe.Pointer)
		if firstRaw == raw || *(*int)(firstRaw) != 1 {
			t.Fatalf("first raw-only detach did not clone allocation: old=%p new=%p value=%d", raw, firstRaw, *(*int)(firstRaw))
		}
		*(*int)(firstRaw) = 2
		second := first.cloneDetached(make(chan struct{}))
		secondRaw := second.data[0].Interface().(unsafe.Pointer)
		if secondRaw == firstRaw || *(*int)(secondRaw) != 2 {
			t.Fatalf("second raw-only detach did not clone allocation: first=%p second=%p value=%d", firstRaw, secondRaw, *(*int)(secondRaw))
		}
	})

	t.Run("channel panic and host publication", func(t *testing.T) {
		i := New(Options{})
		root := i.frame
		pointer := new(int)
		raw := reflect.ValueOf(unsafe.Pointer(pointer))
		i.registerOwnedValue(reflect.ValueOf(pointer), root)
		i.funcMu.RLock()
		obj := i.ownedObjectLocked(reflect.ValueOf(pointer))
		i.funcMu.RUnlock()
		if obj == nil {
			t.Fatal("owned pointer was not registered")
		}

		channel := reflect.MakeChan(reflect.ChanOf(reflect.BothDir, raw.Type()), 1)
		i.registerOwnedChannel(channel, root)
		i.funcSweepMu.RLock()
		send := i.markInterpretedFuncChannelSend(root, channel, raw)
		i.funcSweepMu.RUnlock()
		if send == nil {
			t.Fatal("raw-only buffered send was not tracked")
		}
		if _, ok := send.objects[obj]; !ok {
			t.Fatal("raw-only buffered send did not retain containing allocation")
		}
		i.funcSweepMu.RLock()
		i.rollbackInterpretedFuncChannelSend(send)
		i.funcSweepMu.RUnlock()

		token := i.beginOwnedPanicLocked(raw)
		if token == nil {
			t.Fatal("raw-only panic token was not created")
		}
		if _, ok := token.objects[obj]; !ok {
			t.Fatal("raw-only panic did not retain containing allocation")
		}
		next := root.cloneDetached(make(chan struct{}))
		adopted := i.adoptOwnedPanicValuesLocked(next, token, raw)
		if len(adopted) != 1 || !adopted[0].IsValid() {
			t.Fatalf("adopt raw-only panic value: %v", adopted)
		}
		adoptedRaw := adopted[0].Interface().(unsafe.Pointer)
		if adoptedRaw == raw.Interface().(unsafe.Pointer) {
			t.Fatal("raw-only panic kept abandoned allocation after detach")
		}
		*(*int)(adoptedRaw) = 7
		if got := *(*int)(adoptedRaw); got != 7 {
			t.Fatalf("adopted raw-only panic allocation = %d, want 7", got)
		}

		i.markOwnedValuesHostShared(adopted[0])
		i.funcMu.RLock()
		adoptedObjects := i.ownedObjectsForValueLocked(adopted[0])
		hostShared := len(adoptedObjects) == 1 && adoptedObjects[0].hostShared
		i.funcMu.RUnlock()
		if !hostShared {
			t.Fatal("host-returned raw pointer did not publish containing allocation")
		}
	})
}

func TestGorneshRawOnlyRecoveredPanicRemainsOwned(t *testing.T) {
	i := New(Options{})
	if err := i.Use(stdlibunsafe.Symbols); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "unsafe"
var rawOnlyRecoveredOwnershipGornesh unsafe.Pointer
func recoverRawOnlyOwnershipGornesh() {
	defer func() { rawOnlyRecoveredOwnershipGornesh = recover().(unsafe.Pointer) }()
	raw := unsafe.Pointer(new(int))
	*(*int)(raw) = 41
	panic(raw)
}
recoverRawOnlyOwnershipGornesh()
0`); err != nil {
		t.Fatalf("recover raw-only owned allocation: %v", err)
	}
	symbol := i.srcPkg["main"]["rawOnlyRecoveredOwnershipGornesh"]
	before := unwrapOwnedValue(i.frame.data[symbol.index])
	i.funcMu.RLock()
	objects := i.ownedObjectsForValueLocked(before)
	var hostShared bool
	var owner, root *frame
	if len(objects) != 0 {
		hostShared = objects[0].hostShared
		owner = objects[0].owner
		if owner != nil {
			root = owner.root
		}
	}
	i.funcMu.RUnlock()
	if len(objects) == 0 {
		t.Fatal("recovered raw-only allocation lost ownership metadata")
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("activate detached root: %v", err)
	}
	after := unwrapOwnedValue(i.frame.data[symbol.index])
	if before.Pointer() == after.Pointer() || *(*int)(after.Interface().(unsafe.Pointer)) != 41 {
		t.Fatalf("recovered raw-only allocation was not detached: before=%x after=%x value=%d hostShared=%v owner=%p ownerRoot=%p oldRoot=%p", before.Pointer(), after.Pointer(), *(*int)(after.Interface().(unsafe.Pointer)), hostShared, owner, root, i.frame.cloneOf)
	}
}

func TestGorneshCustomUnsafeConversionHookPublishesRetainedSource(t *testing.T) {
	var retained *int
	var retainedResult unsafe.Pointer
	customConvert := func(from, to reflect.Type) func(reflect.Value, reflect.Value) {
		if from.Kind() != reflect.Ptr || to.Kind() != reflect.UnsafePointer {
			return nil
		}
		return func(source, destination reflect.Value) {
			result := unsafe.Pointer(source.Pointer())
			if source.Type().Elem().Kind() == reflect.Int {
				retained = source.Interface().(*int)
			} else if source.Type().Elem().Kind() == reflect.Struct {
				result = unsafe.Pointer(source.Elem().Field(0).Pointer())
				retainedResult = result
			}
			destination.SetPointer(result)
		}
	}
	i := New(Options{})
	if err := i.Use(Exports{"github.com/traefik/yaegi/yaegi": {
		"convert": reflect.ValueOf(customConvert),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(stdlibunsafe.Symbols); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "unsafe"
var customUnsafeHookRawGornesh = unsafe.Pointer(new(int))
*(*int)(customUnsafeHookRawGornesh) = 1
type customUnsafeResultHolderGornesh struct { Target *int }
var customUnsafeResultHolderValueGornesh = &customUnsafeResultHolderGornesh{Target: new(int)}
var customUnsafeHookResultGornesh = unsafe.Pointer(customUnsafeResultHolderValueGornesh)
*(*int)(customUnsafeHookResultGornesh) = 3
0`); err != nil {
		t.Fatalf("run custom unsafe conversion: %v", err)
	}
	if retained == nil || *retained != 1 {
		t.Fatalf("custom unsafe hook did not retain source: %v", retained)
	}
	if retainedResult == nil || *(*int)(retainedResult) != 3 {
		t.Fatalf("custom unsafe hook did not retain result: %v", retainedResult)
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`
*(*int)(customUnsafeHookRawGornesh) = 2
*(*int)(customUnsafeHookResultGornesh) = 4`); err != nil {
		t.Fatalf("mutate custom-hook raw pointer after detach: %v", err)
	}
	if *retained != 2 {
		t.Fatalf("custom unsafe hook source was cloned despite native retention: got %d want 2", *retained)
	}
	if *(*int)(retainedResult) != 4 {
		t.Fatalf("custom unsafe hook result was cloned despite native retention: got %d want 4", *(*int)(retainedResult))
	}
}

func TestGorneshDetachedRootRemapsInterfaceCellPointer(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedInterfaceCellGornesh interface{} = 1
var detachedInterfaceCellPointerGornesh = &detachedInterfaceCellGornesh`); err != nil {
		t.Fatalf("define interface cell pointer: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*detachedInterfaceCellPointerGornesh = 2
detachedInterfaceCellPointerGornesh == &detachedInterfaceCellGornesh && detachedInterfaceCellGornesh.(int) == 2`)
	if err != nil || value.Interface() != true {
		t.Fatalf("interface cell pointer after detached root: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootPreservesOwnedMapAndPointerAliases(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
type detachedAliasBoxGornesh struct { Value int }
var detachedAliasMapOneGornesh = map[string]int{"value": 1}
var detachedAliasMapTwoGornesh = detachedAliasMapOneGornesh
var detachedAliasPointerOneGornesh = &detachedAliasBoxGornesh{Value: 1}
var detachedAliasPointerTwoGornesh = detachedAliasPointerOneGornesh`); err != nil {
		t.Fatalf("define aggregate aliases: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedAliasMapOneGornesh["value"] = 2
detachedAliasPointerOneGornesh.Value = 3
detachedAliasMapTwoGornesh["value"]*10 + detachedAliasPointerTwoGornesh.Value`)
	if err != nil || value.Interface() != 23 {
		t.Fatalf("aggregate aliases after detached root: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootPreservesOverlappingSliceAliases(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedSliceBaseGornesh = []int{1, 2, 3, 4}
var detachedSliceLeftGornesh = detachedSliceBaseGornesh[:3]
var detachedSliceRightGornesh = detachedSliceBaseGornesh[1:]`); err != nil {
		t.Fatalf("define overlapping slices: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedSliceLeftGornesh[1] = 7
detachedSliceRightGornesh[1] = 8
detachedSliceRightGornesh[0]*100 + detachedSliceBaseGornesh[2]`)
	if err != nil || value.Interface() != 708 {
		t.Fatalf("overlapping slices after detached root: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootPreservesOwnedCycles(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
type detachedCycleNodeGornesh struct {
	Value int
	Next *detachedCycleNodeGornesh
}
var detachedPointerCycleGornesh = &detachedCycleNodeGornesh{Value: 1}
var detachedMapCycleGornesh = map[string]interface{}{}
detachedPointerCycleGornesh.Next = detachedPointerCycleGornesh
detachedMapCycleGornesh["self"] = detachedMapCycleGornesh`); err != nil {
		t.Fatalf("define aggregate cycles: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedPointerCycleGornesh.Next.Value = 2
detachedMapCycleGornesh["value"] = 3
detachedMapCycleGornesh["self"].(map[string]interface{})["value"].(int)*10 + detachedPointerCycleGornesh.Value`)
	if err != nil || value.Interface() != 32 {
		t.Fatalf("aggregate cycles after detached root: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootRebasesClosureOwnedAggregateAlias(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedClosureMapGornesh map[string]int
var detachedClosureFuncGornesh func()
func installDetachedClosureGornesh() {
	local := map[string]int{"value": 1}
	detachedClosureMapGornesh = local
	detachedClosureFuncGornesh = func() { local["value"]++ }
}
installDetachedClosureGornesh()`); err != nil {
		t.Fatalf("install closure aggregate alias: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`detachedClosureFuncGornesh(); detachedClosureMapGornesh["value"]`)
	if err != nil || value.Interface() != 2 {
		t.Fatalf("closure aggregate alias after detached root: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootRebasesRecursiveCapturedClosure(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedRecursiveClosureGornesh func(int) int
func installDetachedRecursiveClosureGornesh() {
	count := 0
	var local func(int) int
	local = func(depth int) int {
		count++
		if depth > 0 { return local(depth-1) }
		return count
	}
	detachedRecursiveClosureGornesh = local
}
installDetachedRecursiveClosureGornesh()`); err != nil {
		t.Fatalf("install recursive captured closure: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`detachedRecursiveClosureGornesh(2)*10 + detachedRecursiveClosureGornesh(1)`)
	if err != nil || value.Interface() != 35 {
		t.Fatalf("recursive captured closure after detach: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootPreservesSiblingClosureCellAlias(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedSiblingIncrementGornesh func()
var detachedSiblingGetGornesh func() int
func installDetachedSiblingClosuresGornesh() {
	value := 0
	detachedSiblingIncrementGornesh = func() { value++ }
	detachedSiblingGetGornesh = func() int { return value }
}
installDetachedSiblingClosuresGornesh()`); err != nil {
		t.Fatalf("install sibling closures: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`detachedSiblingIncrementGornesh(); detachedSiblingGetGornesh()`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("sibling closure cell alias after detach: value=%v err=%v", value, err)
	}
}

func TestGorneshOwnedObjectMetadataLocalChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func detachedOwnedChurnGornesh() {
	ownedMap := map[string]int{"value": 1}
	ownedSlice := make([]int, 4)
	ownedPointer := new(int)
	if len(ownedMap)+len(ownedSlice)+*ownedPointer == -1 { panic("unreachable") }
}`); err != nil {
		t.Fatalf("define owned-object churn: %v", err)
	}
	i.funcMu.RLock()
	baseline := len(i.ownedObjects)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 50; iteration++ {
		if _, err := i.Eval(`detachedOwnedChurnGornesh(); 0`); err != nil {
			t.Fatalf("owned-object churn %d: %v", iteration, err)
		}
	}
	i.funcMu.RLock()
	got := len(i.ownedObjects)
	i.funcMu.RUnlock()
	if got != baseline {
		t.Fatalf("owned-object metadata after local churn = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshOwnedObjectMetadataDiscardedGoroutineReturnsAreBounded(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"detachedchurn/detachedchurn": {
		"Started":  reflect.ValueOf(func() { started <- struct{}{} }),
		"Block":    reflect.ValueOf(func() { <-release }),
		"Finished": reflect.ValueOf(func() { finished <- struct{}{} }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedchurn"
func detachedDiscardedFactoryGornesh() *int {
	value := new(int)
	detachedchurn.Started()
	detachedchurn.Block()
	detachedchurn.Finished()
	return value
}
func detachedDiscardedOuterGornesh() { go detachedDiscardedFactoryGornesh() }`); err != nil {
		t.Fatalf("define discarded goroutine factory: %v", err)
	}
	i.funcMu.RLock()
	baseline := len(i.ownedObjects)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`detachedDiscardedOuterGornesh()`); err != nil {
			t.Fatalf("launch discarded factory %d: %v", iteration, err)
		}
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatalf("discarded factory %d did not start", iteration)
		}
		release <- struct{}{}
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Fatalf("discarded factory %d did not finish", iteration)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			i.funcMu.RLock()
			count := len(i.ownedObjects)
			i.funcMu.RUnlock()
			if count == baseline {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("discarded factory %d metadata did not return to baseline: got %d want %d", iteration, count, baseline)
			}
			time.Sleep(time.Millisecond)
		}
	}
	i.funcMu.RLock()
	got := len(i.ownedObjects)
	i.funcMu.RUnlock()
	if got != baseline {
		t.Fatalf("owned-object metadata after discarded goroutine returns = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshDiscardedLocalBufferedChannelMetadataIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func discardOwnedBufferedChannelGornesh() {
	channel := make(chan map[string]int, 1)
	channel <- map[string]int{"value": 1}
}
func discardFuncBufferedChannelGornesh() {
	channel := make(chan func(), 1)
	channel <- func() {}
}`); err != nil {
		t.Fatalf("define discarded buffered channel churn: %v", err)
	}
	i.funcMu.RLock()
	ownedBaseline, funcBaseline := len(i.ownedObjects), len(i.funcMeta)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`discardOwnedBufferedChannelGornesh(); discardFuncBufferedChannelGornesh(); 0`); err != nil {
			t.Fatalf("discard buffered channels %d: %v", iteration, err)
		}
	}
	i.funcMu.RLock()
	ownedCount, funcCount := len(i.ownedObjects), len(i.funcMeta)
	i.funcMu.RUnlock()
	if ownedCount != ownedBaseline || funcCount != funcBaseline {
		t.Fatalf("metadata after discarded buffered channels = owned %d funcs %d, want %d/%d", ownedCount, funcCount, ownedBaseline, funcBaseline)
	}
}

func TestGorneshDetachedRootClonesInterpretedFactoryAggregates(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func detachedFactoryMapGornesh() map[string]int { return map[string]int{"value": 1} }
func detachedFactorySliceGornesh() []int { return []int{1, 2} }
func detachedFactoryPointerGornesh() *int { value := 1; return &value }
var detachedFactoryMapValueGornesh = detachedFactoryMapGornesh()
var detachedFactorySliceValueGornesh = detachedFactorySliceGornesh()
var detachedFactoryPointerValueGornesh = detachedFactoryPointerGornesh()`); err != nil {
		t.Fatalf("define interpreted aggregate factories: %v", err)
	}
	global := func(name string) reflect.Value {
		t.Helper()
		symbol := i.srcPkg["main"][name]
		return unwrapOwnedValue(i.frame.data[symbol.index])
	}
	oldMap := global("detachedFactoryMapValueGornesh").Pointer()
	oldSlice := global("detachedFactorySliceValueGornesh").Pointer()
	oldPointer := global("detachedFactoryPointerValueGornesh").Pointer()
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("detach interpreted factory aggregates: %v", err)
	}
	if got := global("detachedFactoryMapValueGornesh").Pointer(); got == oldMap {
		t.Fatalf("factory map was not cloned: pointer=%x", got)
	}
	if got := global("detachedFactorySliceValueGornesh").Pointer(); got == oldSlice {
		t.Fatalf("factory slice was not cloned: pointer=%x", got)
	}
	if got := global("detachedFactoryPointerValueGornesh").Pointer(); got == oldPointer {
		t.Fatalf("factory pointer was not cloned: pointer=%x", got)
	}
}

func TestGorneshDetachedRootClonesAppendConversionAndVariadicSlices(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func detachedVariadicSliceFactoryGornesh(values ...int) []int { return values }
var detachedAppendBaseGornesh = make([]int, 1, 4)
var detachedAppendReuseGornesh = append(detachedAppendBaseGornesh, 2)
var detachedAppendSmallGornesh = make([]int, 1, 1)
var detachedAppendGrowthGornesh = append(detachedAppendSmallGornesh, 2)
var detachedByteConversionGornesh = []byte("abc")
var detachedRuneConversionGornesh = []rune("abc")
var detachedVariadicSliceGornesh = detachedVariadicSliceFactoryGornesh(1, 2, 3)`); err != nil {
		t.Fatalf("define append/conversion/variadic slices: %v", err)
	}
	global := func(name string) reflect.Value {
		t.Helper()
		return unwrapOwnedValue(i.frame.data[i.srcPkg["main"][name].index])
	}
	names := []string{
		"detachedAppendBaseGornesh", "detachedAppendGrowthGornesh",
		"detachedByteConversionGornesh", "detachedRuneConversionGornesh", "detachedVariadicSliceGornesh",
	}
	oldPointers := map[string]uintptr{}
	for _, name := range names {
		oldPointers[name] = global(name).Pointer()
	}
	if global("detachedAppendBaseGornesh").Pointer() != global("detachedAppendReuseGornesh").Pointer() {
		t.Fatal("append-with-capacity did not reuse source before detach")
	}
	if global("detachedAppendSmallGornesh").Pointer() == global("detachedAppendGrowthGornesh").Pointer() {
		t.Fatal("append reallocation did not create distinct backing before detach")
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedAppendReuseGornesh[0] = 7
detachedAppendBaseGornesh[0] == 7`)
	if err != nil || value.Interface() != true {
		t.Fatalf("append alias after detach: value=%v err=%v", value, err)
	}
	for _, name := range names {
		if got := global(name).Pointer(); got == oldPointers[name] {
			t.Fatalf("%s backing was not cloned: pointer=%x", name, got)
		}
	}
	if global("detachedAppendSmallGornesh").Pointer() == global("detachedAppendGrowthGornesh").Pointer() {
		t.Fatal("append reallocation backings merged after detach")
	}
}

func TestGorneshDetachedRootPreservesBinaryNamedAllocationViews(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"detachedtypes/detachedtypes": {
		"NamedMap":     reflect.ValueOf((*DetachedNamedMapGornesh)(nil)),
		"NamedPointer": reflect.ValueOf((*DetachedNamedPointerGornesh)(nil)),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedtypes"
var detachedNamedMapGornesh = detachedtypes.NamedMap{"value": 1}
var detachedPlainMapViewGornesh = map[string]int(detachedNamedMapGornesh)
var detachedPlainPointerGornesh = new(int)
var detachedNamedPointerViewGornesh = detachedtypes.NamedPointer(detachedPlainPointerGornesh)`); err != nil {
		t.Fatalf("define binary named allocation views: %v", err)
	}
	mapSymbol := i.srcPkg["main"]["detachedNamedMapGornesh"]
	pointerSymbol := i.srcPkg["main"]["detachedPlainPointerGornesh"]
	oldMap := unwrapOwnedValue(i.frame.data[mapSymbol.index]).Pointer()
	oldPointer := unwrapOwnedValue(i.frame.data[pointerSymbol.index]).Pointer()
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedNamedMapGornesh["value"] = 2
*detachedNamedPointerViewGornesh = 3
detachedPlainMapViewGornesh["value"]*10 + *detachedPlainPointerGornesh`)
	if err != nil || value.Interface() != 23 {
		t.Fatalf("binary named allocation aliases after detach: value=%v err=%v", value, err)
	}
	if got := unwrapOwnedValue(i.frame.data[mapSymbol.index]).Pointer(); got == oldMap {
		t.Fatalf("binary named map was not cloned: pointer=%x", got)
	}
	if got := unwrapOwnedValue(i.frame.data[pointerSymbol.index]).Pointer(); got == oldPointer {
		t.Fatalf("binary named pointer was not cloned: pointer=%x", got)
	}
}

func TestGorneshDetachedRootPreservesOwnedChannelTransfer(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedOwnedChannelGornesh = make(chan map[string]int, 1)
var detachedOwnedChannelValueGornesh map[string]int
func sendDetachedOwnedChannelGornesh() {
	value := map[string]int{"value": 1}
	detachedOwnedChannelGornesh <- value
}
sendDetachedOwnedChannelGornesh()
detachedOwnedChannelValueGornesh = <-detachedOwnedChannelGornesh
0`); err != nil {
		t.Fatalf("transfer owned map through channel: %v", err)
	}
	symbol := i.srcPkg["main"]["detachedOwnedChannelValueGornesh"]
	channelValue := unwrapOwnedValue(i.frame.data[symbol.index])
	oldPointer := channelValue.Pointer()
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("detach channel-transferred owned map: %v", err)
	}
	if got := unwrapOwnedValue(i.frame.data[symbol.index]).Pointer(); got == oldPointer {
		t.Fatalf("channel-transferred owned map was not cloned: pointer=%x", got)
	}
}

func TestGorneshDetachedRootSnapshotsOwnedBufferedChannelValue(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"detachedpending/detachedpending": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"Finished": reflect.ValueOf(func() { close(finished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedpending"
var detachedPendingChannelGornesh = make(chan map[string]int, 1)
var detachedPendingValueGornesh map[string]int
func sendPendingOwnedMapGornesh() {
	value := map[string]int{"value": 1}
	defer func() {
		value["value"] = 2
		detachedpending.Finished()
	}()
	detachedPendingChannelGornesh <- value
	detachedpending.Block()
}`); err != nil {
		t.Fatalf("define pending owned channel value: %v", err)
	}
	i.funcMu.RLock()
	baseline := len(i.ownedObjects)
	i.funcMu.RUnlock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `sendPendingOwnedMapGornesh()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("pending owned channel sender did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled pending owned channel Eval returned nil error")
	}
	value, err := i.Eval(`
detachedPendingValueGornesh = <-detachedPendingChannelGornesh
detachedPendingValueGornesh["value"]`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("receive detached pending snapshot: value=%v err=%v", value, err)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("old pending sender did not finish")
	}
	value, err = i.Eval(`detachedPendingValueGornesh["value"]`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("old deferred write reached detached receiver: value=%v err=%v", value, err)
	}
	symbol := i.srcPkg["main"]["detachedPendingValueGornesh"]
	i.funcSweepMu.Lock()
	i.frame.mutex.Lock()
	i.frame.data[symbol.index] = reflect.New(i.frame.data[symbol.index].Type()).Elem()
	i.frame.mutex.Unlock()
	i.funcSweepMu.Unlock()
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("detach after pending channel drain: %v", err)
	}
	i.funcMu.RLock()
	got := len(i.ownedObjects)
	i.funcMu.RUnlock()
	if got != baseline {
		t.Fatalf("owned-object metadata after pending channel drain = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshDetachedRootSnapshotsDynamicChannelGraphAndFuncCaptures(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"dynamictoken/dynamictoken": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"Finished": reflect.ValueOf(func() { close(finished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "dynamictoken"
var dynamicTokenMapChannelGornesh = make(chan map[string]*int, 1)
var dynamicTokenFuncChannelGornesh = make(chan func() int, 1)
var dynamicTokenCurrentMapGornesh map[string]*int
var dynamicTokenCurrentFuncGornesh func() int
func sendDynamicTokenValuesGornesh() {
	outer := map[string]*int{}
	dynamicTokenMapChannelGornesh <- outer
	child := new(int)
	*child = 1
	outer["child"] = child
	state := map[string]int{"value": 1}
	callback := func() int { return state["value"] }
	dynamicTokenFuncChannelGornesh <- callback
	defer func() {
		*child = 2
		state["value"] = 2
		dynamictoken.Finished()
	}()
	dynamictoken.Block()
}`); err != nil {
		t.Fatalf("define dynamic channel token values: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `sendDynamicTokenValuesGornesh()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("dynamic channel sender did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled dynamic channel sender returned nil error")
	}
	// Leave the queued values pending across two detached-root generations
	// before receiving them. The second generation must clone the first
	// pending snapshot, not fall back to the canceled sender's raw payload.
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
dynamicTokenCurrentMapGornesh = <-dynamicTokenMapChannelGornesh
	dynamicTokenCurrentFuncGornesh = <-dynamicTokenFuncChannelGornesh
	*dynamicTokenCurrentMapGornesh["child"] * 10 + dynamicTokenCurrentFuncGornesh()`)
	if err != nil || value.Interface() != 11 {
		t.Fatalf("receive dynamic token snapshots: value=%v err=%v", value, err)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("old dynamic token sender did not finish")
	}
	value, err = i.Eval(`*dynamicTokenCurrentMapGornesh["child"] * 10 + dynamicTokenCurrentFuncGornesh()`)
	if err != nil || value.Interface() != 11 {
		t.Fatalf("old writes reached dynamic token snapshots: value=%v err=%v", value, err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`*dynamicTokenCurrentMapGornesh["child"] * 10 + dynamicTokenCurrentFuncGornesh()`)
	if err != nil || value.Interface() != 11 {
		t.Fatalf("dynamic token snapshots after second detach: value=%v err=%v", value, err)
	}
}

func TestGorneshHostVisibleChannelPayloadRemainsHostShared(t *testing.T) {
	var returned map[string]int
	i := New(Options{})
	if err := i.Use(Exports{"detachedchannelhost/detachedchannelhost": {
		"Return": reflect.ValueOf(func() map[string]int { return returned }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedchannelhost"
var detachedHostChannelGornesh = make(chan map[string]int, 1)
var detachedHostReturnedMapGornesh map[string]int
func sendDetachedHostChannelGornesh() {
	detachedHostChannelGornesh <- map[string]int{"value": 1}
}`); err != nil {
		t.Fatalf("define host-visible channel: %v", err)
	}
	channel := unwrapOwnedValue(i.Globals()["detachedHostChannelGornesh"])
	if _, err := i.Eval(`sendDetachedHostChannelGornesh()`); err != nil {
		t.Fatalf("send host-visible channel payload: %v", err)
	}
	received, ok := channel.Recv()
	if !ok {
		t.Fatal("host-visible channel closed before payload")
	}
	returned = received.Interface().(map[string]int)
	returned["value"] = 2
	if _, err := i.Eval(`detachedHostReturnedMapGornesh = detachedchannelhost.Return()`); err != nil {
		t.Fatalf("return host-received map: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`detachedHostReturnedMapGornesh["value"] = 3`); err != nil {
		t.Fatalf("mutate returned host map after detach: %v", err)
	}
	if got := returned["value"]; got != 3 {
		t.Fatalf("host-received payload alias split after detach: got %d want 3", got)
	}
}

func TestGorneshDetachedRootDoesNotTraverseHostVisibleChannelPayload(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedHostRaceChannelGornesh = make(chan map[string]int, 1)
func sendDetachedHostRaceChannelGornesh() {
	detachedHostRaceChannelGornesh <- map[string]int{"value": 1}
}`); err != nil {
		t.Fatalf("define host channel race payload: %v", err)
	}
	channel := unwrapOwnedValue(i.Globals()["detachedHostRaceChannelGornesh"])
	if _, err := i.Eval(`sendDetachedHostRaceChannelGornesh()`); err != nil {
		t.Fatalf("send host channel race payload: %v", err)
	}
	received, ok := channel.Recv()
	if !ok {
		t.Fatal("host race channel closed before payload")
	}
	hostMap := received.Interface().(map[string]int)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Pace the publisher: an unpaced loop starves the detach sequence
		// under -race on loaded CI machines and can blow the test timeout.
		// The test only needs concurrent mutation to exist, not maximum
		// write rate.
		value := 0
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Microsecond):
				hostMap["value"] = value
				value++
			}
		}
	}()
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		close(stop)
		<-done
		t.Fatalf("detach during host payload mutation: %v", err)
	}
	close(stop)
	<-done
}

func TestGorneshDetachedRootPreservesOwnedRecoveredPanic(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedOwnedRecoveredPanicGornesh map[string]int
func panicDetachedOwnedMapGornesh() {
	value := map[string]int{"value": 1}
	panic(value)
}
func recoverDetachedOwnedMapGornesh() {
	defer func() {
		detachedOwnedRecoveredPanicGornesh = recover().(map[string]int)
	}()
	panicDetachedOwnedMapGornesh()
}
recoverDetachedOwnedMapGornesh()
0`); err != nil {
		t.Fatalf("recover owned panic map: %v", err)
	}
	symbol := i.srcPkg["main"]["detachedOwnedRecoveredPanicGornesh"]
	oldPointer := unwrapOwnedValue(i.frame.data[symbol.index]).Pointer()
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("detach recovered owned panic map: %v", err)
	}
	if got := unwrapOwnedValue(i.frame.data[symbol.index]).Pointer(); got == oldPointer {
		t.Fatalf("recovered owned panic map was not cloned: pointer=%x", got)
	}
}

func TestGorneshOwnedObjectMetadataRecoveredPanicChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func recoverAndDiscardOwnedPanicGornesh() {
	defer func() { _ = recover() }()
	panic(map[string]int{"value": 1})
}`); err != nil {
		t.Fatalf("define recovered owned panic churn: %v", err)
	}
	i.funcMu.RLock()
	baseline := len(i.ownedObjects)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`recoverAndDiscardOwnedPanicGornesh(); 0`); err != nil {
			t.Fatalf("recover owned panic churn %d: %v", iteration, err)
		}
	}
	i.funcMu.RLock()
	got := len(i.ownedObjects)
	i.funcMu.RUnlock()
	if got != baseline {
		t.Fatalf("owned-object metadata after recovered panic churn = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshOwnedPanicTokenMutationLifecycleIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func recoverMutatedOwnedPanicGornesh() {
	pointer := new(int)
	value := map[string]interface{}{"pointer": pointer}
	defer func() {
		delete(value, "pointer")
		value["nested"] = map[string]interface{}{"callback": func() {}}
		value["cycle"] = value
		_ = recover()
	}()
	panic(value)
}

func recoverRepanicMutatedOwnedPanicGornesh() {
	pointer := new(int)
	value := map[string]interface{}{"pointer": pointer}
	defer func() { _ = recover() }()
	defer func() {
		delete(value, "pointer")
		value["nested"] = map[string]interface{}{"callback": func() {}}
		recovered := recover()
		panic(recovered)
	}()
	panic(value)
}

func recoverReplacementMutatedOwnedPanicGornesh() {
	first := map[string]interface{}{"pointer": new(int)}
	second := map[string]interface{}{"pointer": new(int)}
	defer func() { _ = recover() }()
	defer func() {
		delete(first, "pointer")
		first["nested"] = map[string]interface{}{"callback": func() {}}
		panic(second)
	}()
	panic(first)
}`); err != nil {
		t.Fatalf("define panic-token mutation churn: %v", err)
	}
	i.funcMu.RLock()
	ownedBaseline, funcBaseline := len(i.ownedObjects), len(i.funcMeta)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`recoverMutatedOwnedPanicGornesh(); recoverRepanicMutatedOwnedPanicGornesh(); recoverReplacementMutatedOwnedPanicGornesh(); 0`); err != nil {
			t.Fatalf("panic-token mutation churn %d: %v", iteration, err)
		}
		i.funcMu.RLock()
		ownedCount, funcCount, tokenCount := len(i.ownedObjects), len(i.funcMeta), len(i.panicTokens)
		activeObjectTokens := 0
		for _, object := range i.ownedObjects {
			activeObjectTokens += len(object.panicTokens)
		}
		i.funcMu.RUnlock()
		if tokenCount != 0 || activeObjectTokens != 0 {
			t.Fatalf("active panic tokens after churn %d = registry %d objects %d", iteration, tokenCount, activeObjectTokens)
		}
		if ownedCount != ownedBaseline || funcCount != funcBaseline {
			t.Fatalf("metadata after panic-token mutation churn %d = owned %d funcs %d, want %d/%d", iteration, ownedCount, funcCount, ownedBaseline, funcBaseline)
		}
	}
}

func TestGorneshOwnedObjectMetadataAPIPanicChurnIsBounded(t *testing.T) {
	i := New(Options{Stderr: io.Discard})
	if _, err := i.Eval(`
func apiOwnedPanicGornesh() { panic(map[string]int{"value": 1}) }
func apiNestedOwnedPanicGornesh() {
	pointer := new(int)
	panic(map[string]*int{"pointer": pointer})
}
func apiCyclicOwnedPanicGornesh() {
	value := map[string]interface{}{}
	value["self"] = value
	panic(value)
}
func apiReplacedOwnedPanicGornesh() {
	defer func() { panic(map[string]int{"second": 2}) }()
	panic(map[string]int{"first": 1})
}
func apiReplacedFuncPanicGornesh() {
	defer func() { panic(2) }()
	panic(func() {})
}
func apiLateMutatedPanicGornesh() {
	value := map[string]interface{}{"pointer": new(int)}
	defer func() {
		delete(value, "pointer")
		value["nested"] = map[string]interface{}{"pointer": new(int)}
		value["cycle"] = value
	}()
	panic(value)
}`); err != nil {
		t.Fatalf("define API panic churn: %v", err)
	}
	i.funcMu.RLock()
	ownedBaseline, funcBaseline := len(i.ownedObjects), len(i.funcMeta)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`apiOwnedPanicGornesh()`); err == nil {
			t.Fatalf("API owned panic %d returned nil error", iteration)
		}
		if _, err := i.Eval(`apiNestedOwnedPanicGornesh()`); err == nil {
			t.Fatalf("nested API owned panic %d returned nil error", iteration)
		}
		if _, err := i.Eval(`apiCyclicOwnedPanicGornesh()`); err == nil {
			t.Fatalf("cyclic API owned panic %d returned nil error", iteration)
		}
		if _, err := i.Eval(`apiLateMutatedPanicGornesh()`); err == nil {
			t.Fatalf("late-mutated API owned panic %d returned nil error", iteration)
		}
	}
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`apiReplacedOwnedPanicGornesh()`); err == nil {
			t.Fatalf("replaced API owned panic %d returned nil error", iteration)
		}
		if _, err := i.Eval(`apiReplacedFuncPanicGornesh()`); err == nil {
			t.Fatalf("replaced API func panic %d returned nil error", iteration)
		}
	}
	i.funcMu.RLock()
	ownedCount, funcCount := len(i.ownedObjects), len(i.funcMeta)
	i.funcMu.RUnlock()
	if ownedCount != ownedBaseline || funcCount != funcBaseline {
		t.Fatalf("metadata after API panic churn = owned %d funcs %d, want %d/%d", ownedCount, funcCount, ownedBaseline, funcBaseline)
	}
}

func TestGorneshCanceledOldRootAPIPanicCleanupUsesExecutionOwner(t *testing.T) {
	var entered chan struct{}
	var release chan struct{}
	i := New(Options{Stderr: io.Discard})
	if err := i.Use(Exports{"oldrootpanic/oldrootpanic": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "oldrootpanic"
func canceledOldRootAPIPanicGornesh() {
	defer func() { panic(map[string]*int{"pointer": new(int)}) }()
	oldrootpanic.Block()
}`); err != nil {
		t.Fatalf("define canceled old-root API panic: %v", err)
	}
	i.funcMu.RLock()
	baseline := len(i.ownedObjects)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		entered = make(chan struct{})
		release = make(chan struct{})
		cancelOwner := make(chan struct{})
		worker := make(chan error, 1)
		go func() {
			_, err := i.evalWithCancel(`canceledOldRootAPIPanicGornesh()`, "", true, cancelOwner)
			worker <- err
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			close(cancelOwner)
			t.Fatalf("old-root panic iteration %d did not block", iteration)
		}
		close(cancelOwner)
		if _, err := i.Eval(`0`); err != nil {
			t.Fatalf("detach during old-root panic iteration %d: %v", iteration, err)
		}
		close(release)
		select {
		case err := <-worker:
			if err == nil {
				t.Fatalf("old-root panic iteration %d returned nil error", iteration)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("old-root panic iteration %d did not finish", iteration)
		}
		i.funcMu.RLock()
		got := len(i.ownedObjects)
		i.funcMu.RUnlock()
		if got != baseline {
			t.Fatalf("owned metadata after old-root panic iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshOwnedPanicTokenSurvivesRepeatedDetachAndFinishesOnce(t *testing.T) {
	panicEntered := make(chan struct{})
	panicRelease := make(chan struct{})
	panicFinished := make(chan struct{})
	detachEntered := make(chan struct{})
	detachRelease := make(chan struct{})
	i := New(Options{Stderr: io.Discard})
	if err := i.Use(Exports{"repeatedpanicdetach/repeatedpanicdetach": {
		"BlockPanic":    reflect.ValueOf(func() { close(panicEntered); <-panicRelease }),
		"PanicFinished": reflect.ValueOf(func() { close(panicFinished) }),
		"BlockDetach":   reflect.ValueOf(func() { close(detachEntered); <-detachRelease }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "repeatedpanicdetach"
func repeatedDetachedOwnedPanicGornesh() {
	value := map[string]interface{}{"pointer": new(int), "callback": func() {}}
	defer repeatedpanicdetach.PanicFinished()
	defer func() {
		repeatedpanicdetach.BlockPanic()
		_ = recover()
	}()
	panic(value)
}`); err != nil {
		t.Fatalf("define repeated-detach panic: %v", err)
	}
	i.funcMu.RLock()
	ownedBaseline, funcBaseline := len(i.ownedObjects), len(i.funcMeta)
	i.funcMu.RUnlock()

	ctx, cancel := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `repeatedDetachedOwnedPanicGornesh()`)
		firstResult <- err
	}()
	select {
	case <-panicEntered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("panic defer did not block")
	}
	cancel()
	if err := <-firstResult; err == nil {
		t.Fatal("canceled panic Eval returned nil error")
	}
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("first panic-token detach: %v", err)
	}

	secondCtx, secondCancel := context.WithCancel(context.Background())
	secondResult := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(secondCtx, `repeatedpanicdetach.BlockDetach()`)
		secondResult <- err
	}()
	select {
	case <-detachEntered:
	case <-time.After(3 * time.Second):
		secondCancel()
		t.Fatal("second detach blocker did not start")
	}
	secondCancel()
	if err := <-secondResult; err == nil {
		t.Fatal("second canceled Eval returned nil error")
	}
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("second panic-token detach: %v", err)
	}
	close(detachRelease)
	close(panicRelease)
	select {
	case <-panicFinished:
	case <-time.After(3 * time.Second):
		t.Fatal("repeated-detach panic did not finish")
	}
	waitForFuncSweepGornesh(i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("sweep after repeated panic detach: %v", err)
	}
	i.funcMu.RLock()
	ownedCount, funcCount, tokenCount := len(i.ownedObjects), len(i.funcMeta), len(i.panicTokens)
	i.funcMu.RUnlock()
	if tokenCount != 0 || ownedCount != ownedBaseline || funcCount != funcBaseline {
		t.Fatalf("repeated-detach cleanup = tokens %d owned %d funcs %d, want 0/%d/%d", tokenCount, ownedCount, funcCount, ownedBaseline, funcBaseline)
	}
}

func TestGorneshOwnedObjectMetadataRootChurnIsBounded(t *testing.T) {
	i := New(Options{})
	i.funcMu.RLock()
	baseline := len(i.ownedObjects)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`map[string]int{"value": 1}`); err != nil {
			t.Fatalf("discard root map result %d: %v", iteration, err)
		}
	}
	i.funcMu.RLock()
	got := len(i.ownedObjects)
	i.funcMu.RUnlock()
	if got != baseline {
		t.Fatalf("owned metadata after discarded root results = %d, want baseline %d", got, baseline)
	}

	if _, err := i.Eval(`var detachedRootReplacementGornesh = map[string]int{"value": 0}`); err != nil {
		t.Fatalf("define root replacement map: %v", err)
	}
	i.Globals()
	i.funcMu.RLock()
	baseline = len(i.ownedObjects)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`detachedRootReplacementGornesh = map[string]int{"value": 1}; 0`); err != nil {
			t.Fatalf("replace root map %d: %v", iteration, err)
		}
	}
	i.funcMu.RLock()
	got = len(i.ownedObjects)
	i.funcMu.RUnlock()
	if got != baseline {
		t.Fatalf("owned metadata after root replacement = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshDetachedRootKeepsHostPublishedAggregateShared(t *testing.T) {
	i := New(Options{})
	value, err := i.Eval(`var detachedPublishedMapGornesh = map[string]int{"value": 1}; detachedPublishedMapGornesh`)
	if err != nil {
		t.Fatalf("publish owned map: %v", err)
	}
	hostMap, ok := value.Interface().(map[string]int)
	if !ok {
		t.Fatalf("published map type = %T", value.Interface())
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`detachedPublishedMapGornesh["value"] = 2`); err != nil {
		t.Fatalf("mutate published map after detach: %v", err)
	}
	if got := hostMap["value"]; got != 2 {
		t.Fatalf("host alias split across detached root: got %d want 2", got)
	}
}

func TestGorneshDetachedRootKeepsHostSharedGlobalCellIsland(t *testing.T) {
	var retained *int
	i := New(Options{})
	if err := i.Use(Exports{"detachedhost/detachedhost": {
		"RetainInt": reflect.ValueOf(func(value *int) { retained = value }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedhost"
var detachedSharedCellGornesh = 1
var detachedSharedCellPointerGornesh = &detachedSharedCellGornesh
detachedhost.RetainInt(detachedSharedCellPointerGornesh)`); err != nil {
		t.Fatalf("retain global cell pointer: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedSharedCellGornesh = 2
detachedSharedCellPointerGornesh == &detachedSharedCellGornesh`)
	if err != nil || value.Interface() != true {
		t.Fatalf("host-shared global cell identity: value=%v err=%v", value, err)
	}
	if retained == nil || *retained != 2 {
		t.Fatalf("retained host pointer did not observe global write: %v", retained)
	}
}

func TestGorneshDetachedRootSeedsSiblingPointersInSharedCellIsland(t *testing.T) {
	var retained *int
	i := New(Options{})
	if err := i.Use(Exports{"detachedhost/detachedhost": {
		"RetainInt": reflect.ValueOf(func(value *int) { retained = value }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedhost"
type detachedSharedPairGornesh struct { A, B int }
var detachedSharedPairValueGornesh = detachedSharedPairGornesh{A: 1, B: 2}
var detachedSharedPairAGornesh = &detachedSharedPairValueGornesh.A
var detachedSharedPairBGornesh = &detachedSharedPairValueGornesh.B
detachedhost.RetainInt(detachedSharedPairAGornesh)`); err != nil {
		t.Fatalf("retain shared cell field: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedSharedPairValueGornesh.A = 3
detachedSharedPairValueGornesh.B = 4
	detachedSharedPairBGornesh == &detachedSharedPairValueGornesh.B && *detachedSharedPairBGornesh == 4`)
	if err != nil || value.Interface() != true {
		t.Fatalf("shared cell sibling pointer identity: value=%v err=%v", value, err)
	}
	if retained == nil || *retained != 3 {
		t.Fatalf("retained shared field did not observe write: %v", retained)
	}
}

func TestGorneshDetachedRootKeepsHostSharedCapturedCell(t *testing.T) {
	var retained *int
	i := New(Options{})
	if err := i.Use(Exports{"detachedhost/detachedhost": {
		"RetainInt": reflect.ValueOf(func(value *int) { retained = value }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedhost"
var detachedSharedCapturedFuncGornesh func()
func installDetachedSharedCapturedCellGornesh() {
	value := 1
	detachedhost.RetainInt(&value)
	detachedSharedCapturedFuncGornesh = func() { value++ }
}
installDetachedSharedCapturedCellGornesh()`); err != nil {
		t.Fatalf("install host-shared captured cell: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`detachedSharedCapturedFuncGornesh()`); err != nil {
		t.Fatalf("call host-shared captured closure: %v", err)
	}
	if retained == nil || *retained != 2 {
		t.Fatalf("host-shared captured cell split: %v", retained)
	}
}

func TestGorneshDetachedRootPropagatesHostShareAcrossInteriorPointers(t *testing.T) {
	t.Run("container to interior", func(t *testing.T) {
		var retained reflect.Value
		i := New(Options{})
		if err := i.Use(Exports{"detachedhost/detachedhost": {
			"Retain": reflect.ValueOf(func(value interface{}) { retained = reflect.ValueOf(value) }),
		}}); err != nil {
			t.Fatal(err)
		}
		if _, err := i.Eval(`
import "detachedhost"
type detachedInteriorBoxGornesh struct { Value int }
var detachedInteriorBoxPointerGornesh = &detachedInteriorBoxGornesh{Value: 1}
var detachedInteriorFieldPointerGornesh = &detachedInteriorBoxPointerGornesh.Value
detachedhost.Retain(detachedInteriorBoxPointerGornesh)`); err != nil {
			t.Fatalf("retain owned container: %v", err)
		}
		forceDetachedRootCloneGornesh(t, i)
		if _, err := i.Eval(`*detachedInteriorFieldPointerGornesh = 2`); err != nil {
			t.Fatalf("mutate interior pointer: %v", err)
		}
		if got := retained.Elem().FieldByName("Value").Int(); got != 2 {
			t.Fatalf("host container alias split: got %d want 2", got)
		}
	})

	t.Run("interior to container", func(t *testing.T) {
		var retained *int
		i := New(Options{})
		if err := i.Use(Exports{"detachedhost/detachedhost": {
			"RetainInt": reflect.ValueOf(func(value *int) { retained = value }),
		}}); err != nil {
			t.Fatal(err)
		}
		if _, err := i.Eval(`
import "detachedhost"
type detachedReverseInteriorBoxGornesh struct { Value int }
var detachedReverseInteriorBoxPointerGornesh = &detachedReverseInteriorBoxGornesh{Value: 1}
var detachedReverseInteriorFieldPointerGornesh = &detachedReverseInteriorBoxPointerGornesh.Value
detachedhost.RetainInt(detachedReverseInteriorFieldPointerGornesh)`); err != nil {
			t.Fatalf("retain owned interior pointer: %v", err)
		}
		forceDetachedRootCloneGornesh(t, i)
		if _, err := i.Eval(`detachedReverseInteriorBoxPointerGornesh.Value = 2`); err != nil {
			t.Fatalf("mutate containing pointer: %v", err)
		}
		if retained == nil || *retained != 2 {
			t.Fatalf("host interior alias split: %v", retained)
		}
	})
}

func TestGorneshDetachedRootPropagatesHostShareAcrossAggregateEdges(t *testing.T) {
	var retained reflect.Value
	i := New(Options{})
	if err := i.Use(Exports{"detachedhost/detachedhost": {
		"Retain": reflect.ValueOf(func(value interface{}) { retained = reflect.ValueOf(value) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedhost"
type detachedEdgeBoxGornesh struct { Value int }
var detachedEdgePointerGornesh = &detachedEdgeBoxGornesh{Value: 1}
var detachedEdgeMapGornesh = map[string]*detachedEdgeBoxGornesh{"pointer": detachedEdgePointerGornesh}
detachedhost.Retain(detachedEdgeMapGornesh)`); err != nil {
		t.Fatalf("retain owned aggregate graph: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
detachedEdgePointerGornesh.Value = 2
detachedEdgeMapGornesh["pointer"] == detachedEdgePointerGornesh`)
	if err != nil || value.Interface() != true {
		t.Fatalf("host-shared aggregate edge identity: value=%v err=%v", value, err)
	}
	retainedPointer := retained.MapIndex(reflect.ValueOf("pointer"))
	if got := retainedPointer.Elem().FieldByName("Value").Int(); got != 2 {
		t.Fatalf("host-shared aggregate child split: got %d want 2", got)
	}
}

func TestGorneshDetachedRootDoesNotPublishNilNativeCallArguments(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
type detachedNilCallBoxGornesh struct { Value int }
var detachedNilCallPointerGornesh = &detachedNilCallBoxGornesh{Value: 1}
func recoverDetachedNilCallGornesh() {
	defer func() { recover() }()
	var call func(*detachedNilCallBoxGornesh)
	call(detachedNilCallPointerGornesh)
}
func recoverDetachedNilDeferredCallGornesh() {
	defer func() { recover() }()
	var call func(*detachedNilCallBoxGornesh)
	defer call(detachedNilCallPointerGornesh)
}
recoverDetachedNilCallGornesh()
recoverDetachedNilDeferredCallGornesh()
0`); err != nil {
		t.Fatalf("recover nil native calls: %v", err)
	}
	symbol := i.srcPkg["main"]["detachedNilCallPointerGornesh"]
	oldPointer := unwrapOwnedValue(i.frame.data[symbol.index]).Pointer()
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("detach after nil native calls: %v", err)
	}
	if got := unwrapOwnedValue(i.frame.data[symbol.index]).Pointer(); got == oldPointer {
		t.Fatalf("nil native call falsely published argument: pointer=%x", got)
	}
}

func TestGorneshDeferredPanicRunsRemainingDefersInGoOrder(t *testing.T) {
	i := New(Options{})
	value, err := i.Eval(`
var deferredPanicOrderGornesh int
func runDeferredPanicOrderGornesh() {
	defer func() {
		if recover() != nil { deferredPanicOrderGornesh = deferredPanicOrderGornesh*10 + 3 }
	}()
	defer func() { deferredPanicOrderGornesh = deferredPanicOrderGornesh*10 + 2 }()
	defer func() { panic("boom") }()
}
runDeferredPanicOrderGornesh()
deferredPanicOrderGornesh`)
	if err != nil || value.Interface() != 23 {
		t.Fatalf("deferred panic order: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootMarksBinaryMethodReceiverHostShared(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"detachedhost/detachedhost": {
		"Box":     reflect.ValueOf((*DetachedNativeMethodBoxGornesh)(nil)),
		"Entered": reflect.ValueOf(func() { close(entered) }),
		"Block":   reflect.ValueOf(func() { <-release }),
		"Done":    reflect.ValueOf(func() { close(finished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedhost"
var detachedMethodBoxGornesh = &detachedhost.Box{
	Entered: detachedhost.Entered,
	Block: detachedhost.Block,
	Done: detachedhost.Done,
}`); err != nil {
		t.Fatalf("define binary method receiver: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `detachedMethodBoxGornesh.BlockAndMutate()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("binary method did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled binary method Eval returned nil error")
	}
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("detach while binary receiver is blocked: %v", err)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("binary method did not finish")
	}
	value, err := i.Eval(`detachedMethodBoxGornesh.Value`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("binary receiver after detach: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootKeepsGlobalsPublicationShared(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedGlobalsScalarGornesh = 1
var detachedGlobalsMapGornesh = map[string]int{"value": 1}`); err != nil {
		t.Fatalf("define Globals values: %v", err)
	}
	globals := i.Globals()
	scalar := globals["detachedGlobalsScalarGornesh"]
	hostMap, ok := globals["detachedGlobalsMapGornesh"].Interface().(map[string]int)
	if !scalar.IsValid() || !ok {
		t.Fatalf("Globals publication missing values: scalar=%v map=%T", scalar, globals["detachedGlobalsMapGornesh"].Interface())
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`detachedGlobalsScalarGornesh = 2; detachedGlobalsMapGornesh["value"] = 2`); err != nil {
		t.Fatalf("mutate Globals values after detach: %v", err)
	}
	if scalar.Int() != 2 || hostMap["value"] != 2 {
		t.Fatalf("Globals aliases split: scalar=%d map=%d", scalar.Int(), hostMap["value"])
	}
}

func TestGorneshHostSharedMapWriteBarrierPreservesInsertedOwnedAlias(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedWriteBarrierMapGornesh = map[string]*int{}
var detachedWriteBarrierPointerGornesh *int`); err != nil {
		t.Fatalf("define host-shared write barrier values: %v", err)
	}
	hostMap := unwrapOwnedValue(i.Globals()["detachedWriteBarrierMapGornesh"]).Interface().(map[string]*int)
	if _, err := i.Eval(`
detachedWriteBarrierPointerGornesh = new(int)
*detachedWriteBarrierPointerGornesh = 1
detachedWriteBarrierMapGornesh["pointer"] = detachedWriteBarrierPointerGornesh`); err != nil {
		t.Fatalf("insert owned pointer into host-shared map: %v", err)
	}
	hostPointer := hostMap["pointer"]
	if hostPointer == nil || *hostPointer != 1 {
		t.Fatalf("host did not observe inserted pointer: %v", hostPointer)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*detachedWriteBarrierPointerGornesh = 2
detachedWriteBarrierPointerGornesh == detachedWriteBarrierMapGornesh["pointer"]`)
	if err != nil || value.Interface() != true {
		t.Fatalf("inserted pointer alias after detach: value=%v err=%v", value, err)
	}
	if *hostPointer != 2 {
		t.Fatalf("host inserted pointer alias split after detach: got %d want 2", *hostPointer)
	}
}

func TestGorneshHostSharedMapWriteBarrierPreservesInsertedOwnedKey(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var detachedWriteBarrierKeyMapGornesh = map[*int]int{}
var detachedWriteBarrierKeyPointerGornesh *int
func detachedWriteBarrierFirstKeyGornesh() *int {
	for key := range detachedWriteBarrierKeyMapGornesh { return key }
	return nil
}

0`); err != nil {
		t.Fatalf("define host-shared map key values: %v", err)
	}
	mapValue, err := i.Eval(`detachedWriteBarrierKeyMapGornesh`)
	if err != nil {
		t.Fatalf("publish map key destination: %v", err)
	}
	hostMap := unwrapOwnedValue(mapValue).Interface().(map[*int]int)
	if _, err := i.Eval(`
detachedWriteBarrierKeyPointerGornesh = new(int)
*detachedWriteBarrierKeyPointerGornesh = 1
detachedWriteBarrierKeyMapGornesh[detachedWriteBarrierKeyPointerGornesh] = 1
0`); err != nil {
		t.Fatalf("insert owned map key: %v", err)
	}
	var hostKey *int
	for key := range hostMap {
		hostKey = key
	}
	if hostKey == nil || *hostKey != 1 {
		t.Fatalf("host did not observe inserted map key: %v", hostKey)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*detachedWriteBarrierKeyPointerGornesh = 2
detachedWriteBarrierKeyPointerGornesh == detachedWriteBarrierFirstKeyGornesh()`)
	if err != nil || value.Interface() != true {
		t.Fatalf("inserted map key alias after detach: value=%v err=%v", value, err)
	}
	if *hostKey != 2 {
		t.Fatalf("host map key split after detach: got %d want 2", *hostKey)
	}
}

func TestGorneshNativeReturnedAggregatesPropagateHostSharedWrites(t *testing.T) {
	hostMap := map[string]*int{}
	hostSlice := make([]*int, 1)
	hostBox := &detachedNativeReturnBoxGornesh{}
	i := New(Options{})
	if err := i.Use(Exports{"nativeprovenance/nativeprovenance": {
		"ReturnMap":   reflect.ValueOf(func() map[string]*int { return hostMap }),
		"ReturnSlice": reflect.ValueOf(func() []*int { return hostSlice }),
		"ReturnBox":   reflect.ValueOf(func() *detachedNativeReturnBoxGornesh { return hostBox }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "nativeprovenance"
var nativeProvenanceMapGornesh = nativeprovenance.ReturnMap()
var nativeProvenanceSliceGornesh = nativeprovenance.ReturnSlice()
var nativeProvenanceBoxGornesh = nativeprovenance.ReturnBox()
var nativeProvenanceMapPointerGornesh *int
var nativeProvenanceSlicePointerGornesh *int
var nativeProvenanceBoxPointerGornesh *int
func installNativeProvenancePointersGornesh() {
	nativeProvenanceMapPointerGornesh = new(int)
	nativeProvenanceSlicePointerGornesh = new(int)
	nativeProvenanceBoxPointerGornesh = new(int)
	*nativeProvenanceMapPointerGornesh = 1
	*nativeProvenanceSlicePointerGornesh = 1
	*nativeProvenanceBoxPointerGornesh = 1
	nativeProvenanceMapGornesh["pointer"] = nativeProvenanceMapPointerGornesh
	nativeProvenanceSliceGornesh[0] = nativeProvenanceSlicePointerGornesh
	nativeProvenanceBoxGornesh.Pointer = nativeProvenanceBoxPointerGornesh
}
0`); err != nil {
		t.Fatalf("define native aggregate provenance: %v", err)
	}
	if _, err := i.Eval(`installNativeProvenancePointersGornesh(); 0`); err != nil {
		t.Fatalf("write owned pointers into native aggregates: %v", err)
	}
	mapPointer, slicePointer, boxPointer := hostMap["pointer"], hostSlice[0], hostBox.Pointer
	if mapPointer == nil || slicePointer == nil || boxPointer == nil {
		t.Fatalf("host did not observe inserted pointers: map=%v slice=%v box=%v", mapPointer, slicePointer, boxPointer)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*nativeProvenanceMapPointerGornesh = 2
*nativeProvenanceSlicePointerGornesh = 2
*nativeProvenanceBoxPointerGornesh = 2
nativeProvenanceMapPointerGornesh == nativeProvenanceMapGornesh["pointer"] &&
	nativeProvenanceSlicePointerGornesh == nativeProvenanceSliceGornesh[0] &&
	nativeProvenanceBoxPointerGornesh == nativeProvenanceBoxGornesh.Pointer`)
	if err != nil || value.Interface() != true {
		t.Fatalf("native aggregate inserted aliases after detach: value=%v err=%v", value, err)
	}
	if *mapPointer != 2 || *slicePointer != 2 || *boxPointer != 2 {
		t.Fatalf("native aggregate host aliases split: map=%d slice=%d box=%d", *mapPointer, *slicePointer, *boxPointer)
	}
}

func TestGorneshNativeResultPublicationPrecedesCanceledDiscard(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var retained *int
	i := New(Options{})
	if err := i.Use(Exports{"nativecancelresult/nativecancelresult": {
		"RoundTrip": reflect.ValueOf(func(callback func() *int) *int {
			retained = callback()
			close(entered)
			<-release
			return retained
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "nativecancelresult"
var NativeCanceledResultPointerGornesh *int
func createNativeCanceledResultPointerGornesh() *int {
	NativeCanceledResultPointerGornesh = new(int)
	return NativeCanceledResultPointerGornesh
}
func blockNativeCanceledResultGornesh() {
	nativecancelresult.RoundTrip(createNativeCanceledResultPointerGornesh)
}`); err != nil {
		t.Fatalf("define canceled native result: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `blockNativeCanceledResultGornesh()`)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled native result Eval returned nil error")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		i.funcMu.RLock()
		object := i.ownedObjectLocked(reflect.ValueOf(retained))
		published := object != nil && object.hostShared
		i.funcMu.RUnlock()
		if published {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native result was not published before canceled call discard")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*NativeCanceledResultPointerGornesh = 2
NativeCanceledResultPointerGornesh`)
	if err != nil || value.Interface().(*int) != retained || *retained != 2 {
		t.Fatalf("canceled native result alias split: value=%v retained=%v err=%v", value, retained, err)
	}
}

func TestGorneshRangeAssignmentsPropagateHostSharedCells(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var RangeBarrierChannelValueGornesh *int
var RangeBarrierMapKeyGornesh *int
var RangeBarrierMapValueGornesh *int
var RangeBarrierSliceValueGornesh *int
var RangeBarrierSliceIndexGornesh int
var rangeBarrierChannelPointerGornesh *int
var rangeBarrierMapKeyPointerGornesh *int
var rangeBarrierMapValuePointerGornesh *int
var rangeBarrierSlicePointerGornesh *int
func installRangeBarrierChannelGornesh() {
	rangeBarrierChannelPointerGornesh = new(int)
	channel := make(chan *int, 1)
	channel <- rangeBarrierChannelPointerGornesh
	close(channel)
	for RangeBarrierChannelValueGornesh = range channel {}
}

func installRangeBarrierMapGornesh() {
	rangeBarrierMapKeyPointerGornesh = new(int)
	rangeBarrierMapValuePointerGornesh = new(int)
	for RangeBarrierMapKeyGornesh, RangeBarrierMapValueGornesh = range map[*int]*int{
		rangeBarrierMapKeyPointerGornesh: rangeBarrierMapValuePointerGornesh,
	} {}
}
func installRangeBarrierSliceGornesh() {
	rangeBarrierSlicePointerGornesh = new(int)
	for RangeBarrierSliceIndexGornesh, RangeBarrierSliceValueGornesh = range []*int{rangeBarrierSlicePointerGornesh} {
		_ = RangeBarrierSliceValueGornesh
	}
}
0`); err != nil {
		t.Fatalf("define range write-barrier values: %v", err)
	}
	symbols := i.Symbols("main")["main"]
	channelCell := symbols["RangeBarrierChannelValueGornesh"]
	mapKeyCell := symbols["RangeBarrierMapKeyGornesh"]
	mapValueCell := symbols["RangeBarrierMapValueGornesh"]
	sliceCell := symbols["RangeBarrierSliceValueGornesh"]
	if _, err := i.Eval(`installRangeBarrierChannelGornesh(); installRangeBarrierMapGornesh(); installRangeBarrierSliceGornesh(); 0`); err != nil {
		t.Fatalf("range owned pointers into host-shared cells: %v", err)
	}
	current, currentErr := i.Eval(`RangeBarrierChannelValueGornesh != nil && RangeBarrierMapKeyGornesh != nil && RangeBarrierMapValueGornesh != nil && RangeBarrierSliceValueGornesh != nil`)
	if currentErr != nil || current.Interface() != true {
		individual, _ := i.Eval(`[]bool{RangeBarrierChannelValueGornesh != nil, RangeBarrierMapKeyGornesh != nil, RangeBarrierMapValueGornesh != nil, RangeBarrierSliceValueGornesh != nil}`)
		t.Fatalf("range assignments did not update current globals: value=%v individual=%v err=%v", current, individual, currentErr)
	}
	hostChannelPointer := unwrapOwnedValue(channelCell).Interface().(*int)
	hostMapKeyPointer := unwrapOwnedValue(mapKeyCell).Interface().(*int)
	hostMapValuePointer := unwrapOwnedValue(mapValueCell).Interface().(*int)
	hostSlicePointer := unwrapOwnedValue(sliceCell).Interface().(*int)
	if hostChannelPointer == nil || hostMapKeyPointer == nil || hostMapValuePointer == nil || hostSlicePointer == nil {
		t.Fatalf("range assignments were not published to host cells: channel=%v mapKey=%v mapValue=%v slice=%v", hostChannelPointer, hostMapKeyPointer, hostMapValuePointer, hostSlicePointer)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*rangeBarrierChannelPointerGornesh = 2
*rangeBarrierMapKeyPointerGornesh = 2
*rangeBarrierMapValuePointerGornesh = 2
*rangeBarrierSlicePointerGornesh = 2
rangeBarrierChannelPointerGornesh == RangeBarrierChannelValueGornesh &&
	rangeBarrierMapKeyPointerGornesh == RangeBarrierMapKeyGornesh &&
	rangeBarrierMapValuePointerGornesh == RangeBarrierMapValueGornesh &&
	rangeBarrierSlicePointerGornesh == RangeBarrierSliceValueGornesh`)
	if err != nil || value.Interface() != true {
		t.Fatalf("range aliases after detach: value=%v err=%v", value, err)
	}
	if *hostChannelPointer != 2 || *hostMapKeyPointer != 2 || *hostMapValuePointer != 2 || *hostSlicePointer != 2 {
		t.Fatalf("range host aliases split: channel=%d mapKey=%d mapValue=%d slice=%d", *hostChannelPointer, *hostMapKeyPointer, *hostMapValuePointer, *hostSlicePointer)
	}
}

func TestGorneshRangeExpressionTargetsPropagateHostSharedWrites(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
type RangeExpressionBoxGornesh struct { Pointer *int }
var RangeExpressionKeysGornesh [1]int
var RangeExpressionValuesGornesh [1]*int
var RangeExpressionBoxValueGornesh RangeExpressionBoxGornesh
var RangeExpressionDerefValueGornesh *int
var RangeExpressionMapGornesh = map[string]*int{}
var RangeExpressionOrderIndexGornesh int
var RangeExpressionOrderValuesGornesh [2]int
var RangeExpressionOrderMapGornesh = map[int]int{}
var rangeExpressionArrayPointerGornesh *int
var rangeExpressionFieldPointerGornesh *int
var rangeExpressionDerefPointerGornesh *int
var rangeExpressionDerefSlotGornesh = &RangeExpressionDerefValueGornesh
var rangeExpressionMapPointerGornesh *int
func installRangeExpressionArrayGornesh() {
	rangeExpressionArrayPointerGornesh = new(int)
	for RangeExpressionKeysGornesh[0], RangeExpressionValuesGornesh[0] = range map[int]*int{3: rangeExpressionArrayPointerGornesh} { break }
}
func installRangeExpressionFieldGornesh() {
	rangeExpressionFieldPointerGornesh = new(int)
	ch := make(chan *int, 1); ch <- rangeExpressionFieldPointerGornesh; close(ch)
	for RangeExpressionBoxValueGornesh.Pointer = range ch {}
}
func installRangeExpressionDerefGornesh() {
	rangeExpressionDerefPointerGornesh = new(int)
	ch := make(chan *int, 1); ch <- rangeExpressionDerefPointerGornesh; close(ch)
	for *rangeExpressionDerefSlotGornesh = range ch {}
}
func installRangeExpressionMapGornesh() {
	rangeExpressionMapPointerGornesh = new(int)
	ch := make(chan *int, 1); ch <- rangeExpressionMapPointerGornesh; close(ch)
	for RangeExpressionMapGornesh["pointer"] = range ch {}
}
func installRangeExpressionOrderGornesh() {
	for RangeExpressionOrderIndexGornesh, RangeExpressionOrderValuesGornesh[RangeExpressionOrderIndexGornesh] = range []int{7, 8} {
		_ = RangeExpressionOrderIndexGornesh
	}
}
func installRangeExpressionMapOrderGornesh() {
	RangeExpressionOrderIndexGornesh = 0
	for RangeExpressionOrderIndexGornesh, RangeExpressionOrderMapGornesh[RangeExpressionOrderIndexGornesh] = range []int{7, 8} {
		_ = RangeExpressionOrderIndexGornesh
	}
}
0`); err != nil {
		t.Fatalf("define range expression targets: %v", err)
	}
	_ = i.Symbols("main")
	for _, source := range []string{
		`installRangeExpressionArrayGornesh(); 0`,
		`installRangeExpressionFieldGornesh(); 0`,
		`installRangeExpressionDerefGornesh(); 0`,
		`installRangeExpressionMapGornesh(); 0`,
		`installRangeExpressionOrderGornesh(); 0`,
		`installRangeExpressionMapOrderGornesh(); 0`,
	} {
		if _, err := i.Eval(source); err != nil {
			t.Fatalf("range expression target %q: %v", source, err)
		}
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*rangeExpressionArrayPointerGornesh = 2
*rangeExpressionFieldPointerGornesh = 2
*rangeExpressionDerefPointerGornesh = 2
*rangeExpressionMapPointerGornesh = 2
RangeExpressionKeysGornesh[0] == 3 &&
	RangeExpressionValuesGornesh[0] == rangeExpressionArrayPointerGornesh &&
	RangeExpressionBoxValueGornesh.Pointer == rangeExpressionFieldPointerGornesh &&
	RangeExpressionDerefValueGornesh == rangeExpressionDerefPointerGornesh &&
	RangeExpressionMapGornesh["pointer"] == rangeExpressionMapPointerGornesh &&
	RangeExpressionOrderIndexGornesh == 1 &&
	RangeExpressionOrderValuesGornesh[0] == 8 && RangeExpressionOrderValuesGornesh[1] == 0 &&
	RangeExpressionOrderMapGornesh[0] == 8 && RangeExpressionOrderMapGornesh[1] == 0`)
	if err != nil || value.Interface() != true {
		individual, _ := i.Eval(`[]bool{
RangeExpressionKeysGornesh[0] == 3,
RangeExpressionValuesGornesh[0] == rangeExpressionArrayPointerGornesh,
RangeExpressionBoxValueGornesh.Pointer == rangeExpressionFieldPointerGornesh,
RangeExpressionDerefValueGornesh == rangeExpressionDerefPointerGornesh,
RangeExpressionMapGornesh["pointer"] == rangeExpressionMapPointerGornesh,
RangeExpressionOrderIndexGornesh == 1,
RangeExpressionOrderValuesGornesh[0] == 8,
RangeExpressionOrderValuesGornesh[1] == 0,
RangeExpressionOrderMapGornesh[0] == 8,
RangeExpressionOrderMapGornesh[1] == 0,
}`)
		t.Fatalf("range expression target aliases after detach: value=%v individual=%v err=%v", value, individual, err)
	}
}

func TestGorneshNonemptyInterfaceCompositeOutputPropagatesHostSharedCell(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
type CompositeOutputInterfaceGornesh interface { Method() }
type CompositeOutputValueGornesh struct { Pointer *int }
func (CompositeOutputValueGornesh) Method() {}
var CompositeOutputInterfaceValueGornesh CompositeOutputInterfaceGornesh
var compositeOutputPointerGornesh *int
func installCompositeOutputInterfaceGornesh() {
	compositeOutputPointerGornesh = new(int)
	CompositeOutputInterfaceValueGornesh = CompositeOutputValueGornesh{Pointer: compositeOutputPointerGornesh}
}
0`); err != nil {
		t.Fatalf("define nonempty interface composite output: %v", err)
	}
	_ = i.Symbols("main")
	if _, err := i.Eval(`installCompositeOutputInterfaceGornesh(); 0`); err != nil {
		t.Fatalf("install nonempty interface composite output: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`CompositeOutputInterfaceValueGornesh.(CompositeOutputValueGornesh).Pointer == compositeOutputPointerGornesh`)
	if err != nil || value.Interface() != true {
		t.Fatalf("nonempty interface composite alias after detach: value=%v err=%v", value, err)
	}
}

func TestGorneshDirectResultStoresPropagateHostSharedCells(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var DirectStoreMapGornesh *int
var DirectStoreAssertGornesh *int
var DirectStoreRecvGornesh *int
var DirectStoreRecv2Gornesh *int
var DirectStoreSelectGornesh *int
var DirectStoreAssertFailureGornesh = "old"
var directStoreMapPointerGornesh *int
var directStoreAssertPointerGornesh *int
var directStoreRecvPointerGornesh *int
var directStoreRecv2PointerGornesh *int
var directStoreSelectPointerGornesh *int
func installDirectStoreMapGornesh() {
	directStoreMapPointerGornesh = new(int)
	m := map[string]*int{"value": directStoreMapPointerGornesh}
	DirectStoreMapGornesh, _ = m["value"]
}
func installDirectStoreAssertGornesh() {
	directStoreAssertPointerGornesh = new(int)
	var asserted interface{} = directStoreAssertPointerGornesh
	DirectStoreAssertGornesh, _ = asserted.(*int)
}
func installDirectStoreRecvGornesh() {
	directStoreRecvPointerGornesh = new(int)
	ch := make(chan *int, 1)
	ch <- directStoreRecvPointerGornesh
	DirectStoreRecvGornesh = <-ch
}
func installDirectStoreRecv2Gornesh() {
	directStoreRecv2PointerGornesh = new(int)
	ch := make(chan *int, 1)
	ch <- directStoreRecv2PointerGornesh
	DirectStoreRecv2Gornesh, _ = <-ch
}
func installDirectStoreSelectGornesh() bool {
	directStoreSelectPointerGornesh = new(int)
	ch := make(chan *int, 1)
	ch <- directStoreSelectPointerGornesh
	selectOK := false
	select {
	case DirectStoreSelectGornesh, selectOK = <-ch:
	default:
		return false
	}
	return selectOK
}
func zeroDirectStoreValuesGornesh() bool {
	m := map[string]*int{}
	okMap := false
	DirectStoreMapGornesh, okMap = m["missing"]
	var asserted interface{} = directStoreAssertPointerGornesh
	okAssert := false
	DirectStoreAssertFailureGornesh, okAssert = asserted.(string)
	return DirectStoreMapGornesh == nil && !okMap && DirectStoreAssertFailureGornesh == "" && !okAssert
}
func restoreDirectStoreValuesGornesh() {
	DirectStoreMapGornesh = directStoreMapPointerGornesh
	DirectStoreAssertGornesh = directStoreAssertPointerGornesh
}
0`); err != nil {
		t.Fatalf("define direct result stores: %v", err)
	}
	symbols := i.Symbols("main")["main"]
	cells := []reflect.Value{
		symbols["DirectStoreMapGornesh"],
		symbols["DirectStoreAssertGornesh"],
		symbols["DirectStoreRecvGornesh"],
		symbols["DirectStoreRecv2Gornesh"],
		symbols["DirectStoreSelectGornesh"],
	}
	for _, source := range []string{
		`installDirectStoreMapGornesh(); 0`,
		`installDirectStoreAssertGornesh(); 0`,
		`installDirectStoreRecvGornesh(); 0`,
		`installDirectStoreRecv2Gornesh(); 0`,
		`installDirectStoreSelectGornesh()`,
	} {
		value, err := i.Eval(source)
		if err != nil || source == `installDirectStoreSelectGornesh()` && value.Interface() != true {
			t.Fatalf("direct result store %q: value=%v err=%v", source, value, err)
		}
	}
	value, err := i.Eval(`zeroDirectStoreValuesGornesh()`)
	if err != nil || value.Interface() != true {
		t.Fatalf("direct result zero semantics: value=%v err=%v", value, err)
	}
	if _, err := i.Eval(`restoreDirectStoreValuesGornesh(); 0`); err != nil {
		t.Fatalf("restore direct result stores: %v", err)
	}
	hostPointers := make([]*int, len(cells))
	for index, cell := range cells {
		hostPointers[index] = unwrapOwnedValue(cell).Interface().(*int)
		if hostPointers[index] == nil {
			t.Fatalf("direct result store %d did not update host cell", index)
		}
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`
*directStoreMapPointerGornesh = 2
*directStoreAssertPointerGornesh = 2
*directStoreRecvPointerGornesh = 2
*directStoreRecv2PointerGornesh = 2
*directStoreSelectPointerGornesh = 2
DirectStoreMapGornesh == directStoreMapPointerGornesh &&
	DirectStoreAssertGornesh == directStoreAssertPointerGornesh &&
	DirectStoreRecvGornesh == directStoreRecvPointerGornesh &&
	DirectStoreRecv2Gornesh == directStoreRecv2PointerGornesh &&
	DirectStoreSelectGornesh == directStoreSelectPointerGornesh`)
	if err != nil || value.Interface() != true {
		t.Fatalf("direct result store aliases after detach: value=%v err=%v", value, err)
	}
	for index, pointer := range hostPointers {
		if *pointer != 2 {
			t.Fatalf("direct result store host alias %d split: got %d want 2", index, *pointer)
		}
	}
}

func TestGorneshDirectAllocatorOutputsPropagateHostSharedCells(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
type DirectAllocatorBoxGornesh struct { Pointer *int }
var DirectAllocatorNewGornesh *int
var DirectAllocatorMapGornesh map[string]*int
var DirectAllocatorSliceGornesh []*int
var DirectAllocatorLiteralGornesh []*int
var DirectAllocatorAppendGornesh []*int
var DirectAllocatorConvertGornesh []byte
var DirectAllocatorStructGornesh DirectAllocatorBoxGornesh
var directAllocatorNewAliasGornesh *int
var directAllocatorMapAliasGornesh map[string]*int
var directAllocatorSliceAliasGornesh []*int
var directAllocatorLiteralAliasGornesh []*int
var directAllocatorAppendAliasGornesh []*int
var directAllocatorConvertAliasGornesh []byte
var directAllocatorStructAliasGornesh *int
func installDirectAllocatorValuesGornesh() {
	DirectAllocatorNewGornesh = new(int)
	directAllocatorNewAliasGornesh = DirectAllocatorNewGornesh
	DirectAllocatorMapGornesh = make(map[string]*int)
	directAllocatorMapAliasGornesh = DirectAllocatorMapGornesh
	DirectAllocatorSliceGornesh = make([]*int, 1)
	directAllocatorSliceAliasGornesh = DirectAllocatorSliceGornesh
	pLiteral := new(int)
	DirectAllocatorLiteralGornesh = []*int{pLiteral}
	directAllocatorLiteralAliasGornesh = DirectAllocatorLiteralGornesh
	pAppend := new(int)
	DirectAllocatorAppendGornesh = append([]*int{}, pAppend)
	directAllocatorAppendAliasGornesh = DirectAllocatorAppendGornesh
	DirectAllocatorConvertGornesh = []byte("a")
	directAllocatorConvertAliasGornesh = DirectAllocatorConvertGornesh
	pStruct := new(int)
	DirectAllocatorStructGornesh = DirectAllocatorBoxGornesh{Pointer: pStruct}
	directAllocatorStructAliasGornesh = pStruct
}
0`); err != nil {
		t.Fatalf("define direct allocator outputs: %v", err)
	}
	_ = i.Symbols("main")
	if _, err := i.Eval(`installDirectAllocatorValuesGornesh(); 0`); err != nil {
		t.Fatalf("install direct allocator outputs: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*DirectAllocatorNewGornesh = 2
pMap := new(int)
DirectAllocatorMapGornesh["pointer"] = pMap
pSlice := new(int)
DirectAllocatorSliceGornesh[0] = pSlice
*DirectAllocatorLiteralGornesh[0] = 2
*DirectAllocatorAppendGornesh[0] = 2
DirectAllocatorConvertGornesh[0] = 'b'
*DirectAllocatorStructGornesh.Pointer = 2
directAllocatorNewAliasGornesh == DirectAllocatorNewGornesh &&
	directAllocatorMapAliasGornesh["pointer"] == pMap &&
	directAllocatorSliceAliasGornesh[0] == pSlice &&
	*directAllocatorLiteralAliasGornesh[0] == 2 &&
	*directAllocatorAppendAliasGornesh[0] == 2 &&
	directAllocatorConvertAliasGornesh[0] == 'b' &&
	directAllocatorStructAliasGornesh == DirectAllocatorStructGornesh.Pointer &&
	*directAllocatorStructAliasGornesh == 2`)
	if err != nil || value.Interface() != true {
		t.Fatalf("direct allocator output aliases after detach: value=%v err=%v", value, err)
	}
}

func TestGorneshDetachedRootKeepsSymbolsPublicationShared(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var DetachedSymbolsScalarGornesh = 1
var DetachedSymbolsMapGornesh = map[string]int{"value": 1}
func DetachedSymbolsCallbackGornesh() { DetachedSymbolsScalarGornesh++ }`); err != nil {
		t.Fatalf("define Symbols values: %v", err)
	}
	exports := i.Symbols("main")
	syms := exports["main"]
	if syms == nil {
		t.Fatalf("Symbols main package missing: keys=%v", reflect.ValueOf(exports).MapKeys())
	}
	scalar := syms["DetachedSymbolsScalarGornesh"]
	hostMap, ok := syms["DetachedSymbolsMapGornesh"].Interface().(map[string]int)
	callback := syms["DetachedSymbolsCallbackGornesh"]
	if !scalar.IsValid() || !callback.IsValid() || !ok {
		t.Fatalf("Symbols publication missing values")
	}
	if err := i.Use(Exports{"detachedpublished/detachedpublished": {
		"Callback": callback,
		"Call":     reflect.ValueOf(func(fn func()) { fn() }),
	}}); err != nil {
		t.Fatal(err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
import "detachedpublished"
DetachedSymbolsScalarGornesh = 100
DetachedSymbolsMapGornesh["value"] = 2
detachedpublished.Call(detachedpublished.Callback)
DetachedSymbolsScalarGornesh`)
	if err != nil || value.Interface() != 101 {
		t.Fatalf("Symbols callback round trip: value=%v err=%v", value, err)
	}
	if scalar.Int() != 101 || hostMap["value"] != 2 {
		t.Fatalf("Symbols aliases split: scalar=%d map=%d", scalar.Int(), hostMap["value"])
	}
}

func TestGorneshDetachedRootIsolatesCanceledDeferredAggregateWrites(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	deferredFinished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"detachedclone/detachedclone": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"DeferredFinished": reflect.ValueOf(func() { close(deferredFinished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "detachedclone"
var detachedDeferredMapGornesh = map[string]int{"value": 1}
var detachedDeferredSliceGornesh = []int{1}
var detachedDeferredValueGornesh = 1
var detachedDeferredPointerGornesh = &detachedDeferredValueGornesh
func detachedDeferredMutationGornesh() {
	defer detachedclone.DeferredFinished()
	defer func() {
		detachedDeferredMapGornesh["value"] = 7
		detachedDeferredSliceGornesh[0] = 7
		*detachedDeferredPointerGornesh = 7
	}()
	detachedclone.Block()
}`); err != nil {
		t.Fatalf("define canceled deferred mutation: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `detachedDeferredMutationGornesh()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("canceled deferred mutation did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled deferred mutation returned nil error")
	}
	value, err := i.Eval(`
detachedDeferredMapGornesh["value"] = 99
detachedDeferredSliceGornesh[0] = 99
*detachedDeferredPointerGornesh = 99
detachedDeferredMapGornesh["value"] + detachedDeferredSliceGornesh[0] + *detachedDeferredPointerGornesh`)
	if err != nil || value.Interface() != 297 {
		t.Fatalf("write new detached graph: value=%v err=%v", value, err)
	}
	close(release)
	select {
	case <-deferredFinished:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled deferred mutation did not finish")
	}
	waitForFuncSweepGornesh(i)
	value, err = i.Eval(`detachedDeferredMapGornesh["value"] + detachedDeferredSliceGornesh[0] + *detachedDeferredPointerGornesh`)
	if err != nil || value.Interface() != 297 {
		t.Fatalf("old deferred write reached new graph: value=%v err=%v", value, err)
	}
}

func TestGorneshHostSharedSliceAppendWriteBarrierDistinguishesReallocation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"appendbarrier/appendbarrier": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"Finished": reflect.ValueOf(func() { close(finished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "appendbarrier"
var appendBarrierReuseGornesh = make([]*int, 0, 2)
var appendBarrierGrowthGornesh = make([]*int, 1, 1)
var appendBarrierReusePointerGornesh *int
var appendBarrierGrowthPointerGornesh *int
func appendBarrierBlockedWriteGornesh() {
	defer appendbarrier.Finished()
	defer func() { *appendBarrierGrowthPointerGornesh = 7 }()
	appendbarrier.Block()
}`); err != nil {
		t.Fatalf("define append write-barrier values: %v", err)
	}
	reuseValue, err := i.Eval(`appendBarrierReuseGornesh`)
	if err != nil {
		t.Fatalf("publish reusable append slice: %v", err)
	}
	growthValue, err := i.Eval(`appendBarrierGrowthGornesh`)
	if err != nil {
		t.Fatalf("publish growing append slice: %v", err)
	}
	hostReuse := unwrapOwnedValue(reuseValue).Interface().([]*int)
	hostGrowth := unwrapOwnedValue(growthValue).Interface().([]*int)
	if _, err := i.Eval(`
appendBarrierReusePointerGornesh = new(int)
*appendBarrierReusePointerGornesh = 1
appendBarrierReuseGornesh = append(appendBarrierReuseGornesh, appendBarrierReusePointerGornesh)
appendBarrierGrowthPointerGornesh = new(int)
*appendBarrierGrowthPointerGornesh = 1
appendBarrierGrowthGornesh = append(appendBarrierGrowthGornesh, appendBarrierGrowthPointerGornesh)
0`); err != nil {
		t.Fatalf("append owned pointers: %v", err)
	}
	hostReusePointer := hostReuse[:1][0]
	if hostReusePointer == nil || *hostReusePointer != 1 {
		t.Fatalf("host did not observe no-reallocation append: %v", hostReusePointer)
	}
	if hostGrowth[0] != nil {
		t.Fatalf("reallocating append mutated old host backing: %v", hostGrowth[0])
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `appendBarrierBlockedWriteGornesh()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("append write-barrier blocker was not entered")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled append write-barrier Eval returned nil error")
	}
	value, err := i.Eval(`
*appendBarrierReusePointerGornesh = 99
*appendBarrierGrowthPointerGornesh = 99
appendBarrierReusePointerGornesh == appendBarrierReuseGornesh[0] &&
	appendBarrierGrowthPointerGornesh == appendBarrierGrowthGornesh[1]`)
	if err != nil || value.Interface() != true {
		t.Fatalf("append pointer aliases after detach: value=%v err=%v", value, err)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("append write-barrier blocker did not finish")
	}
	waitForFuncSweepGornesh(i)
	value, err = i.Eval(`*appendBarrierGrowthPointerGornesh`)
	if err != nil || value.Interface() != 99 {
		t.Fatalf("reallocated append pointer was unnecessarily shared: value=%v err=%v", value, err)
	}
	if *hostReusePointer != 99 {
		t.Fatalf("no-reallocation appended pointer split from host: got %d want 99", *hostReusePointer)
	}
}

func TestGorneshHostSharedSliceCopyWriteBarrierPreservesInsertedAliases(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var copyBarrierDestinationGornesh = make([]*int, 1)
var copyBarrierDeferredDestinationGornesh = make([]*int, 1)
var copyBarrierSourceGornesh = make([]*int, 1)
var copyBarrierPointerGornesh *int
func copyBarrierDeferredGornesh() { defer copy(copyBarrierDeferredDestinationGornesh, copyBarrierSourceGornesh) }`); err != nil {
		t.Fatalf("define copy write-barrier values: %v", err)
	}
	destinationValue, err := i.Eval(`copyBarrierDestinationGornesh`)
	if err != nil {
		t.Fatalf("publish copy destination: %v", err)
	}
	deferredDestinationValue, err := i.Eval(`copyBarrierDeferredDestinationGornesh`)
	if err != nil {
		t.Fatalf("publish deferred copy destination: %v", err)
	}
	hostDestination := unwrapOwnedValue(destinationValue).Interface().([]*int)
	hostDeferredDestination := unwrapOwnedValue(deferredDestinationValue).Interface().([]*int)
	if _, err := i.Eval(`
copyBarrierPointerGornesh = new(int)
*copyBarrierPointerGornesh = 1
copyBarrierSourceGornesh[0] = copyBarrierPointerGornesh
copy(copyBarrierDestinationGornesh, copyBarrierSourceGornesh)
copyBarrierDeferredGornesh()`); err != nil {
		t.Fatalf("copy owned pointer into host-shared slices: %v", err)
	}
	if hostDestination[0] == nil || hostDeferredDestination[0] == nil {
		t.Fatalf("host did not observe copied pointers: direct=%v deferred=%v", hostDestination[0], hostDeferredDestination[0])
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*copyBarrierPointerGornesh = 2
copyBarrierPointerGornesh == copyBarrierDestinationGornesh[0] &&
	copyBarrierPointerGornesh == copyBarrierDeferredDestinationGornesh[0]`)
	if err != nil || value.Interface() != true {
		t.Fatalf("copied pointer aliases after detach: value=%v err=%v", value, err)
	}
	if *hostDestination[0] != 2 || *hostDeferredDestination[0] != 2 {
		t.Fatalf("copied host aliases split: direct=%d deferred=%d", *hostDestination[0], *hostDeferredDestination[0])
	}
}

func TestGorneshOwnedChannelReleaseHandlesCyclicRootGraphs(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var channelReleaseCycleMapGornesh = map[string]interface{}{}
func releaseLocalOwnedChannelGornesh() {
	ch := make(chan int, 1)
	ch <- 1
}
channelReleaseCycleMapGornesh["self"] = channelReleaseCycleMapGornesh
0`); err != nil {
		t.Fatalf("define cyclic root graphs: %v", err)
	}
	i.funcMu.RLock()
	baseline := len(i.ownedChannels)
	i.funcMu.RUnlock()
	done := make(chan error, 1)
	go func() {
		_, err := i.Eval(`
for iteration := 0; iteration < 20; iteration++ {
	releaseLocalOwnedChannelGornesh()
}
0`)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("release local channels with cyclic roots: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("release local channels with cyclic roots did not terminate")
	}
	i.funcMu.RLock()
	got := len(i.ownedChannels)
	i.funcMu.RUnlock()
	if got != baseline {
		t.Fatalf("owned channels after cyclic-root churn = %d, want baseline %d", got, baseline)
	}

	mapCycle := map[string]interface{}{}
	mapCycle["self"] = mapCycle
	pointerCycle := &detachedChannelCycleNodeGornesh{}
	pointerCycle.Next = pointerCycle
	sliceCycle := []interface{}{nil}
	sliceCycle[0] = sliceCycle
	i.registerOwnedValue(reflect.ValueOf(mapCycle), i.frame)
	i.registerOwnedValue(reflect.ValueOf(pointerCycle), i.frame)
	i.registerOwnedValue(reflect.ValueOf(sliceCycle), i.frame)
	target := reflect.MakeChan(reflect.TypeOf(make(chan int)), 1)
	i.funcMu.RLock()
	for name, value := range map[string]reflect.Value{
		"map":     reflect.ValueOf(mapCycle),
		"pointer": reflect.ValueOf(pointerCycle),
		"slice":   reflect.ValueOf(sliceCycle),
	} {
		if i.ownedValueContainsChannelLocked(value, target.Pointer(), map[*ownedObject]struct{}{}) {
			t.Fatalf("%s cycle unexpectedly contains target channel", name)
		}
	}
	i.funcMu.RUnlock()
}

func TestGorneshOwnedChannelReceiveMatchesDeliveredToken(t *testing.T) {
	i := New(Options{})
	root := i.frame
	channel := reflect.MakeChan(reflect.ChanOf(reflect.BothDir, reflect.TypeOf(detachedChannelOrderPayloadGornesh{})), 1)
	i.registerOwnedChannel(channel, root)
	mapA := reflect.MakeMap(reflect.TypeOf(map[string]int{}))
	mapB := reflect.MakeMap(reflect.TypeOf(map[string]int{}))
	i.registerOwnedValue(mapA, root)
	i.registerOwnedValue(mapB, root)
	callbackA := reflect.ValueOf(func() {})
	callbackB := reflect.ValueOf(func() {})
	keyA, _ := funcvalKeyOf(callbackA)
	keyB, _ := funcvalKeyOf(callbackB)
	groupA := &funcMetaGroup{root: root, version: 1}
	groupB := &funcMetaGroup{root: root, version: 1}
	i.funcMu.Lock()
	i.funcMeta[keyA] = interpretedFuncMeta{frame: root, group: groupA}
	i.funcMeta[keyB] = interpretedFuncMeta{frame: root, group: groupB}
	root.funcMeta = append(root.funcMeta, keyA, keyB)
	i.funcMu.Unlock()
	payloadA := reflect.ValueOf(detachedChannelOrderPayloadGornesh{Value: mapA.Interface().(map[string]int), Callback: callbackA.Interface().(func())})
	payloadB := reflect.ValueOf(detachedChannelOrderPayloadGornesh{Value: mapB.Interface().(map[string]int), Callback: callbackB.Interface().(func())})

	i.funcSweepMu.RLock()
	sendA := i.markInterpretedFuncChannelSend(root, channel, payloadA)
	sendB := i.markInterpretedFuncChannelSend(root, channel, payloadB)
	channel.Send(payloadB)
	delivered, _ := channel.Recv()
	i.adoptInterpretedFuncValue(root, channel, delivered)
	i.funcSweepMu.RUnlock()
	if sendA == nil || sendB == nil {
		t.Fatal("channel sends were not tracked")
	}
	i.markOwnedValuesHostShared(channel)

	i.funcMu.RLock()
	objectA := i.ownedObjectLocked(mapA)
	objectB := i.ownedObjectLocked(mapB)
	metaA := i.funcMeta[keyA]
	metaB := i.funcMeta[keyB]
	i.funcMu.RUnlock()
	if objectA == nil || !objectA.hostShared || objectB == nil || objectB.hostShared {
		t.Fatalf("delivered token mismatch: objectA shared=%v objectB shared=%v", objectA != nil && objectA.hostShared, objectB != nil && objectB.hostShared)
	}
	if metaA.retention != funcMetaOpaque || metaB.retention != funcMetaVisible {
		t.Fatalf("delivered callback token mismatch: retentionA=%d retentionB=%d", metaA.retention, metaB.retention)
	}
}

func TestGorneshOwnedChannelReceiveMatchesImmutableShellSignature(t *testing.T) {
	i := New(Options{})
	root := i.frame
	typ := reflect.TypeOf(detachedChannelOrderPayloadGornesh{})
	channel := reflect.MakeChan(reflect.ChanOf(reflect.BothDir, typ), 1)
	i.registerOwnedChannel(channel, root)
	sharedMap := reflect.MakeMap(reflect.TypeOf(map[string]int{}))
	i.registerOwnedValue(sharedMap, root)
	payloadA := reflect.ValueOf(detachedChannelOrderPayloadGornesh{Marker: 1, Value: sharedMap.Interface().(map[string]int)})
	payloadB := reflect.ValueOf(detachedChannelOrderPayloadGornesh{Marker: 2, Value: sharedMap.Interface().(map[string]int)})

	i.funcSweepMu.RLock()
	sendA := i.markInterpretedFuncChannelSend(root, channel, payloadA)
	sendB := i.markInterpretedFuncChannelSend(root, channel, payloadB)
	channel.Send(payloadB)
	delivered, _ := channel.Recv()
	i.adoptInterpretedFuncValue(root, channel, delivered)
	i.funcSweepMu.RUnlock()

	if sendA == nil || sendB == nil {
		t.Fatal("channel sends were not tracked")
	}
	if sendA.state == ownedChannelSendTerminal || sendB.state != ownedChannelSendTerminal {
		t.Fatalf("immutable shell matched wrong token: stateA=%d stateB=%d", sendA.state, sendB.state)
	}
	i.funcSweepMu.RLock()
	i.rollbackInterpretedFuncChannelSend(sendA)
	i.funcSweepMu.RUnlock()
}

func TestGorneshOwnedChannelReceiveMatchesReferenceViews(t *testing.T) {
	for _, test := range []struct {
		name   string
		values func(*Interpreter, *frame) (reflect.Value, reflect.Value)
	}{
		{
			name: "overlapping slices",
			values: func(i *Interpreter, root *frame) (reflect.Value, reflect.Value) {
				base := reflect.MakeSlice(reflect.TypeOf([]int{}), 2, 2)
				i.registerOwnedValue(base, root)
				return base.Slice3(0, 1, 2), base.Slice3(1, 2, 2)
			},
		},
		{
			name: "interior pointers",
			values: func(i *Interpreter, root *frame) (reflect.Value, reflect.Value) {
				base := reflect.New(reflect.TypeOf([2]int{}))
				i.registerOwnedValue(base, root)
				return base.Elem().Index(0).Addr(), base.Elem().Index(1).Addr()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			i := New(Options{})
			root := i.frame
			valueA, valueB := test.values(i, root)
			channel := reflect.MakeChan(reflect.ChanOf(reflect.BothDir, valueA.Type()), 1)
			i.registerOwnedChannel(channel, root)
			i.funcSweepMu.RLock()
			sendA := i.markInterpretedFuncChannelSend(root, channel, valueA)
			sendB := i.markInterpretedFuncChannelSend(root, channel, valueB)
			channel.Send(valueB)
			delivered, _ := channel.Recv()
			i.adoptInterpretedFuncValue(root, channel, delivered)
			i.funcSweepMu.RUnlock()
			if sendA == nil || sendB == nil {
				t.Fatal("channel sends were not tracked")
			}
			if sendA.state == ownedChannelSendTerminal || sendB.state != ownedChannelSendTerminal {
				t.Fatalf("reference view matched wrong token: stateA=%d stateB=%d", sendA.state, sendB.state)
			}
			i.funcSweepMu.RLock()
			i.rollbackInterpretedFuncChannelSend(sendA)
			i.funcSweepMu.RUnlock()
		})
	}
}

func TestGorneshOwnedChannelReachableThroughClosureCaptureSurvivesRelease(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var capturedOwnedChannelGetterGornesh func() map[string]int
func installCapturedOwnedChannelGornesh() {
	channel := make(chan map[string]int, 1)
	channel <- map[string]int{"value": 1}
	capturedOwnedChannelGetterGornesh = func() map[string]int {
		value := <-channel
		return value
	}
}
0`); err != nil {
		t.Fatalf("define captured owned channel: %v", err)
	}
	i.funcMu.RLock()
	channelBaseline, objectBaseline := len(i.ownedChannels), len(i.ownedObjects)
	i.funcMu.RUnlock()
	if _, err := i.Eval(`installCapturedOwnedChannelGornesh(); 0`); err != nil {
		t.Fatalf("install captured owned channel: %v", err)
	}
	i.funcMu.RLock()
	channelsAfterInstall := len(i.ownedChannels)
	i.funcMu.RUnlock()
	if channelsAfterInstall != channelBaseline+1 {
		t.Fatalf("captured channel metadata after installer release = %d, want %d", channelsAfterInstall, channelBaseline+1)
	}
	value, err := i.Eval(`capturedOwnedChannelGetterGornesh()["value"]`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("receive through captured channel: value=%v err=%v", value, err)
	}
	if _, err := i.Eval(`capturedOwnedChannelGetterGornesh = nil; 0`); err != nil {
		t.Fatalf("clear captured channel getter: %v", err)
	}
	i.funcMu.RLock()
	channelCount, objectCount := len(i.ownedChannels), len(i.ownedObjects)
	i.funcMu.RUnlock()
	if channelCount != channelBaseline || objectCount != objectBaseline {
		t.Fatalf("captured channel cleanup = channels %d objects %d, want %d/%d", channelCount, objectCount, channelBaseline, objectBaseline)
	}
}

func TestGorneshRootOwnedChannelReplacementChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var rootMapChannelChurnGornesh = make(chan map[string]int, 1)
var rootFuncChannelChurnGornesh = make(chan func(), 1)
func churnRootOwnedChannelsGornesh() {
	rootMapChannelChurnGornesh <- map[string]int{"value": 1}
	rootFuncChannelChurnGornesh <- func() {}
	rootMapChannelChurnGornesh = make(chan map[string]int, 1)
	rootFuncChannelChurnGornesh = make(chan func(), 1)
}
0`); err != nil {
		t.Fatalf("define root channel churn: %v", err)
	}
	i.funcMu.RLock()
	channelBaseline, objectBaseline, funcBaseline := len(i.ownedChannels), len(i.ownedObjects), len(i.funcMeta)
	i.funcMu.RUnlock()
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`churnRootOwnedChannelsGornesh(); 0`); err != nil {
			t.Fatalf("root channel churn iteration %d: %v", iteration, err)
		}
		i.funcMu.RLock()
		channelCount, objectCount, funcCount := len(i.ownedChannels), len(i.ownedObjects), len(i.funcMeta)
		i.funcMu.RUnlock()
		if channelCount != channelBaseline || objectCount != objectBaseline || funcCount != funcBaseline {
			t.Fatalf("root channel metadata iteration %d = channels %d objects %d funcs %d, want %d/%d/%d", iteration, channelCount, objectCount, funcCount, channelBaseline, objectBaseline, funcBaseline)
		}
	}
}
