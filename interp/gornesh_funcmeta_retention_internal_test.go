package interp

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func funcMetaCountGornesh(i *Interpreter) int {
	i.funcMu.RLock()
	defer i.funcMu.RUnlock()
	return len(i.funcMeta)
}

func waitForFuncSweepGornesh(i *Interpreter) {
	i.funcSweepMu.Lock()
	defer i.funcSweepMu.Unlock()
	i.funcMu.RLock()
	_ = len(i.funcMeta)
	i.funcMu.RUnlock()
}

func cancelBlockedFuncMetaEvalGornesh(t *testing.T, i *Interpreter, source string, entered, release chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, source)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled metadata Eval returned nil error")
	}
	close(release)
}

func TestGorneshInterpretedFuncMetadataRootSweepKeepsPostSnapshotRegistration(t *testing.T) {
	i := New(Options{})
	root := i.frame
	oldKey, _ := funcvalKeyOf(reflect.ValueOf(func() {}))
	newKey, _ := funcvalKeyOf(reflect.ValueOf(func() {}))
	oldGroup := &funcMetaGroup{root: root, version: 1}
	newGroup := &funcMetaGroup{root: root, version: 1}
	oldMeta := interpretedFuncMeta{frame: root, group: oldGroup}
	newMeta := interpretedFuncMeta{frame: root, group: newGroup}

	i.funcMu.Lock()
	i.funcMeta[oldKey] = oldMeta
	i.funcMeta[newKey] = newMeta
	root.funcMeta = append(root.funcMeta, oldKey, newKey)
	i.funcMu.Unlock()

	// The new entry represents a root-visible wrapper registered after the
	// sweep candidate snapshot. Only the snapshotted old entry may be removed.
	i.deleteUnreachableRootFuncMeta(root,
		map[uintptr]interpretedFuncMeta{oldKey: oldMeta},
		map[*funcMetaGroup]struct{}{},
		map[*funcMetaGroup]uint64{oldGroup: oldGroup.version})

	i.funcMu.RLock()
	_, oldExists := i.funcMeta[oldKey]
	_, newExists := i.funcMeta[newKey]
	i.funcMu.RUnlock()
	if oldExists {
		t.Fatal("unreachable snapshotted metadata was not removed")
	}
	if !newExists {
		t.Fatal("post-snapshot root metadata was removed")
	}
}

func TestGorneshInterpretedFuncMetadataSweepWaitsForInterpretedQuiescence(t *testing.T) {
	started := make(chan struct{})
	stop := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	baseline := funcMetaCountGornesh(i)
	if err := i.Use(Exports{"retention/retention": {
		"Started":     reflect.ValueOf(func() { close(started) }),
		"WaitStarted": reflect.ValueOf(func() { <-started }),
		"ShouldStop": reflect.ValueOf(func() bool {
			select {
			case <-stop:
				return true
			default:
				return false
			}
		}),
		"Finished": reflect.ValueOf(func() { close(finished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var concurrentFuncMapGornesh = map[int]func(){0: func(){}}
var concurrentCaptureGlobalGornesh func()
func launchConcurrentFuncMutationGornesh() {
	captured := func() {}
	concurrentCaptureGlobalGornesh = func() { captured() }
	go func() {
		retention.Started()
		for !retention.ShouldStop() {
			captured = func() {}
			concurrentFuncMapGornesh[0] = func() {}
		}
		retention.Finished()
	}()
}

launchConcurrentFuncMutationGornesh()
retention.WaitStarted()
0`); err != nil {
		t.Fatalf("launch concurrent function mutation: %v", err)
	}
	close(stop)
	<-finished
	// Finished is emitted immediately before the interpreted goroutine returns.
	// Taking the exclusive fence makes its metadata release deterministic.
	waitForFuncSweepGornesh(i)
	if _, err := i.Eval(`concurrentFuncMapGornesh = nil; concurrentCaptureGlobalGornesh = nil; 0`); err != nil {
		t.Fatalf("clear concurrently mutated callbacks: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after concurrent mutation quiesced = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataGlobalReplacementWithBackgroundGoroutineIsBounded(t *testing.T) {
	started := make(chan struct{})
	stop := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	initialBaseline := funcMetaCountGornesh(i)
	if err := i.Use(Exports{"retention/retention": {
		"BackgroundStarted":     reflect.ValueOf(func() { close(started) }),
		"WaitBackgroundStarted": reflect.ValueOf(func() { <-started }),
		"StopBackground": reflect.ValueOf(func() bool {
			select {
			case <-stop:
				return true
			default:
				return false
			}
		}),
		"BackgroundFinished": reflect.ValueOf(func() { close(finished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var backgroundReplacementCallbackGornesh func() int
var backgroundReplacementSequenceGornesh int
var backgroundReplacementTicksGornesh int
func replaceBackgroundCallbackGornesh() {
	backgroundReplacementSequenceGornesh++
	want := backgroundReplacementSequenceGornesh
	backgroundReplacementCallbackGornesh = func() int { return want }
}
go func() {
	retention.BackgroundStarted()
	for !retention.StopBackground() {
		backgroundReplacementTicksGornesh++
	}
	retention.BackgroundFinished()
}()
retention.WaitBackgroundStarted()
0`); err != nil {
		t.Fatalf("launch background replacement goroutine: %v", err)
	}
	if _, err := i.Eval(`replaceBackgroundCallbackGornesh(); 0`); err != nil {
		t.Fatalf("initial background replacement: %v", err)
	}
	for iteration := 2; iteration <= 20; iteration++ {
		value, err := i.Eval(`replaceBackgroundCallbackGornesh(); backgroundReplacementCallbackGornesh()`)
		if err != nil || value.Interface() != iteration {
			t.Fatalf("background replacement iteration %d: value=%v err=%v", iteration, value, err)
		}
		if got := funcMetaCountGornesh(i); got > initialBaseline+3 {
			t.Fatalf("metadata after background replacement iteration %d = %d, want bounded by %d", iteration, got, initialBaseline+3)
		}
	}
	close(stop)
	<-finished
	waitForFuncSweepGornesh(i)
	if _, err := i.Eval(`backgroundReplacementCallbackGornesh = nil; 0`); err != nil {
		t.Fatalf("clear background replacement callback: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != initialBaseline {
		t.Fatalf("metadata after background replacement cleanup = %d, want baseline %d", got, initialBaseline)
	}
}

func TestGorneshInterpretedFuncMetadataLocalChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func churnFuncMetadataGornesh() {
	for index := 0; index < 20; index++ {
		callback := func() int { return index }
		_ = callback
	}
}`); err != nil {
		t.Fatalf("define churnFuncMetadataGornesh: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	for iteration := 0; iteration < 5; iteration++ {
		if _, err := i.Eval(`churnFuncMetadataGornesh()`); err != nil {
			t.Fatalf("churn iteration %d: %v", iteration, err)
		}
		if got := funcMetaCountGornesh(i); got != baseline {
			t.Fatalf("func metadata after iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshInterpretedFuncMetadataReturnedClosureIsRetained(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func makeReturnedClosureGornesh() func() int {
	value := 40
	return func() int { value++; return value }
}`); err != nil {
		t.Fatalf("define makeReturnedClosureGornesh: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	value, err := i.Eval(`makeReturnedClosureGornesh()`)
	if err != nil {
		t.Fatalf("create returned closure: %v", err)
	}
	callback := value.Interface().(func() int)
	if got := callback(); got != 41 {
		t.Fatalf("returned closure result = %d, want 41", got)
	}
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("func metadata after returned closure = %d, want more than baseline %d", got, baseline)
	}
}

func TestGorneshHostReturnedClosureRebindsAfterCanceledRootDetach(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var saved func()
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"Get": reflect.ValueOf(func() func() { return saved }),
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var reboundGlobalGornesh int
func makeHostReturnedClosureGornesh() func() {
	return func() { reboundGlobalGornesh++ }
}`); err != nil {
		t.Fatalf("define host-returned closure: %v", err)
	}
	value, err := i.Eval(`makeHostReturnedClosureGornesh()`)
	if err != nil {
		t.Fatalf("return closure to host: %v", err)
	}
	saved = value.Interface().(func())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `retention.Block()`)
		done <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("canceled Eval returned nil error")
	}
	if _, err := i.Eval(`reboundGlobalGornesh = 100; retention.Get()()`); err != nil {
		t.Fatalf("invoke host-returned closure after detach: %v", err)
	}
	value, err = i.Eval(`reboundGlobalGornesh`)
	if err != nil || value.Interface() != 101 {
		t.Fatalf("rebound global after host round trip: value=%v err=%v", value, err)
	}
}

func TestGorneshInterpretedFuncMetadataAncestorReachability(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"metadata/metadata": {
		"Count": reflect.ValueOf(func() int { return funcMetaCountGornesh(i) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "metadata"
func ancestorReachabilityGornesh() int {
	var saved func() int
	install := func() {
		value := 42
		saved = func() int { return value }
	}
	install()
	if saved() != 42 { panic("lost ancestor callback") }
	return metadata.Count()
}`); err != nil {
		t.Fatalf("define ancestorReachabilityGornesh: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	value, err := i.Eval(`ancestorReachabilityGornesh()`)
	if err != nil {
		t.Fatalf("run ancestorReachabilityGornesh: %v", err)
	}
	if got := value.Interface().(int); got <= baseline {
		t.Fatalf("metadata during ancestor reachability = %d, want more than baseline %d", got, baseline)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after ancestor returns = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataGlobalReachability(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var globalCallbackGornesh func() int
func installGlobalCallbackGornesh() {
	value := 42
	globalCallbackGornesh = func() int { return value }
}`); err != nil {
		t.Fatalf("define global callback: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`installGlobalCallbackGornesh()`); err != nil {
		t.Fatalf("install global callback: %v", err)
	}
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("metadata after global assignment = %d, want more than baseline %d", got, baseline)
	}
	value, err := i.Eval(`globalCallbackGornesh()`)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("global callback: value=%v err=%v", value, err)
	}
}

func TestGorneshInterpretedFuncMetadataNativeRetainedBoundCopy(t *testing.T) {
	var retained, retainedDirect func() int
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"Store": reflect.ValueOf(func(callbacks map[string][]func() int) {
			retained = callbacks["callback"][0]
		}),
		"StoreDirect": reflect.ValueOf(func(callback func() int) {
			retainedDirect = callback
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
func retainNestedCallbackGornesh() {
	value := 42
	callbacks := map[string][]func() int{"callback": {func() int { return value }}}
	retention.Store(callbacks)
	retention.StoreDirect(func() int { return value + 1 })
}`); err != nil {
		t.Fatalf("define native retention: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`retainNestedCallbackGornesh()`); err != nil {
		t.Fatalf("store native callback: %v", err)
	}
	if retained == nil || retained() != 42 {
		t.Fatal("native-retained bound callback did not preserve its closure")
	}
	if retainedDirect == nil || retainedDirect() != 43 {
		t.Fatal("native-retained direct callback did not preserve its closure")
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after native retained a bound copy = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataChannelEscapeIsRetained(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var callbackChannelGornesh = make(chan func() int, 1)
func sendCallbackGornesh() {
	value := 42
	callbackChannelGornesh <- func() int { return value }
}`); err != nil {
		t.Fatalf("define channel escape: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`sendCallbackGornesh()`); err != nil {
		t.Fatalf("send callback: %v", err)
	}
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("metadata after channel escape = %d, want more than baseline %d", got, baseline)
	}
	value, err := i.Eval(`(<-callbackChannelGornesh)()`)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("escaped channel callback: value=%v err=%v", value, err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after channel drain = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataChannelSendDrainChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var churnCallbackChannelGornesh = make(chan func() int, 1)
var churnCallbackSequenceGornesh int
func sendUniqueCallbackGornesh() int {
	churnCallbackSequenceGornesh++
	want := churnCallbackSequenceGornesh
	churnCallbackChannelGornesh <- func() int { return want }
	return want
}`); err != nil {
		t.Fatalf("define channel churn: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	for iteration := 1; iteration <= 20; iteration++ {
		value, err := i.Eval(`want := sendUniqueCallbackGornesh(); got := (<-churnCallbackChannelGornesh)(); []int{want, got}`)
		if err != nil {
			t.Fatalf("channel churn iteration %d: %v", iteration, err)
		}
		pair := value.Interface().([]int)
		if pair[0] != iteration || pair[1] != iteration {
			t.Fatalf("channel churn iteration %d values = %v", iteration, pair)
		}
		if got := funcMetaCountGornesh(i); got != baseline {
			t.Fatalf("metadata after channel churn iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshInterpretedFuncMetadataRangeChannelDrainChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var rangeCallbackResultGornesh int
var rangeCallbackSequenceGornesh int
func rangeDrainUniqueCallbackGornesh() int {
	rangeCallbackSequenceGornesh++
	want := rangeCallbackSequenceGornesh
	callbacks := make(chan func(), 1)
	callbacks <- func() { rangeCallbackResultGornesh = want }
	close(callbacks)
	for callback := range callbacks {
		callback()
	}
	return rangeCallbackResultGornesh
}`); err != nil {
		t.Fatalf("define range channel churn: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	for iteration := 1; iteration <= 20; iteration++ {
		value, err := i.Eval(`rangeDrainUniqueCallbackGornesh()`)
		if err != nil {
			t.Fatalf("range channel churn iteration %d: %v", iteration, err)
		}
		if got := value.Interface().(int); got != iteration {
			t.Fatalf("range channel churn iteration %d value = %d, want %d", iteration, got, iteration)
		}
		if got := funcMetaCountGornesh(i); got != baseline {
			t.Fatalf("metadata after range channel churn iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshInterpretedFuncMetadataSelectSameGroupRollbackIsCounted(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var rollbackChannelOneGornesh = make(chan func())
var rollbackChannelTwoGornesh = make(chan func())
func rollbackSameCallbackSelectGornesh() {
	callback := func() {}
	select {
	case rollbackChannelOneGornesh <- callback:
	case rollbackChannelTwoGornesh <- callback:
	default:
	}
}`); err != nil {
		t.Fatalf("define select rollback churn: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`rollbackSameCallbackSelectGornesh()`); err != nil {
			t.Fatalf("select rollback iteration %d: %v", iteration, err)
		}
		if got := funcMetaCountGornesh(i); got != baseline {
			t.Fatalf("metadata after select rollback iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshInterpretedFuncMetadataChannelPendingCountSurvivesRootDetach(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"BlockPending": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var pendingCallbackChannelGornesh = make(chan func(), 2)
var pendingCallbackGlobalGornesh int
func sendTwoPendingCallbacksGornesh() {
	pendingCallbackChannelGornesh <- func() { pendingCallbackGlobalGornesh++ }
	pendingCallbackChannelGornesh <- func() { pendingCallbackGlobalGornesh++ }
}`); err != nil {
		t.Fatalf("define pending channel callbacks: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`sendTwoPendingCallbacksGornesh(); (<-pendingCallbackChannelGornesh)()`); err != nil {
		t.Fatalf("send two and receive first callback: %v", err)
	}
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("metadata with second callback pending = %d, want more than baseline %d", got, baseline)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `retention.BlockPending()`)
		done <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("canceled pending-channel Eval returned nil error")
	}
	if _, err := i.Eval(`pendingCallbackGlobalGornesh = 100; (<-pendingCallbackChannelGornesh)()`); err != nil {
		t.Fatalf("receive second callback after root detach: %v", err)
	}
	value, err := i.Eval(`pendingCallbackGlobalGornesh`)
	if err != nil || value.Interface() != 101 {
		t.Fatalf("pending callback rebound global: value=%v err=%v", value, err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after second channel drain = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataRangeChannelRebindsAfterRootDetach(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"BlockRange": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var rangePendingCallbackChannelGornesh = make(chan func(), 1)
var rangePendingCallbackGlobalGornesh int
func sendRangePendingCallbackGornesh() {
	rangePendingCallbackChannelGornesh <- func() { rangePendingCallbackGlobalGornesh++ }
}
func drainRangePendingCallbackGornesh() {
	for callback := range rangePendingCallbackChannelGornesh {
		callback()
	}
}`); err != nil {
		t.Fatalf("define pending range callback: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`sendRangePendingCallbackGornesh()`); err != nil {
		t.Fatalf("send pending range callback: %v", err)
	}
	cancelBlockedFuncMetaEvalGornesh(t, i, `retention.BlockRange()`, entered, release)
	value, err := i.Eval(`
rangePendingCallbackGlobalGornesh = 100
close(rangePendingCallbackChannelGornesh)
drainRangePendingCallbackGornesh()
rangePendingCallbackGlobalGornesh`)
	if err != nil || value.Interface() != 101 {
		t.Fatalf("range callback rebound global: value=%v err=%v", value, err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after detached range drain = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshNestedSliceCallbackRemainsOriginBoundAfterRootDetach(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"BlockSlice": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var hiddenSliceCallbacksGornesh = make([]func(), 1)
var hiddenSliceCallbackGlobalGornesh int
func installHiddenSliceCallbackGornesh() {
	hiddenSliceCallbacksGornesh[0] = func() { hiddenSliceCallbackGlobalGornesh++ }
	hiddenSliceCallbacksGornesh = hiddenSliceCallbacksGornesh[:0]
}`); err != nil {
		t.Fatalf("define hidden slice callback: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`installHiddenSliceCallbackGornesh(); 0`); err != nil {
		t.Fatalf("install hidden slice callback: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata for origin-bound nested slice callback = %d, want baseline %d", got, baseline)
	}
	cancelBlockedFuncMetaEvalGornesh(t, i, `retention.BlockSlice()`, entered, release)
	value, err := i.Eval(`
hiddenSliceCallbackGlobalGornesh = 100
hiddenSliceCallbacksGornesh = hiddenSliceCallbacksGornesh[:1]
hiddenSliceCallbacksGornesh[0]()
hiddenSliceCallbackGlobalGornesh`)
	if err != nil || value.Interface() != 100 {
		t.Fatalf("origin-bound slice callback attached to later root: value=%v err=%v", value, err)
	}
	if _, err := i.Eval(`hiddenSliceCallbacksGornesh = nil; 0`); err != nil {
		t.Fatalf("clear hidden slice callback: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after clearing hidden slice callback = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataCapturedGroupReachabilitySurvivesRootDetach(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"BlockCapture": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var capturedGroupInnerGornesh func()
var capturedGroupOuterGornesh func()
var capturedGroupGlobalGornesh int
func installCapturedGroupInnerGornesh() {
	capturedGroupInnerGornesh = func() { capturedGroupGlobalGornesh++ }
}
func installCapturedGroupOuterGornesh() {
	captured := capturedGroupInnerGornesh
	capturedGroupOuterGornesh = func() { captured() }
	capturedGroupInnerGornesh = nil
}`); err != nil {
		t.Fatalf("define captured group callbacks: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`installCapturedGroupInnerGornesh(); installCapturedGroupOuterGornesh(); 0`); err != nil {
		t.Fatalf("install captured group callbacks: %v", err)
	}
	if got := funcMetaCountGornesh(i); got < baseline+2 {
		t.Fatalf("metadata for captured group chain = %d, want at least %d", got, baseline+2)
	}
	cancelBlockedFuncMetaEvalGornesh(t, i, `retention.BlockCapture()`, entered, release)
	value, err := i.Eval(`capturedGroupGlobalGornesh = 100; capturedGroupOuterGornesh(); capturedGroupGlobalGornesh`)
	if err != nil || value.Interface() != 101 {
		t.Fatalf("captured callback rebound global: value=%v err=%v", value, err)
	}
	if _, err := i.Eval(`capturedGroupOuterGornesh = nil; 0`); err != nil {
		t.Fatalf("clear captured callback chain: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after clearing captured callback chain = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataGlobalReplacementIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var replaceableCallbackGornesh func() int
var replacementSequenceGornesh int
func replaceGlobalCallbackGornesh() {
	replacementSequenceGornesh++
	want := replacementSequenceGornesh
	replaceableCallbackGornesh = func() int { return want }
}`); err != nil {
		t.Fatalf("define global replacement: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`replaceGlobalCallbackGornesh()`); err != nil {
		t.Fatalf("initial global replacement: %v", err)
	}
	steady := funcMetaCountGornesh(i)
	if steady <= baseline {
		t.Fatalf("metadata after initial global replacement = %d, want more than baseline %d", steady, baseline)
	}
	for iteration := 2; iteration <= 20; iteration++ {
		value, err := i.Eval(`replaceGlobalCallbackGornesh(); replaceableCallbackGornesh()`)
		if err != nil || value.Interface() != iteration {
			t.Fatalf("global replacement iteration %d: value=%v err=%v", iteration, value, err)
		}
		if got := funcMetaCountGornesh(i); got != steady {
			t.Fatalf("metadata after global replacement iteration %d = %d, want steady %d", iteration, got, steady)
		}
	}
	if _, err := i.Eval(`replaceableCallbackGornesh = nil`); err != nil {
		t.Fatalf("clear global callback: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after clearing global callback = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataVisibleGlobalTransfersAcrossRootDetach(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"BlockVisibleGlobal": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var detachedVisibleGlobalGornesh func() int
func installDetachedVisibleGlobalGornesh() {
	value := 42
	detachedVisibleGlobalGornesh = func() int { return value }
}`); err != nil {
		t.Fatalf("define detached visible global: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`installDetachedVisibleGlobalGornesh()`); err != nil {
		t.Fatalf("install detached visible global: %v", err)
	}
	if got := funcMetaCountGornesh(i); got <= baseline {
		t.Fatalf("metadata after visible global install = %d, want more than baseline %d", got, baseline)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `retention.BlockVisibleGlobal()`)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled visible-global Eval returned nil error")
	}
	close(release)
	if _, err := i.Eval(`detachedVisibleGlobalGornesh = nil`); err != nil {
		t.Fatalf("clear visible global after root detach: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after detached visible global clear = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataRecoveredFunctionPanicChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func panicWithCallbackGornesh() {
	value := 42
	panic(func() int { return value })
}
func recoverAndDiscardCallbackGornesh() {
	defer func() { _ = recover() }()
	panicWithCallbackGornesh()
}`); err != nil {
		t.Fatalf("define recovered callback panic: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := i.Eval(`recoverAndDiscardCallbackGornesh()`); err != nil {
			t.Fatalf("recovered callback panic iteration %d: %v", iteration, err)
		}
		if got := funcMetaCountGornesh(i); got != baseline {
			t.Fatalf("metadata after recovered callback panic iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshRawCallbackInNativeRetainedReferenceKeepsOriginalOwner(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var retained map[string]func()
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"StoreMap": reflect.ValueOf(func(callbacks map[string]func()) { retained = callbacks }),
		"Get":      reflect.ValueOf(func() func() { return retained["callback"] }),
		"Wait": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var rawEscapedCallbackRunsGornesh int
var rawEscapedUnrelatedCallbacksGornesh = map[string]func(){}
var rawEscapedEmptyMapChannelGornesh = make(chan map[string]func(), 1)
func installRawEscapedCallbackGornesh() {
	callbacks := map[string]func(){}
	retention.StoreMap(callbacks)
	callbacks["callback"] = func() { rawEscapedCallbackRunsGornesh++ }
	rawEscapedEmptyMapChannelGornesh <- map[string]func(){}
	select {
	case rawEscapedEmptyMapChannelGornesh <- map[string]func(){}:
	default:
	}
	retention.Wait()
}`); err != nil {
		t.Fatalf("define raw callback escape: %v", err)
	}
	baseline := funcMetaCountGornesh(i)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `installRawEscapedCallbackGornesh()`)
		done <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("canceled callback installation returned nil error")
	}
	if retained == nil || retained["callback"] == nil {
		t.Fatal("native code did not retain the callback map")
	}
	if _, err := i.Eval(`var laterEvalGornesh = 1`); err != nil {
		t.Fatalf("later Eval: %v", err)
	}
	retained["callback"]()
	value, err := i.Eval(`rawEscapedCallbackRunsGornesh`)
	if err != nil {
		t.Fatalf("read raw callback count: %v", err)
	}
	if got := value.Interface().(int); got != 0 {
		t.Fatalf("raw callback attached to later Eval %d time(s), want 0", got)
	}
	value, err = i.Eval(`retention.Get()(); rawEscapedCallbackRunsGornesh`)
	if err != nil {
		t.Fatalf("round-trip raw callback through native code: %v", err)
	}
	if got := value.Interface().(int); got != 0 {
		t.Fatalf("ambiguous unrelated map promoted raw callback to later root %d time(s), want 0", got)
	}
	if _, err := i.Eval(`<-rawEscapedEmptyMapChannelGornesh; 0`); err != nil {
		t.Fatalf("drain unrelated empty callback map: %v", err)
	}
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("ambiguous buffered send/select metadata = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataScalarEscapeChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func scalarSendMetadataChurnGornesh() {
	callback := func() {}
	channel := make(chan int, 1)
	channel <- 1
	_ = callback
}
func scalarPanicMetadataChurnGornesh() {
	callback := func() {}
	_ = callback
	panic(1)
}
func recoverScalarPanicMetadataChurnGornesh() {
	defer func() { _ = recover() }()
	scalarPanicMetadataChurnGornesh()
}`); err != nil {
		t.Fatalf("define scalar escape churn: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	for iteration := 0; iteration < 5; iteration++ {
		if _, err := i.Eval(`scalarSendMetadataChurnGornesh(); recoverScalarPanicMetadataChurnGornesh()`); err != nil {
			t.Fatalf("scalar escape churn iteration %d: %v", iteration, err)
		}
		if got := funcMetaCountGornesh(i); got != baseline {
			t.Fatalf("metadata after scalar escape iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshInterpretedFuncMetadataConcurrentChildReleaseIsBounded(t *testing.T) {
	var assigned sync.WaitGroup
	var finished sync.WaitGroup
	assigned.Add(100)
	finished.Add(100)
	release := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"metadata/metadata": {
		"Assigned": reflect.ValueOf(func() { assigned.Done() }),
		"Block":    reflect.ValueOf(func() { <-release }),
		"Run": reflect.ValueOf(func(callback func()) {
			callback()
			finished.Done()
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "metadata"
func concurrentChildMetadataGornesh() {
	var saved func() int
	go metadata.Run(func() {
		saved = func() int { return 42 }
		metadata.Assigned()
		metadata.Block()
		_ = saved
	})
}`); err != nil {
		t.Fatalf("define concurrent metadata churn: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	if _, err := i.Eval(`for iteration := 0; iteration < 100; iteration++ { concurrentChildMetadataGornesh() }`); err != nil {
		t.Fatalf("concurrent churn: %v", err)
	}
	assigned.Wait()
	close(release)
	finished.Wait()
	if got := funcMetaCountGornesh(i); got != baseline {
		t.Fatalf("metadata after concurrent child release = %d, want baseline %d", got, baseline)
	}
}

func TestGorneshInterpretedFuncMetadataUnrelatedReturnedFunctionChurnIsBounded(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"metadata/metadata": {
		"Native": reflect.ValueOf(func() int { return 42 }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "metadata"
func returnUnrelatedFunctionGornesh() func() int {
	temporary := func() int { return 0 }
	_ = temporary
	return metadata.Native
}`); err != nil {
		t.Fatalf("define unrelated function return: %v", err)
	}
	baseline := funcMetaCountGornesh(i)
	for iteration := 0; iteration < 5; iteration++ {
		value, err := i.Eval(`returnUnrelatedFunctionGornesh()()`)
		if err != nil || value.Interface() != 42 {
			t.Fatalf("unrelated function return iteration %d: value=%v err=%v", iteration, value, err)
		}
		if got := funcMetaCountGornesh(i); got != baseline {
			t.Fatalf("metadata after unrelated function return iteration %d = %d, want baseline %d", iteration, got, baseline)
		}
	}
}

func TestGorneshEscapeTokenUsesExactFunctionCaptures(t *testing.T) {
	i := New(Options{})
	root := i.frame
	captured := reflect.MakeMap(reflect.TypeOf(map[string]int{}))
	captured.SetMapIndex(reflect.ValueOf("value"), reflect.ValueOf(1))
	i.registerOwnedValue(captured, root)
	frame := newFrame(root, 1, root.runid())
	frame.data[0] = captured
	group := &funcMetaGroup{root: root, captures: []funcMetaCapture{{frame: frame, index: 0}}}
	wrapper := reflect.ValueOf(func() {})
	key, _ := funcvalKeyOf(wrapper)
	i.funcMu.Lock()
	i.funcMeta[key] = interpretedFuncMeta{frame: root, group: group, captures: nil}
	objects := map[*ownedObject]struct{}{}
	i.collectOwnedChannelGraphLocked(wrapper, objects, map[reflect.Value]struct{}{}, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
	i.funcMu.Unlock()
	if len(objects) != 0 {
		t.Fatalf("capture-free function retained %d sibling-owned object(s)", len(objects))
	}
}

func TestGorneshEscapeTokenDoesNotReadHostSharedCaptureCell(t *testing.T) {
	i := New(Options{})
	root := i.frame
	frame := newFrame(root, 1, root.runid())
	frame.data[0] = reflect.New(reflect.TypeOf((func())(nil))).Elem()
	frame.data[0].Set(reflect.ValueOf(func() {}))
	cell := frame.data[0].Addr().Interface().(*func())
	i.registerOwnedValue(frame.data[0].Addr(), root)
	i.markOwnedValuesHostShared(frame.data[0].Addr())
	i.funcMu.RLock()
	hostShared := i.ownedCellHostSharedLocked(frame.data[0])
	i.funcMu.RUnlock()
	if !hostShared {
		t.Fatal("captured callback cell was not marked host-shared")
	}

	wrapper := reflect.ValueOf(func() {})
	key, _ := funcvalKeyOf(wrapper)
	i.funcMu.Lock()
	i.funcMeta[key] = interpretedFuncMeta{frame: root, group: &funcMetaGroup{root: root}, captures: []funcMetaCapture{{frame: frame, index: 0}}}
	i.funcMu.Unlock()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				*cell = func() {}
			}
		}
	}()
	for iteration := 0; iteration < 1000; iteration++ {
		i.funcMu.Lock()
		i.collectOwnedChannelGraphLocked(wrapper, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
		i.funcMu.Unlock()
		if _, ok := snapshotFuncMetaCapture(funcMetaCapture{frame: frame, index: 0}); ok {
			t.Fatal("root sweep snapshot read host-shared captured callback cell")
		}
	}
	close(stop)
	<-done
}

func TestGorneshQueuedAggregateReplacementMetadataIsBounded(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var lateWriteChannelGornesh = make(chan map[string]interface{}, 1)
var lateWritePayloadGornesh = map[string]interface{}{}
var lateWriteResultGornesh int
func replaceLateWritePayloadGornesh(value int) {
	copy := value
	lateWritePayloadGornesh["pointer"] = &copy
	lateWritePayloadGornesh["callback"] = func() { lateWriteResultGornesh = *lateWritePayloadGornesh["pointer"].(*int) }
}
lateWriteChannelGornesh <- lateWritePayloadGornesh
0`); err != nil {
		t.Fatalf("define queued aggregate: %v", err)
	}

	for iteration := 1; iteration <= 20; iteration++ {
		if _, err := i.Eval(`replaceLateWritePayloadGornesh(` + strconv.Itoa(iteration) + `)`); err != nil {
			t.Fatalf("replace queued aggregate iteration %d: %v", iteration, err)
		}
		i.funcMu.RLock()
		maxObjects, maxFuncs := 0, 0
		for _, channel := range i.ownedChannels {
			for _, send := range channel.sends {
				if len(send.objects) > maxObjects {
					maxObjects = len(send.objects)
				}
				if len(send.funcs) > maxFuncs {
					maxFuncs = len(send.funcs)
				}
			}
		}
		i.funcMu.RUnlock()
		if maxObjects > 4 || maxFuncs > 2 {
			t.Fatalf("queued aggregate iteration %d retained objects=%d funcs=%d, want bounded", iteration, maxObjects, maxFuncs)
		}
	}
	value, err := i.Eval(`payload := <-lateWriteChannelGornesh; payload["callback"].(func())(); lateWriteResultGornesh`)
	if err != nil || value.Interface() != 20 {
		t.Fatalf("receive latest queued callback: value=%v err=%v", value, err)
	}
}

func TestGorneshPendingAggregateWritesStayOnDetachedSnapshot(t *testing.T) {
	enteredOne, releaseOne := make(chan struct{}), make(chan struct{})
	enteredTwo, releaseTwo := make(chan struct{}), make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"BlockLateOne": reflect.ValueOf(func() { close(enteredOne); <-releaseOne }),
		"BlockLateTwo": reflect.ValueOf(func() { close(enteredTwo); <-releaseTwo }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var pendingLateWriteChannelGornesh = make(chan map[string]interface{}, 1)
var pendingLateWritePayloadGornesh = map[string]interface{}{}
var pendingLateWriteResultGornesh int
func replacePendingLateWriteGornesh(value int) {
	want := value
	pendingLateWritePayloadGornesh["callback"] = func() { pendingLateWriteResultGornesh = want }
}
pendingLateWriteChannelGornesh <- pendingLateWritePayloadGornesh
0`); err != nil {
		t.Fatalf("define pending aggregate: %v", err)
	}
	cancelBlockedFuncMetaEvalGornesh(t, i, `retention.BlockLateOne()`, enteredOne, releaseOne)
	if _, err := i.Eval(`replacePendingLateWriteGornesh(1)`); err != nil {
		t.Fatalf("write first detached callback: %v", err)
	}
	cancelBlockedFuncMetaEvalGornesh(t, i, `retention.BlockLateTwo()`, enteredTwo, releaseTwo)
	if _, err := i.Eval(`replacePendingLateWriteGornesh(2)`); err != nil {
		t.Fatalf("write second detached callback: %v", err)
	}
	value, err := i.Eval(`payload := <-pendingLateWriteChannelGornesh; payload["callback"].(func())(); pendingLateWriteResultGornesh`)
	if err != nil || value.Interface() != 2 {
		t.Fatalf("receive detached late-written callback: value=%v err=%v", value, err)
	}
}

func TestGorneshPendingReplacementPreservesCallbackCopiedToGlobal(t *testing.T) {
	enteredOne, releaseOne := make(chan struct{}), make(chan struct{})
	enteredTwo, releaseTwo := make(chan struct{}), make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"retention/retention": {
		"BlockSavedOne": reflect.ValueOf(func() { close(enteredOne); <-releaseOne }),
		"BlockSavedTwo": reflect.ValueOf(func() { close(enteredTwo); <-releaseTwo }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "retention"
var savedPendingChannelGornesh = make(chan map[string]interface{}, 1)
var savedPendingPayloadGornesh = map[string]interface{}{}
var savedPendingCallbackGornesh func()
var savedPendingResultGornesh int
savedPendingChannelGornesh <- savedPendingPayloadGornesh
0`); err != nil {
		t.Fatalf("define saved pending aggregate: %v", err)
	}
	cancelBlockedFuncMetaEvalGornesh(t, i, `retention.BlockSavedOne()`, enteredOne, releaseOne)
	if _, err := i.Eval(`
callbackA := func() { savedPendingResultGornesh = savedPendingResultGornesh*10 + 1 }
savedPendingPayloadGornesh["callback"] = callbackA
savedPendingCallbackGornesh = callbackA
savedPendingPayloadGornesh["callback"] = func() { savedPendingResultGornesh = savedPendingResultGornesh*10 + 2 }`); err != nil {
		t.Fatalf("install and replace saved pending callback: %v", err)
	}
	cancelBlockedFuncMetaEvalGornesh(t, i, `retention.BlockSavedTwo()`, enteredTwo, releaseTwo)
	value, err := i.Eval(`
payload := <-savedPendingChannelGornesh
savedPendingCallbackGornesh()
payload["callback"].(func())()
savedPendingResultGornesh`)
	if err != nil || value.Interface() != 12 {
		t.Fatalf("invoke saved and received pending callbacks: value=%v err=%v", value, err)
	}
}

func TestGorneshPendingHostSharedMembershipsDoNotAccumulateAcrossDetach(t *testing.T) {
	nativePointer := new(int)
	i := New(Options{})
	if err := i.Use(Exports{"pendinghost/pendinghost": {
		"Pointer": reflect.ValueOf(func() *int { return nativePointer }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "pendinghost"
var pendingHostChannelGornesh = make(chan map[string]interface{}, 1)
var pendingHostPayloadGornesh = map[string]interface{}{}
var pendingHostResultGornesh int
pendingHostChannelGornesh <- pendingHostPayloadGornesh
0`); err != nil {
		t.Fatalf("define host-shared pending aggregate: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`
pendingHostPayloadGornesh["pointer"] = pendinghost.Pointer()
pendingHostPayloadGornesh["callback"] = func() { pendingHostResultGornesh++ }`); err != nil {
		t.Fatalf("insert host-shared pending values: %v", err)
	}
	for generation := 0; generation < 3; generation++ {
		forceDetachedRootCloneGornesh(t, i)
	}
	value, err := i.Eval(`
payload := <-pendingHostChannelGornesh
*payload["pointer"].(*int) = 7
payload["callback"].(func())()
pendingHostPayloadGornesh = nil
pendingHostResultGornesh`)
	if err != nil || value.Interface() != 1 || *nativePointer != 7 {
		t.Fatalf("receive host-shared pending aggregate: value=%v native=%d err=%v", value, *nativePointer, err)
	}
	i.funcMu.RLock()
	defer i.funcMu.RUnlock()
	for _, obj := range i.ownedObjects {
		if obj.channelRefs != 0 {
			t.Fatalf("owned object retained %d channel reference(s) after receive", obj.channelRefs)
		}
	}
	for _, meta := range i.funcMeta {
		if meta.group != nil && meta.group.pending != 0 {
			t.Fatalf("function group retained %d pending send(s) after receive", meta.group.pending)
		}
	}
}

func TestGorneshPublishingDetachedChannelReleasesPendingFunctionMetadata(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"publishpending/publishpending": {
		"Retain": reflect.ValueOf(func(chan map[string]interface{}) {}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "publishpending"
var publishPendingChannelGornesh = make(chan map[string]interface{}, 1)
var publishPendingPayloadGornesh = map[string]interface{}{}
publishPendingChannelGornesh <- publishPendingPayloadGornesh
0`); err != nil {
		t.Fatalf("define pending channel for publication: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`publishPendingPayloadGornesh["callback"] = func() int { return 1 }`); err != nil {
		t.Fatalf("insert detached pending callback: %v", err)
	}
	forceDetachedRootCloneGornesh(t, i)
	if _, err := i.Eval(`0`); err != nil {
		t.Fatalf("activate detached pending channel: %v", err)
	}
	i.funcMu.RLock()
	pendingKeys := map[uintptr]struct{}{}
	for _, channel := range i.ownedChannels {
		for _, send := range channel.sends {
			for value := range send.pendingFuncs {
				if key, ok := funcvalKeyOf(value); ok {
					pendingKeys[key] = struct{}{}
				}
			}
		}
	}
	i.funcMu.RUnlock()
	if len(pendingKeys) == 0 {
		t.Fatal("detached channel had no pending callback metadata")
	}
	if _, err := i.Eval(`publishpending.Retain(publishPendingChannelGornesh)`); err != nil {
		t.Fatalf("publish detached channel: %v", err)
	}
	i.funcMu.RLock()
	defer i.funcMu.RUnlock()
	for key := range pendingKeys {
		if meta, ok := i.funcMeta[key]; ok && meta.retention == funcMetaChannel {
			t.Fatal("published detached channel left pending callback channel-retained")
		}
	}
}
