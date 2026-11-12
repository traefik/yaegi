package interp_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
	"unsafe"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	stdlibunsafe "github.com/traefik/yaegi/stdlib/unsafe"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

type callbackBundleGornesh struct {
	Run func()
}

type callbackNodeGornesh struct {
	Run  func()
	Next *callbackNodeGornesh
}

type blockingDirectoryFS struct {
	fs.FS
	holdEntered chan struct{}
	holdRelease chan struct{}
	appStat     chan struct{}
	holdOnce    sync.Once
	appOnce     sync.Once
}

func (b *blockingDirectoryFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "gopath/src/hold" {
		b.holdOnce.Do(func() {
			close(b.holdEntered)
			<-b.holdRelease
		})
	}
	return fs.ReadDir(b.FS, name)
}

func (b *blockingDirectoryFS) Stat(name string) (fs.FileInfo, error) {
	info, err := fs.Stat(b.FS, name)
	if name == "app" {
		b.appOnce.Do(func() { close(b.appStat) })
	}
	return info, err
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *synchronizedBuffer) Reset() {
	b.mu.Lock()
	b.b.Reset()
	b.mu.Unlock()
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestContextSuccessKeepsPublishedFunctionsCallable(t *testing.T) {
	callAfterReturn := func(t *testing.T, cancel context.CancelFunc, value reflect.Value, want int) {
		t.Helper()
		cancel()
		callback, ok := value.Interface().(func() int)
		if !ok {
			t.Fatalf("published value has type %T, want func() int", value.Interface())
		}
		if got := callback(); got != want {
			t.Fatalf("published callback returned %d, want %d", got, want)
		}
	}

	t.Run("EvalWithContext result", func(t *testing.T) {
		i := interp.New(interp.Options{})
		ctx, cancel := context.WithCancel(context.Background())
		value, err := i.EvalWithContext(ctx, `func() int { return 41 }`)
		if err != nil {
			t.Fatal(err)
		}
		callAfterReturn(t, cancel, value, 41)
	})

	t.Run("ExecuteWithContext result", func(t *testing.T) {
		i := interp.New(interp.Options{})
		program, err := i.Compile(`func() int { return 42 }`)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		value, err := i.ExecuteWithContext(ctx, program)
		if err != nil {
			t.Fatal(err)
		}
		callAfterReturn(t, cancel, value, 42)
	})

	t.Run("EvalPathWithContext file symbol", func(t *testing.T) {
		i := interp.New(interp.Options{SourcecodeFilesystem: fstest.MapFS{
			"one.go": {Data: []byte(`package main; var Callback = func() int { return 43 }`)},
		}})
		ctx, cancel := context.WithCancel(context.Background())
		if _, err := i.EvalPathWithContext(ctx, "one.go"); err != nil {
			t.Fatal(err)
		}
		value := i.Globals()["Callback"]
		callAfterReturn(t, cancel, value, 43)
	})

	t.Run("EvalPathWithContext directory symbol", func(t *testing.T) {
		i := interp.New(interp.Options{
			GoPath: "gopath",
			SourcecodeFilesystem: fstest.MapFS{
				"gopath/src/app/main.go": {Data: []byte(`package main
					var Callback = func() int { return 44 }
					func main() {}`)},
			},
		})
		ctx, cancel := context.WithCancel(context.Background())
		if _, err := i.EvalPathWithContext(ctx, "app"); err != nil {
			t.Fatal(err)
		}
		value := i.Symbols("app")["app"]["Callback"]
		callAfterReturn(t, cancel, value, 44)
	})
}

func TestEvalWithContextSerializesLiveOwnersAndReleasesOnCancel(t *testing.T) {
	aEntered := make(chan struct{})
	aRelease := make(chan struct{})
	aReturned := make(chan struct{})
	aAfter := make(chan struct{})
	bEntered := make(chan struct{})
	bRelease := make(chan struct{})
	bReturned := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"owner/owner": {
		"BlockA": reflect.ValueOf(func() { close(aEntered); <-aRelease; close(aReturned) }),
		"AfterA": reflect.ValueOf(func() { close(aAfter) }),
		"BlockB": reflect.ValueOf(func() { close(bEntered); <-bRelease; close(bReturned) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "owner"; var ownerValueGornesh int`); err != nil {
		t.Fatal(err)
	}

	ctxA, cancelA := context.WithCancel(context.Background())
	aResult := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctxA, `owner.BlockA(); ownerValueGornesh = 1; owner.AfterA()`)
		aResult <- err
	}()
	select {
	case <-aEntered:
	case <-time.After(time.Second):
		t.Fatal("first evaluation did not enter native call")
	}

	ctxB, cancelB := context.WithCancel(context.Background())
	bResult := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctxB, `owner.BlockB()`)
		bResult <- err
	}()
	select {
	case <-bEntered:
		t.Fatal("overlapping evaluation entered before the live owner was released")
	case <-time.After(20 * time.Millisecond):
	}

	cancelA()
	if err := <-aResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("first evaluation error = %v, want context canceled", err)
	}
	select {
	case <-bEntered:
	case <-time.After(time.Second):
		t.Fatal("second evaluation did not proceed after caller cancellation")
	}

	close(aRelease)
	<-aReturned
	select {
	case <-aAfter:
		t.Fatal("canceled first evaluation resumed after its native call")
	case <-time.After(20 * time.Millisecond):
	}

	cancelB()
	if err := <-bResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("second evaluation error = %v, want context canceled", err)
	}
	value, err := i.Eval(`ownerValueGornesh`)
	if err != nil || value.Interface() != 0 {
		t.Fatalf("canceled owner published value: value=%v err=%v", value, err)
	}
	close(bRelease)
	<-bReturned
}

func TestPlainEvalWaitsForLiveContextOwner(t *testing.T) {
	entered := make(chan struct{})
	releaseNative := make(chan struct{})
	nativeReturned := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"plainwait/plainwait": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-releaseNative
			close(nativeReturned)
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "plainwait"; var plainWaitValueGornesh int`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	contextResult := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `plainwait.Block(); plainWaitValueGornesh = 1`)
		contextResult <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("context evaluation did not enter native call")
	}

	plainResult := make(chan error, 1)
	go func() {
		_, err := i.Eval(`plainWaitValueGornesh = 2`)
		plainResult <- err
	}()
	select {
	case err := <-plainResult:
		t.Fatalf("unrelated plain Eval bypassed the live owner: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	cancel()
	if err := <-contextResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("context evaluation error = %v, want context canceled", err)
	}
	select {
	case err := <-plainResult:
		if err != nil {
			t.Fatalf("plain Eval after owner cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("plain Eval did not proceed after owner cancellation")
	}
	if got := i.Globals()["plainWaitValueGornesh"].Interface(); got != 2 {
		t.Fatalf("plain Eval value = %v, want 2", got)
	}
	close(releaseNative)
	<-nativeReturned
	if got := i.Globals()["plainWaitValueGornesh"].Interface(); got != 2 {
		t.Fatalf("canceled context Eval overwrote later value: %v", got)
	}
}

func TestEvalWithContextDoesNotResumeCanceledExecution(t *testing.T) {
	var out synchronizedBuffer
	i := interp.New(interp.Options{Stdout: &out})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "time"`); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`var kept = 41`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := i.EvalWithContext(ctx, `time.Sleep(200*time.Millisecond); println("late-output")`); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled Eval error = %v, want context deadline exceeded", err)
	}

	out.Reset()
	v, err := i.EvalWithContext(context.Background(), `for start := time.Now(); time.Since(start) < 300*time.Millisecond; {}; kept + 1`)
	if err != nil {
		t.Fatalf("Eval after cancellation: %v", err)
	}
	if got := v.Interface(); got != 42 {
		t.Fatalf("persistent state after cancellation = %v, want 42", got)
	}
	if got := out.String(); strings.Contains(got, "late-output") {
		t.Fatalf("canceled execution resumed during later Eval: %q", got)
	}
}

func TestDetachedRootPreservesTypedAndUnsafePointerAliases(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(stdlibunsafe.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(interp.Exports{"unsafedetach/unsafedetach": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "unsafe"
import "unsafedetach"

type unsafePointerAliasBoxGornesh struct {
	Prefix int
	Values [3]int
}

var unsafePointerAliasBoxValueGornesh = new(unsafePointerAliasBoxGornesh)
var unsafePointerAliasTypedGornesh = &unsafePointerAliasBoxValueGornesh.Values[1]
var unsafePointerAliasRawGornesh = unsafe.Pointer(unsafePointerAliasTypedGornesh)
var unsafeSliceAliasGornesh = []int{1, 2, 3}
var unsafeSliceAliasTypedGornesh = &unsafeSliceAliasGornesh[1]
var unsafeSliceAliasRawGornesh = unsafe.Pointer(unsafeSliceAliasTypedGornesh)
`); err != nil {
		t.Fatalf("define typed/unsafe aliases: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `unsafedetach.Block()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("unsafe alias detach blocker was not entered")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		close(release)
		t.Fatalf("unsafe alias canceled Eval error = %v, want context canceled", err)
	}

	value, err := i.Eval(`
*unsafePointerAliasTypedGornesh = 41
*(*int)(unsafePointerAliasRawGornesh) = 42
*unsafeSliceAliasTypedGornesh = 51
*(*int)(unsafeSliceAliasRawGornesh) = 52
aliasMaskGornesh := 0
if unsafePointerAliasBoxValueGornesh.Values[1] == 42 { aliasMaskGornesh |= 1 }
if *unsafePointerAliasTypedGornesh == 42 { aliasMaskGornesh |= 2 }
if unsafeSliceAliasGornesh[1] == 52 { aliasMaskGornesh |= 4 }
if *unsafeSliceAliasTypedGornesh == 52 { aliasMaskGornesh |= 8 }
aliasMaskGornesh`)
	close(release)
	if err != nil || value.Interface() != 15 {
		t.Fatalf("typed/unsafe aliases split across detach: mask=%v want 15 err=%v", value, err)
	}
}

func TestDetachedRootPreservesFirstFieldAliasIndependentOfGlobalOrder(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(stdlibunsafe.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(interp.Exports{"fieldorderdetach/fieldorderdetach": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "unsafe"
import "fieldorderdetach"

type firstFieldAliasContainerGornesh struct {
	X int
	Y int
}

var firstFieldAliasGornesh *int
var firstFieldAliasBoxGornesh *firstFieldAliasContainerGornesh
var firstFieldAliasRawGornesh unsafe.Pointer

firstFieldAliasBoxGornesh = &firstFieldAliasContainerGornesh{X: 1, Y: 2}
firstFieldAliasGornesh = &firstFieldAliasBoxGornesh.X
firstFieldAliasRawGornesh = unsafe.Pointer(firstFieldAliasGornesh)
`); err != nil {
		t.Fatalf("define first-field aliases: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `fieldorderdetach.Block()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("first-field alias detach blocker was not entered")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		close(release)
		t.Fatalf("first-field alias canceled Eval error = %v, want context canceled", err)
	}

	value, err := i.Eval(`
*firstFieldAliasGornesh = 3
firstTypedMutationGornesh := firstFieldAliasBoxGornesh.X == 3 && *(*int)(firstFieldAliasRawGornesh) == 3
firstFieldAliasBoxGornesh.X = 4
firstBoxMutationGornesh := *firstFieldAliasGornesh == 4 && *(*int)(firstFieldAliasRawGornesh) == 4
*(*int)(firstFieldAliasRawGornesh) = 5
firstTypedMutationGornesh && firstBoxMutationGornesh &&
	firstFieldAliasBoxGornesh.X == 5 && *firstFieldAliasGornesh == 5`)
	close(release)
	if err != nil || value.Interface() != true {
		t.Fatalf("first-field aliases split across detach: value=%v err=%v", value, err)
	}
}

func TestDetachedRootPreservesRawOnlyBufferedChannelAllocation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(stdlibunsafe.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(interp.Exports{"rawchanneldetach/rawchanneldetach": {
		"Block": reflect.ValueOf(func() { close(entered); <-release }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "unsafe"
import "rawchanneldetach"
var rawOnlyChannelGornesh = make(chan unsafe.Pointer, 1)
func sendRawOnlyPointerGornesh() {
	pointer := new(int)
	*pointer = 41
	rawOnlyChannelGornesh <- unsafe.Pointer(pointer)
}
sendRawOnlyPointerGornesh()`); err != nil {
		t.Fatalf("send raw-only pointer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `rawchanneldetach.Block()`)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		close(release)
		t.Fatalf("raw-only channel canceled Eval error = %v, want context canceled", err)
	}
	value, err := i.Eval(`raw := <-rawOnlyChannelGornesh; *(*int)(raw) = 42; *(*int)(raw)`)
	close(release)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("receive raw-only allocation after detach: value=%v err=%v", value, err)
	}
}

func TestDetachedRootKeepsAPIReturnedUnsafePointerHostShared(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(stdlibunsafe.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(interp.Exports{"rawhostdetach/rawhostdetach": {
		"Block": reflect.ValueOf(func() { close(entered); <-release }),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "unsafe"
import "rawhostdetach"
var apiRawPointerGornesh = new(int)
*apiRawPointerGornesh = 1
unsafe.Pointer(apiRawPointerGornesh)`)
	if err != nil {
		t.Fatalf("return raw pointer: %v", err)
	}
	raw := value.Interface().(unsafe.Pointer)
	*(*int)(raw) = 7

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `rawhostdetach.Block()`)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		close(release)
		t.Fatalf("raw host pointer canceled Eval error = %v, want context canceled", err)
	}
	value, err = i.Eval(`*apiRawPointerGornesh = *apiRawPointerGornesh + 1; *apiRawPointerGornesh`)
	close(release)
	if err != nil || value.Interface() != 8 || *(*int)(raw) != 8 {
		t.Fatalf("API-returned raw pointer split after detach: value=%v host=%d err=%v", value, *(*int)(raw), err)
	}
}

func TestDetachedRootPreservesRawOnlyRecoveredPanicAllocation(t *testing.T) {
	backgroundStarted := make(chan struct{})
	allowOldMutation := make(chan struct{})
	backgroundFinished := make(chan struct{})
	detachEntered := make(chan struct{})
	detachRelease := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(stdlibunsafe.Symbols); err != nil {
		t.Fatal(err)
	}
	if err := i.Use(interp.Exports{"rawpanicdetach/rawpanicdetach": {
		"BackgroundStarted":  reflect.ValueOf(func() { close(backgroundStarted) }),
		"WaitOldMutation":    reflect.ValueOf(func() { <-allowOldMutation }),
		"BackgroundFinished": reflect.ValueOf(func() { close(backgroundFinished) }),
		"BlockDetach":        reflect.ValueOf(func() { close(detachEntered); <-detachRelease }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "unsafe"
import "rawpanicdetach"
var recoveredRawPanicGornesh unsafe.Pointer
func recoverRawOnlyPanicGornesh() {
	defer func() {
		recoveredRawPanicGornesh = recover().(unsafe.Pointer)
		go func(raw unsafe.Pointer) {
			rawpanicdetach.BackgroundStarted()
			rawpanicdetach.WaitOldMutation()
			*(*int)(raw) = 99
			rawpanicdetach.BackgroundFinished()
		}(recoveredRawPanicGornesh)
	}()
	raw := unsafe.Pointer(new(int))
	*(*int)(raw) = 41
	panic(raw)
}
recoverRawOnlyPanicGornesh()
0`); err != nil {
		t.Fatalf("recover raw-only panic: %v", err)
	}
	<-backgroundStarted

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `rawpanicdetach.BlockDetach()`)
		done <- err
	}()
	<-detachEntered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		close(detachRelease)
		close(allowOldMutation)
		t.Fatalf("raw panic canceled Eval error = %v, want context canceled", err)
	}
	value, err := i.Eval(`*(*int)(recoveredRawPanicGornesh) = 42; *(*int)(recoveredRawPanicGornesh)`)
	if err != nil || value.Interface() != 42 {
		close(detachRelease)
		close(allowOldMutation)
		t.Fatalf("mutate adopted raw panic allocation: value=%v err=%v", value, err)
	}
	close(allowOldMutation)
	<-backgroundFinished
	value, err = i.Eval(`*(*int)(recoveredRawPanicGornesh)`)
	close(detachRelease)
	if err != nil || value.Interface() != 42 {
		t.Fatalf("old-root raw panic mutation reached detached allocation: value=%v err=%v", value, err)
	}
}

func TestDirectCallbackLineageAliasesShareCarrierAcrossDetachGenerations(t *testing.T) {
	var raw, prior func() int
	var entered, release chan struct{}
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"lineage/lineage": {
		"GetRaw":   reflect.ValueOf(func() func() int { return raw }),
		"GetPrior": reflect.ValueOf(func() func() int { return prior }),
		"Block": reflect.ValueOf(func() {
			currentEntered, currentRelease := entered, release
			close(currentEntered)
			<-currentRelease
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "lineage"
var DirectLineageRawGornesh func() int
var DirectLineagePriorGornesh func() int
func makeDirectLineageCounterGornesh() func() int {
	count := 0
	return func() int { count++; return count }
}
makeDirectLineageCounterGornesh()`)
	if err != nil {
		t.Fatalf("create raw direct-lineage callback: %v", err)
	}
	raw = value.Interface().(func() int)
	detach := func() {
		t.Helper()
		entered, release = make(chan struct{}), make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := i.EvalWithContext(ctx, `lineage.Block()`)
			done <- err
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			cancel()
			t.Fatal("direct-lineage detach blocker was not entered")
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			close(release)
			t.Fatalf("direct-lineage canceled Eval error = %v", err)
		}
	}

	detach()
	value, err = i.Eval(`DirectLineageRawGornesh = lineage.GetRaw(); DirectLineageRawGornesh()`)
	close(release)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("first direct-lineage activation: value=%v err=%v", value, err)
	}
	prior = i.Globals()["DirectLineageRawGornesh"].Interface().(func() int)

	detach()
	value, err = i.Eval(`
DirectLineageRawGornesh = lineage.GetRaw()
DirectLineagePriorGornesh = lineage.GetPrior()
DirectLineageRawGornesh()*10 + DirectLineagePriorGornesh()`)
	close(release)
	if err != nil || value.Interface() != 23 {
		t.Fatalf("direct-lineage aliases split across generations: value=%v err=%v, want 23", value, err)
	}
}

func TestEvalWithContextCanceledNativeCallCannotPublish(t *testing.T) {
	var out synchronizedBuffer
	entered := make(chan struct{})
	release := make(chan struct{})
	cleanup := make(chan struct{})
	var enterOnce sync.Once
	var cleanupOnce sync.Once
	i := interp.New(interp.Options{Stdout: &out})
	if err := i.Use(interp.Exports{"block/block": {
		"Wait": reflect.ValueOf(func() int {
			enterOnce.Do(func() { close(entered) })
			<-release
			return 7
		}),
		"Cleanup": reflect.ValueOf(func() {
			cleanupOnce.Do(func() { close(cleanup) })
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "block"`); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`var published int; func saved(v int) int { return v + 1 }; var closure = func(v int) int { return v + 2 }`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `func() {
			defer func() { block.Cleanup() }()
			value := block.Wait()
			published = value
			println("late-body")
		}()`)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("native call was not entered")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Eval error = %v, want context canceled", err)
	}

	out.Reset()
	for _, check := range []struct {
		src  string
		want int
	}{
		{`published`, 0},
		{`saved(10)`, 11},
		{`closure(20)`, 22},
	} {
		v, err := i.Eval(check.src)
		if err != nil || v.Interface() != check.want {
			t.Fatalf("state %q after blocked cancellation: value=%v err=%v, want %d", check.src, v, err, check.want)
		}
	}
	if _, err := i.Eval(`var declaredAfterCancel = 42`); err != nil {
		t.Fatalf("new declaration after cancellation: %v", err)
	}
	if v, err := i.Eval(`published = 99; declaredAfterCancel`); err != nil || v.Interface() != 42 {
		t.Fatalf("new declaration after cancellation: value=%v err=%v", v, err)
	}
	close(release)
	select {
	case <-cleanup:
	case <-time.After(time.Second):
		t.Fatal("deferred cleanup did not run after blocked call returned")
	}
	if got := out.String(); got != "" {
		t.Fatalf("canceled execution published output after release: %q", got)
	}
	v, err := i.Eval(`published`)
	if err != nil || v.Interface() != 99 {
		t.Fatalf("canceled execution published state: value=%v err=%v", v, err)
	}
}

func TestCanceledNestedInterpretedCallCannotPublishReturn(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var enterOnce, finishOnce sync.Once
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"block/block": {
		"Wait": reflect.ValueOf(func() int {
			enterOnce.Do(func() { close(entered) })
			<-release
			return 7
		}),
		"Finished": reflect.ValueOf(func() { finishOnce.Do(func() { close(finished) }) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "block"`); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`var nestedPublished = 99; func nestedSlow() int { return block.Wait() }`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `func() {
			defer block.Finished()
			nestedPublished = nestedSlow()
		}()`)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("nested interpreted call did not enter native call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Eval error = %v, want context canceled", err)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("canceled nested interpreted call did not unwind")
	}
	if v, err := i.Eval(`nestedPublished`); err != nil || v.Interface() != 99 {
		t.Fatalf("canceled nested interpreted call published return: value=%v err=%v", v, err)
	}
}

func TestCanceledDeferredNativeCallbackStillRunsCleanup(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	runCalled := make(chan struct{})
	cleanup := make(chan struct{})
	var enterOnce, runOnce, cleanupOnce sync.Once
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"block/block": {
		"Wait": reflect.ValueOf(func() {
			enterOnce.Do(func() { close(entered) })
			<-release
		}),
		"Run": reflect.ValueOf(func(callback func()) {
			runOnce.Do(func() { close(runCalled) })
			callback()
		}),
		"Cleanup": reflect.ValueOf(func() { cleanupOnce.Do(func() { close(cleanup) }) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "block"`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `func() {
			defer block.Run(func() { block.Cleanup() })
			block.Wait()
		}()`)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("deferred callback test did not enter native call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Eval error = %v, want context canceled", err)
	}
	close(release)
	select {
	case <-runCalled:
	case <-time.After(time.Second):
		t.Fatal("deferred native function did not run after cancellation")
	}
	select {
	case <-cleanup:
	case <-time.After(time.Second):
		t.Fatal("deferred native callback did not run cleanup after cancellation")
	}
}

func TestEvalWithContextCanceledGlobalInitializerSkipsMain(t *testing.T) {
	var out synchronizedBuffer
	entered := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{Stdout: &out})
	if err := i.Use(interp.Exports{"block/block": {
		"Wait": reflect.ValueOf(func() int {
			close(entered)
			<-release
			return 9
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "block"`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `package main
			var slow = block.Wait()
			func main() { println("late-main") }`)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("global initializer was not entered")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Eval error = %v, want context canceled", err)
	}
	out.Reset()
	if v, err := i.Eval(`40 + 2`); err != nil || v.Interface() != 42 {
		t.Fatalf("Eval after canceled initializer: value=%v err=%v", v, err)
	}
	close(release)
	time.Sleep(50 * time.Millisecond)
	if got := out.String(); got != "" {
		t.Fatalf("canceled initializer ran later phases: %q", got)
	}
}

func TestEvalWithContextAlreadyCanceledDoesNotExecute(t *testing.T) {
	var calls int
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"counter/counter": {
		"Increment": reflect.ValueOf(func() { calls++ }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "counter"`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := i.EvalWithContext(ctx, `counter.Increment()`); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Eval error = %v, want context canceled", err)
	}
	if calls != 0 {
		t.Fatalf("pre-canceled Eval executed source %d time(s)", calls)
	}
}

func TestCanceledCallbackCannotAttachToLaterEval(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var storedDirect, storedSlice, storedMap, storedStruct, storedPointer, storedCycle, storedDeep func()
	var cyclePreserved bool
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"callback/callback": {
		"Bundle":       reflect.ValueOf((*callbackBundleGornesh)(nil)),
		"Node":         reflect.ValueOf((*callbackNodeGornesh)(nil)),
		"Store":        reflect.ValueOf(func(fn func()) { storedDirect = fn }),
		"StoreSlice":   reflect.ValueOf(func(fns []func()) { storedSlice = fns[0] }),
		"StoreMap":     reflect.ValueOf(func(fns map[string]func()) { storedMap = fns["run"] }),
		"StoreBundle":  reflect.ValueOf(func(bundle callbackBundleGornesh) { storedStruct = bundle.Run }),
		"StorePointer": reflect.ValueOf(func(bundle *callbackBundleGornesh) { storedPointer = bundle.Run }),
		"StoreCycle": reflect.ValueOf(func(node *callbackNodeGornesh) {
			cyclePreserved = node.Next == node
			storedCycle = node.Next.Run
		}),
		"StoreDeep": reflect.ValueOf(func(node *callbackNodeGornesh) {
			for node != nil && node.Run == nil {
				node = node.Next
			}
			if node != nil {
				storedDeep = node.Run
			}
		}),
		"Wait": reflect.ValueOf(func() {
			close(entered)
			<-release
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "callback"`); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`var callbackRuns int; func invokeNormally() { callbackRuns++ }; var anonymousNormally = func() { callbackRuns++ }`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `
			callback.Store(func() { callbackRuns++ })
			callback.StoreSlice([]func(){func() { callbackRuns++ }})
			callback.StoreMap(map[string]func(){"run": func() { callbackRuns++ }})
			callback.StoreBundle(callback.Bundle{Run: func() { callbackRuns++ }})
			callback.StorePointer(&callback.Bundle{Run: func() { callbackRuns++ }})
			cycle := &callback.Node{Run: func() { callbackRuns++ }}
			cycle.Next = cycle
			callback.StoreCycle(cycle)
			deep := &callback.Node{}
			cursor := deep
			for i := 0; i < 16; i++ {
				cursor.Next = &callback.Node{}
				cursor = cursor.Next
			}
			cursor.Run = func() { callbackRuns++ }
			callback.StoreDeep(deep)
			callback.Wait()`)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback registration did not reach blocking call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Eval error = %v, want context canceled", err)
	}

	if v, err := i.Eval(`invokeNormally(); anonymousNormally(); callbackRuns`); err != nil || v.Interface() != 2 {
		t.Fatalf("stored interpreted functions did not inherit later Eval: value=%v err=%v", v, err)
	}
	if !cyclePreserved {
		t.Fatal("callback owner binding did not preserve pointer cycle")
	}
	for _, stored := range []func(){storedDirect, storedSlice, storedMap, storedStruct, storedPointer, storedCycle, storedDeep} {
		if stored == nil {
			t.Fatal("native host did not retain callback")
		}
		stored()
	}
	if v, err := i.Eval(`callbackRuns`); err != nil || v.Interface() != 2 {
		t.Fatalf("callback from canceled Eval attached to later owner: value=%v err=%v", v, err)
	}
	close(release)
}

func TestExecuteWithContextKeepsCanceledFrameDetached(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"block/block": {
		"Wait": reflect.ValueOf(func() int {
			close(entered)
			<-release
			return 7
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "block"`); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`var executePublished int`); err != nil {
		t.Fatal(err)
	}
	program, err := i.Compile(`executePublished = block.Wait()`)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.ExecuteWithContext(ctx, program)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("compiled program did not enter native call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Execute error = %v, want context canceled", err)
	}
	if _, err := i.Eval(`executePublished = 99`); err != nil {
		t.Fatal(err)
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
	if v, err := i.Eval(`executePublished`); err != nil || v.Interface() != 99 {
		t.Fatalf("canceled Execute published into later frame: value=%v err=%v", v, err)
	}
}

func TestEvalPathWithContextKeepsCanceledFrameDetached(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{SourcecodeFilesystem: fstest.MapFS{
		"slow.go": {Data: []byte(`package main
			import "block"
			var pathPublished int
			func main() { pathPublished = block.Wait() }
		`)},
	}})
	if err := i.Use(interp.Exports{"block/block": {
		"Wait": reflect.ValueOf(func() int {
			close(entered)
			<-release
			return 7
		}),
	}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalPathWithContext(ctx, "slow.go")
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("path evaluation did not enter native call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled EvalPath error = %v, want context canceled", err)
	}
	if _, err := i.Eval(`pathPublished = 99`); err != nil {
		t.Fatal(err)
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
	if v, err := i.Eval(`pathPublished`); err != nil || v.Interface() != 99 {
		t.Fatalf("canceled EvalPath published into later frame: value=%v err=%v", v, err)
	}
}

func TestEvalSourceDirectoryWithContextKeepsCanceledFrameDetached(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	mainCalled := make(chan struct{})
	i := interp.New(interp.Options{
		GoPath: "gopath",
		SourcecodeFilesystem: fstest.MapFS{
			"gopath/src/app/main.go": {Data: []byte(`package main
				import "block"
				var DirectoryPublished = block.Wait()
				func main() { block.MarkMain() }
			`)},
		},
	})
	if err := i.Use(interp.Exports{"block/block": {
		"Wait": reflect.ValueOf(func() int {
			close(entered)
			<-release
			return 7
		}),
		"MarkMain": reflect.ValueOf(func() { close(mainCalled) }),
	}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := i.EvalPathWithContext(ctx, "app")
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("source-directory evaluation did not enter native call")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled source-directory Eval error = %v, want context canceled", err)
	}
	if _, err := i.Eval(`var afterDirectoryCancel = 1`); err != nil {
		t.Fatal(err)
	}
	symbols := i.Symbols("app")["app"]
	published := symbols["DirectoryPublished"]
	if !published.IsValid() || !published.CanSet() {
		t.Fatalf("source-directory exported variable is not settable: %v", published)
	}
	published.SetInt(99)
	close(release)
	time.Sleep(20 * time.Millisecond)
	select {
	case <-mainCalled:
		t.Fatal("canceled source-directory initializer continued into main")
	default:
	}
	if got := i.Symbols("app")["app"]["DirectoryPublished"].Interface(); got != 99 {
		t.Fatalf("canceled source-directory Eval published into later frame: value=%v", got)
	}
}

func TestEvalSourceDirectoryWithContextPublishesOwnerOnlyAfterPreparation(t *testing.T) {
	holdEntered := make(chan struct{})
	holdRelease := make(chan struct{})
	appStat := make(chan struct{})
	filesystem := &blockingDirectoryFS{
		FS: fstest.MapFS{
			"gopath/src/hold/main.go": {Data: []byte(`package main`)},
			"gopath/src/app/main.go":  {Data: []byte(`package main`)},
		},
		holdEntered: holdEntered,
		holdRelease: holdRelease,
		appStat:     appStat,
	}
	i := interp.New(interp.Options{GoPath: "gopath", SourcecodeFilesystem: filesystem})
	if _, err := i.Eval(`var directoryCallbackRuns int; var directoryCallback = func() { directoryCallbackRuns++ }`); err != nil {
		t.Fatal(err)
	}
	callback, ok := i.Globals()["directoryCallback"].Interface().(func())
	if !ok {
		t.Fatal("interpreted callback is not callable")
	}

	holdResult := make(chan error, 1)
	go func() {
		_, err := i.EvalPathWithContext(context.Background(), "hold")
		holdResult <- err
	}()
	select {
	case <-holdEntered:
	case <-time.After(time.Second):
		t.Fatal("first directory evaluation did not hold compiler preparation")
	}
	defer func() {
		select {
		case <-holdRelease:
		default:
			close(holdRelease)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	appResult := make(chan error, 1)
	go func() {
		_, err := i.EvalPathWithContext(ctx, "app")
		appResult <- err
	}()
	select {
	case <-appStat:
		t.Fatal("second directory evaluation inspected its path before the live owner was released")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	if err := <-appResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("serialized source-directory Eval error = %v, want context canceled", err)
	}

	callback()
	if got := i.Globals()["directoryCallbackRuns"].Interface(); got != 1 {
		t.Fatalf("waiting source-directory Eval published its owner before preparation: callback runs=%v", got)
	}

	close(holdRelease)
	if err := <-holdResult; err != nil {
		t.Fatalf("first directory evaluation: %v", err)
	}
	select {
	case <-appStat:
		t.Fatal("canceled source-directory Eval inspected its path after returning")
	default:
	}
	if _, err := i.EvalPath("app"); err != nil {
		t.Fatalf("source-directory Eval after owner release: %v", err)
	}
	select {
	case <-appStat:
	case <-time.After(time.Second):
		t.Fatal("source-directory Eval did not inspect its path after owner release")
	}
	if v, err := i.Eval(`directoryCallbackRuns`); err != nil || v.Interface() != 1 {
		t.Fatalf("Eval after serialized directory preparation: value=%v err=%v", v, err)
	}
}

func TestEvalUnimportedBinaryPackageReturnsDiagnostic(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"host/host": {
		"Value": reflect.ValueOf(func() int { return 42 }),
	}}); err != nil {
		t.Fatal(err)
	}

	_, err := i.EvalWithContext(context.Background(), `host.Value()`)
	if err == nil {
		t.Fatal("unimported package use unexpectedly succeeded")
	}
	var panicErr interp.Panic
	if errors.As(err, &panicErr) {
		t.Fatalf("unimported package use returned internal panic: %v", err)
	}
	if !strings.Contains(err.Error(), "undefined: host") {
		t.Fatalf("unimported package error = %v, want undefined-name diagnostic", err)
	}

	v, err := i.Eval(`1 + 1`)
	if err != nil || v.Interface() != 2 {
		t.Fatalf("interpreter unusable after diagnostic: value=%v err=%v", v, err)
	}
}

func TestCompileOnlyGlobalsAndSymbolsSkipUnmaterializedVariables(t *testing.T) {
	i := interp.New(interp.Options{})
	program, err := i.Compile(`var CompiledOnlyGlobal = 42`)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := i.Globals()["CompiledOnlyGlobal"]; ok {
		t.Fatalf("Globals published unmaterialized variable: %v", value)
	}
	if _, err := i.Execute(program); err != nil {
		t.Fatal(err)
	}
	if got := i.Globals()["CompiledOnlyGlobal"].Interface(); got != 42 {
		t.Fatalf("Globals after Execute = %v, want 42", got)
	}

	j := interp.New(interp.Options{})
	program, err = j.Compile(`package sample; var Exported = 42`)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := j.Symbols("sample")["sample"]["Exported"]; ok {
		t.Fatalf("Symbols published unmaterialized variable: %v", value)
	}
	if _, err := j.Execute(program); err != nil {
		t.Fatal(err)
	}
	if got := j.Symbols("sample")["sample"]["Exported"].Interface(); got != 42 {
		t.Fatalf("Symbols after Execute = %v, want 42", got)
	}
}

func TestEvalUnsupportedRangeOverFuncReturnsDiagnostic(t *testing.T) {
	i := interp.New(interp.Options{})
	_, err := i.EvalWithContext(context.Background(), `for value := range func(yield func(int) bool) { yield(1) } { println(value) }`)
	if err == nil {
		t.Fatal("range over function unexpectedly succeeded")
	}
	var panicErr interp.Panic
	if errors.As(err, &panicErr) {
		t.Fatalf("unsupported range returned internal panic: %v", err)
	}
	if !strings.Contains(err.Error(), "cannot range over") {
		t.Fatalf("unsupported range error = %v, want range diagnostic", err)
	}

	v, err := i.Eval(`1 + 1`)
	if err != nil || v.Interface() != 2 {
		t.Fatalf("interpreter unusable after diagnostic: value=%v err=%v", v, err)
	}
}
