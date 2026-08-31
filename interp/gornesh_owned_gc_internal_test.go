package interp

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

// sampleOwnedRegistryGornesh reads the ownership registry sizes under funcMu.
func sampleOwnedRegistryGornesh(i *Interpreter) (objects, channels int) {
	i.funcMu.RLock()
	defer i.funcMu.RUnlock()
	return len(i.ownedObjects), len(i.ownedChannels)
}

// watchOwnedRegistryGornesh samples the ownership registries until stop is
// closed and returns the largest combined size observed.
func watchOwnedRegistryGornesh(i *Interpreter, stop <-chan struct{}) int {
	maxSeen := 0
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			objects, channels := sampleOwnedRegistryGornesh(i)
			if objects+channels > maxSeen {
				maxSeen = objects + channels
			}
			return maxSeen
		case <-ticker.C:
			objects, channels := sampleOwnedRegistryGornesh(i)
			if objects+channels > maxSeen {
				maxSeen = objects + channels
			}
		}
	}
}

// TestGorneshOwnedRegistryBoundedUnderRootLoop pins the F1 bound: a single
// top-level Eval allocating far more owned maps than ownedGCRegistryCap must
// keep the object registry bounded via the incremental sweep while it runs.
func TestGorneshOwnedRegistryBoundedUnderRootLoop(t *testing.T) {
	i := New(Options{})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var maxSeen int
	wg.Add(1)
	go func() {
		defer wg.Done()
		maxSeen = watchOwnedRegistryGornesh(i, stop)
	}()
	done := make(chan error, 1)
	go func() {
		_, err := i.Eval(`
for i := 0; i < 300000; i++ {
	m := map[int]int{1: 1}
	_ = m
}
0`)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			close(stop)
			t.Fatalf("root allocation loop failed: %v", err)
		}
	case <-time.After(240 * time.Second):
		close(stop)
		t.Fatal("root allocation loop timed out")
	}
	close(stop)
	wg.Wait()
	if maxSeen > 2*ownedGCRegistryCap {
		t.Fatalf("owned object registry grew unbounded: max=%d cap=%d", maxSeen, ownedGCRegistryCap)
	}
	objects, channels := sampleOwnedRegistryGornesh(i)
	if objects+channels > ownedGCRegistryCap {
		t.Fatalf("owned registries not reclaimed after Eval: objects=%d channels=%d", objects, channels)
	}
}

// TestGorneshOwnedChannelRegistryBoundedUnderChanChurn pins the F1c bound:
// channel churn far beyond ownedGCRegistryCap inside one Eval must keep the
// channel registry bounded via the incremental sweep.
func TestGorneshOwnedChannelRegistryBoundedUnderChanChurn(t *testing.T) {
	i := New(Options{})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var maxSeen int
	wg.Add(1)
	go func() {
		defer wg.Done()
		maxSeen = watchOwnedRegistryGornesh(i, stop)
	}()
	done := make(chan error, 1)
	go func() {
		_, err := i.Eval(`
for i := 0; i < 300000; i++ {
	c := make(chan int, 1)
	_ = c
}
0`)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			close(stop)
			t.Fatalf("channel churn loop failed: %v", err)
		}
	case <-time.After(240 * time.Second):
		close(stop)
		t.Fatal("channel churn loop timed out")
	}
	close(stop)
	wg.Wait()
	if maxSeen > 2*ownedGCRegistryCap {
		t.Fatalf("owned channel registry grew unbounded: max=%d cap=%d", maxSeen, ownedGCRegistryCap)
	}
	objects, channels := sampleOwnedRegistryGornesh(i)
	if objects+channels > ownedGCRegistryCap {
		t.Fatalf("owned registries not reclaimed after Eval: objects=%d channels=%d", objects, channels)
	}
}

// TestGorneshIsolationAfterPreCancelAllocLoop mirrors
// TestGorneshDetachedRootIsolatesCanceledDeferredAggregateWrites with a long
// allocation loop inside the canceled call before the blocking host call. The
// loop forces several incremental sweeps mid-loop; the sweep must not evict
// anything the detach needs, so the zombie's deferred writes still must not
// reach the new root's globals.
func TestGorneshIsolationAfterPreCancelAllocLoop(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	deferredFinished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"ownedgcisolation/ownedgcisolation": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
		"DeferredFinished": reflect.ValueOf(func() { close(deferredFinished) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "ownedgcisolation"
var preCancelMapGornesh = map[string]int{"value": 1}
var preCancelSliceGornesh = []int{1}
var preCancelValueGornesh = 1
var preCancelPointerGornesh = &preCancelValueGornesh
func preCancelMutatorGornesh() {
	defer ownedgcisolation.DeferredFinished()
	defer func() {
		preCancelMapGornesh["value"] = 7
		preCancelSliceGornesh[0] = 7
		*preCancelPointerGornesh = 7
	}()
	for i := 0; i < 200000; i++ {
		junk := map[int]int{1: 1}
		_ = junk
	}
	ownedgcisolation.Block()
}`); err != nil {
		t.Fatalf("define pre-cancel allocation mutator: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `preCancelMutatorGornesh()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(240 * time.Second):
		cancel()
		t.Fatal("canceled mutation did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled mutation returned nil error")
	}
	value, err := i.Eval(`
preCancelMapGornesh["value"] = 99
preCancelSliceGornesh[0] = 99
*preCancelPointerGornesh = 99
preCancelMapGornesh["value"] + preCancelSliceGornesh[0] + *preCancelPointerGornesh`)
	if err != nil || value.Interface() != 297 {
		t.Fatalf("write new detached graph: value=%v err=%v", value, err)
	}
	close(release)
	select {
	case <-deferredFinished:
	case <-time.After(30 * time.Second):
		t.Fatal("canceled deferred mutation did not finish")
	}
	waitForFuncSweepGornesh(i)
	value, err = i.Eval(`preCancelMapGornesh["value"] + preCancelSliceGornesh[0] + *preCancelPointerGornesh`)
	if err != nil || value.Interface() != 297 {
		t.Fatalf("old deferred write reached new graph: value=%v err=%v", value, err)
	}
}

// TestGorneshSweepKeepsLiveCaptures runs allocation pressure through a host
// driver repeatedly invoking a closure that captures a map, across several
// Evals so the incremental sweep fires while the capture is live. The
// captured map must stay registered (the sweep keeps capture cells) and all
// increments must land.
func TestGorneshSweepKeepsLiveCaptures(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"ownedgcdrive/ownedgcdrive": {
		"Drive": reflect.ValueOf(func(n int, fn func() int) int {
			s := 0
			for k := 0; k < n; k++ {
				s = fn()
			}
			return s
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "ownedgcdrive"
func makeCounterGornesh() func() int {
	total := map[string]int{"n": 0}
	return func() int {
		for j := 0; j < 400; j++ {
			junk := map[int]int{1: 1}
			_ = junk
		}
		total["n"] = total["n"] + 1
		return total["n"]
	}
}
counterGornesh := makeCounterGornesh()
`); err != nil {
		t.Fatalf("define counting closure: %v", err)
	}

	// Locate the captured map in the ownership registry through the closure's
	// metadata, without publishing the map to the host.
	res, err := i.Eval(`counterGornesh`)
	if err != nil {
		t.Fatalf("read counter closure: %v", err)
	}
	key, ok := funcvalKeyOf(unwrapOwnedValue(res))
	if !ok {
		t.Fatal("counter closure has no canonical func value")
	}
	i.funcMu.RLock()
	meta, tracked := i.funcMeta[key]
	if !tracked || meta.group == nil {
		i.funcMu.RUnlock()
		t.Fatal("counter closure metadata missing")
	}
	var capturePtr uintptr
	for _, capture := range meta.group.captures {
		if capture.frame == nil || capture.index < 0 || capture.index >= len(capture.frame.data) {
			continue
		}
		capture.frame.mutex.RLock()
		value := unwrapOwnedValue(capture.frame.data[capture.index])
		if value.IsValid() && value.Kind() == reflect.Map {
			capturePtr = value.Pointer()
		}
		capture.frame.mutex.RUnlock()
	}
	i.funcMu.RUnlock()
	if capturePtr == 0 {
		t.Fatal("no captured map found in closure metadata")
	}

	want := 0
	for round := 0; round < 3; round++ {
		value, err := i.Eval(`ownedgcdrive.Drive(60, counterGornesh)`)
		if err != nil {
			t.Fatalf("drive round %d failed: %v", round, err)
		}
		want += 60
		if got := value.Interface().(int); got != want {
			t.Fatalf("drive round %d: captured map lost writes: got %d, want %d", round, got, want)
		}
	}
	// Three rounds allocate 72k junk maps in total, so the incremental sweep
	// ran while the capture was only reachable through its capture cell.
	if want <= ownedGCRegistryCap/400 {
		t.Fatalf("test did not generate enough pressure: %d allocations", want*400)
	}
	i.funcMu.RLock()
	_, stillTracked := i.ownedObjects[objectKey{kind: reflect.Map, ptr: capturePtr}]
	i.funcMu.RUnlock()
	if !stillTracked {
		t.Fatalf("sweep evicted live captured map at %x", capturePtr)
	}
}

// zombieGatesGornesh is the mutex-guarded host-call gate set for the zombie
// fence test: the interpreted worker blocks in Block, and the deferred drain
// phase announces its start and completion. Guarded because the host closures
// run on worker goroutines while the test goroutine re-arms the channels.
type zombieGatesGornesh struct {
	mu            sync.Mutex
	entered       chan struct{}
	release       chan struct{}
	drainStarted  chan struct{}
	drainFinished chan struct{}
}

func (g *zombieGatesGornesh) block() {
	g.mu.Lock()
	entered, release := g.entered, g.release
	g.mu.Unlock()
	close(entered)
	<-release
}

func (g *zombieGatesGornesh) drainStartedFunc() {
	g.mu.Lock()
	drainStarted := g.drainStarted
	g.mu.Unlock()
	close(drainStarted)
}

func (g *zombieGatesGornesh) drainFinishedFunc() {
	g.mu.Lock()
	drainFinished := g.drainFinished
	g.mu.Unlock()
	close(drainFinished)
}

func (g *zombieGatesGornesh) wake() {
	g.mu.Lock()
	release := g.release
	g.mu.Unlock()
	close(release)
}

func (g *zombieGatesGornesh) rearm() {
	g.mu.Lock()
	g.entered = make(chan struct{})
	g.release = make(chan struct{})
	g.drainStarted = make(chan struct{})
	g.drainFinished = make(chan struct{})
	g.mu.Unlock()
}

func (g *zombieGatesGornesh) waitEntered(t *testing.T) {
	t.Helper()
	g.mu.Lock()
	entered := g.entered
	g.mu.Unlock()
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		g.wake()
		t.Fatal("canceled worker did not block")
	}
}

func (g *zombieGatesGornesh) waitDrainStarted(t *testing.T, iteration int) {
	t.Helper()
	g.mu.Lock()
	drainStarted := g.drainStarted
	g.mu.Unlock()
	select {
	case <-drainStarted:
	case <-time.After(30 * time.Second):
		t.Fatalf("iteration %d: zombie drain did not start", iteration)
	}
}

func (g *zombieGatesGornesh) waitDrainFinished(t *testing.T, iteration int) {
	t.Helper()
	g.mu.Lock()
	drainFinished := g.drainFinished
	g.mu.Unlock()
	select {
	case <-drainFinished:
	case <-time.After(240 * time.Second):
		t.Fatalf("iteration %d: zombie drain did not finish", iteration)
	}
}

// TestGorneshSweepUnderZombieFence reuses the canceled-worker harness: the
// registry crosses the cap while a canceled worker drains an allocating
// defer. Sweeps must stay pending without deadlock while the drain holds or
// waits on the exclusive fence, resume afterwards, and leave follow-up Evals
// working.
func TestGorneshSweepUnderZombieFence(t *testing.T) {
	gates := &zombieGatesGornesh{entered: make(chan struct{}), release: make(chan struct{}), drainStarted: make(chan struct{}), drainFinished: make(chan struct{})}
	i := New(Options{})
	if err := i.Use(Exports{"gorneshzombiegc/gorneshzombiegc": {
		"Block":         reflect.ValueOf(gates.block),
		"DrainStarted":  reflect.ValueOf(gates.drainStartedFunc),
		"DrainFinished": reflect.ValueOf(gates.drainFinishedFunc),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "gorneshzombiegc"
func zombieAllocGornesh(n int) {
	for i := 0; i < n; i++ {
		m := map[int]int{1: 1}
		_ = m
	}
}
func zombiePlainGornesh() {
	defer func() {
		gorneshzombiegc.DrainStarted()
		zombieAllocGornesh(100000)
		gorneshzombiegc.DrainFinished()
	}()
	gorneshzombiegc.Block()
}`); err != nil {
		t.Fatalf("define zombie allocator: %v", err)
	}

	for k := 0; k < 3; k++ {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := i.EvalWithContext(ctx, `zombiePlainGornesh()`)
			done <- err
		}()
		gates.waitEntered(t)
		cancel() // API returns; worker keeps unwinding its defers
		if err := <-done; err == nil {
			t.Fatalf("iteration %d: canceled eval returned nil error", k)
		}
		gates.wake() // wake the worker so the allocating drain starts
		gates.waitDrainStarted(t, k)
		// The drain allocates 100k maps: the registry crosses the cap while
		// frameDrains pins the sweep, so arming must be observable without a
		// sweep consuming it under the drain.
		crossed := false
		for tick := 0; tick < 3000; tick++ {
			objects, _ := sampleOwnedRegistryGornesh(i)
			if objects >= ownedGCRegistryCap {
				crossed = true
				break
			}
			time.Sleep(time.Millisecond)
		}
		if !crossed {
			t.Fatalf("iteration %d: registry never crossed the cap during the drain", k)
		}
		// Follow-up Evals overlap or trail the drain and must neither deadlock
		// nor lose data.
		for followUp := 0; followUp < 10; followUp++ {
			if _, err := i.Eval(`1 + 1`); err != nil {
				t.Fatalf("iteration %d: follow-up eval %d failed: %v", k, followUp, err)
			}
		}
		gates.waitDrainFinished(t, k)
		gates.rearm()
	}
	waitForFuncSweepGornesh(i)
	if _, err := i.Eval(`1 + 1`); err != nil {
		t.Fatalf("post-drain eval failed: %v", err)
	}
	objects, channels := sampleOwnedRegistryGornesh(i)
	if objects+channels > 2*ownedGCRegistryCap {
		t.Fatalf("registries unbounded after zombie drains: objects=%d channels=%d", objects, channels)
	}
}

// TestGorneshDirectFuncsCacheEvictedByOwnedGCSweep pins the directFuncs
// activation cache bound: entries whose root is no longer live, or whose
// source and cloned value both lost their metadata, must be evicted by the
// incremental sweep, while a live activation (metadata reachable from the
// durable root) survives and keeps working.
func TestGorneshDirectFuncsCacheEvictedByOwnedGCSweep(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func makeCounterDirectGornesh() func() int {
	n := 0
	return func() int { n++; return n }
}
var keptCounterDirectGornesh = makeCounterDirectGornesh()
`); err != nil {
		t.Fatalf("define retained counter: %v", err)
	}

	value, err := i.Eval(`keptCounterDirectGornesh`)
	if err != nil {
		t.Fatalf("read retained counter: %v", err)
	}
	keptKey, ok := funcvalKeyOf(unwrapOwnedValue(value))
	if !ok {
		t.Fatal("retained counter is not a func value")
	}
	i.funcMu.RLock()
	_, tracked := i.funcMeta[keptKey]
	i.funcMu.RUnlock()
	if !tracked {
		t.Fatal("retained counter metadata missing after eval")
	}

	deadFunc := reflect.ValueOf(func() int { return 42 })
	deadKey, ok := funcvalKeyOf(deadFunc)
	if !ok {
		t.Fatal("dead func is not a func value")
	}
	deadRoot := &frame{}
	deadRoot.root = deadRoot

	liveEntry := directFuncActivationKey{source: keptKey, root: i.frame}
	deadEndpointsEntry := directFuncActivationKey{source: deadKey, root: i.frame}
	deadRootEntry := directFuncActivationKey{source: keptKey, root: deadRoot}

	i.funcMu.Lock()
	i.directFuncs[liveEntry] = newDirectFuncActivation(value)
	i.directFuncs[deadEndpointsEntry] = newDirectFuncActivation(deadFunc)
	i.directFuncs[deadRootEntry] = newDirectFuncActivation(value)
	i.funcMu.Unlock()

	i.ownedGCPending.Store(true)
	i.maybeRunOwnedGCSweep()

	i.funcMu.RLock()
	_, liveKept := i.directFuncs[liveEntry]
	_, deadEndpointsKept := i.directFuncs[deadEndpointsEntry]
	_, deadRootKept := i.directFuncs[deadRootEntry]
	size := len(i.directFuncs)
	i.funcMu.RUnlock()

	if !liveKept {
		t.Fatal("live directFuncs activation was evicted")
	}
	if deadEndpointsKept || deadRootKept {
		t.Fatal("dead directFuncs entries survived the sweep")
	}
	if size != 1 {
		t.Fatalf("directFuncs size after sweep: %d, want 1", size)
	}

	if _, err := i.Eval(`keptCounterDirectGornesh()`); err != nil {
		t.Fatalf("call retained counter after sweep: %v", err)
	}
}
