package interp

import (
	"context"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
)

const gorneshFenceTestTimeout = 3 * time.Second

type gorneshReentrantWriter struct {
	once  sync.Once
	eval  func()
	value reflect.Value
	err   error
}

func (w *gorneshReentrantWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		w.eval()
	})
	return len(p), nil
}

type gorneshReentrantStringer struct {
	once sync.Once
	eval func()
}

type (
	gorneshReentrantConversionSource int
	gorneshReentrantConversionDest   int
	gorneshConversionCallbackSource  func()
	gorneshConversionCallbackDest    func()
)

type gorneshOwnedBlockingStringer struct {
	Entered  func()
	Block    func()
	Finished func()
	Pointer  *int
}

func (s *gorneshReentrantStringer) String() string {
	s.once.Do(s.eval)
	return "reentrant"
}

func (s *gorneshOwnedBlockingStringer) String() string {
	s.Entered()
	s.Block()
	*s.Pointer = 2
	s.Finished()
	return "owned"
}

func evalWithGorneshFenceTimeout(t *testing.T, i *Interpreter, source string) (reflect.Value, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gorneshFenceTestTimeout)
	defer cancel()
	return i.EvalWithContext(ctx, source)
}

func TestGorneshFuncSweepFenceAllowsStdoutReentrantEval(t *testing.T) {
	writer := &gorneshReentrantWriter{}
	i := New(Options{Stdout: writer})
	writer.eval = func() { writer.value, writer.err = i.Eval(`21 * 2`) }

	if _, err := evalWithGorneshFenceTimeout(t, i, `println("outer")`); err != nil {
		t.Fatalf("print with reentrant writer: %v", err)
	}
	if writer.err != nil {
		t.Fatalf("writer reentrant Eval: %v", writer.err)
	}
	if !writer.value.IsValid() || writer.value.Interface() != 42 {
		t.Fatalf("writer reentrant Eval result = %v, want 42", writer.value)
	}
}

func TestGorneshFuncSweepFenceAllowsStringerReentrantEval(t *testing.T) {
	i := New(Options{Stdout: io.Discard})
	var value reflect.Value
	var evalErr error
	stringer := &gorneshReentrantStringer{eval: func() { value, evalErr = i.Eval(`40 + 2`) }}
	if err := i.Use(Exports{"reentrant/reentrant": {
		"Value": reflect.ValueOf(stringer),
	}}); err != nil {
		t.Fatal(err)
	}

	if _, err := evalWithGorneshFenceTimeout(t, i, `import "reentrant"; println(reentrant.Value)`); err != nil {
		t.Fatalf("print with reentrant Stringer: %v", err)
	}
	if evalErr != nil {
		t.Fatalf("Stringer reentrant Eval: %v", evalErr)
	}
	if !value.IsValid() || value.Interface() != 42 {
		t.Fatalf("Stringer reentrant Eval result = %v, want 42", value)
	}
}

func TestGorneshPrintPublishesInterpreterOwnedStringerBeforeExternalCall(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		name := "immediate"
		if deferred {
			name = "deferred"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan struct{})
			bodyEntered := make(chan struct{})
			bodyRelease := make(chan struct{})
			var enteredOnce sync.Once
			i := New(Options{Stdout: io.Discard})
			if err := i.Use(Exports{"ownedprint/ownedprint": {
				"Stringer":    reflect.ValueOf((*gorneshOwnedBlockingStringer)(nil)),
				"Entered":     reflect.ValueOf(func() { enteredOnce.Do(func() { close(entered) }) }),
				"Block":       reflect.ValueOf(func() { <-release }),
				"Finished":    reflect.ValueOf(func() { close(finished) }),
				"BodyEntered": reflect.ValueOf(func() { close(bodyEntered) }),
				"BodyBlock":   reflect.ValueOf(func() { <-bodyRelease }),
			}}); err != nil {
				t.Fatal(err)
			}
			mode := "println(OwnedPrintValueGornesh)"
			if deferred {
				mode = "defer println(OwnedPrintValueGornesh); ownedprint.BodyEntered(); ownedprint.BodyBlock()"
			}
			if _, err := i.Eval(`
import "ownedprint"
var OwnedPrintPointerGornesh *int
var OwnedPrintValueGornesh *ownedprint.Stringer
func runOwnedPrintGornesh() {
	OwnedPrintPointerGornesh = new(int)
	OwnedPrintValueGornesh = new(ownedprint.Stringer)
	OwnedPrintValueGornesh.Entered = ownedprint.Entered
	OwnedPrintValueGornesh.Block = ownedprint.Block
	OwnedPrintValueGornesh.Finished = ownedprint.Finished
	OwnedPrintValueGornesh.Pointer = OwnedPrintPointerGornesh
	` + mode + `
}`); err != nil {
				t.Fatalf("define owned print: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				_, err := i.EvalWithContext(ctx, `runOwnedPrintGornesh()`)
				done <- err
			}()
			if deferred {
				<-bodyEntered
				cancel()
				if err := <-done; err == nil {
					t.Fatal("deferred print Eval returned nil cancellation error")
				}
				close(bodyRelease)
				<-entered
			} else {
				<-entered
				cancel()
				if err := <-done; err == nil {
					t.Fatal("immediate print Eval returned nil cancellation error")
				}
			}
			value, err := i.Eval(`*OwnedPrintPointerGornesh = 99; 0`)
			if err != nil || !value.IsValid() {
				t.Fatalf("detach while String blocks: value=%v err=%v", value, err)
			}
			close(release)
			select {
			case <-finished:
			case <-time.After(gorneshFenceTestTimeout):
				t.Fatal("Stringer did not finish after release")
			}
			value, err = i.Eval(`*OwnedPrintPointerGornesh`)
			if err != nil || value.Interface() != 2 {
				t.Fatalf("Stringer mutation did not remain shared: value=%v err=%v", value, err)
			}
		})
	}
}

func TestGorneshDeferredRuntimeNativeCallPublishesArgumentsAtScheduling(t *testing.T) {
	bodyEntered := make(chan struct{})
	bodyRelease := make(chan struct{})
	mutateEntered := make(chan struct{})
	mutateRelease := make(chan struct{})
	mutateFinished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"deferrednative/deferrednative": {
		"BodyEntered": reflect.ValueOf(func() { close(bodyEntered) }),
		"BodyBlock":   reflect.ValueOf(func() { <-bodyRelease }),
		"Mutate": reflect.ValueOf(func(pointer *int) {
			close(mutateEntered)
			<-mutateRelease
			*pointer = 2
			close(mutateFinished)
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "deferrednative"
var DeferredNativePointerGornesh *int
func runDeferredNativeGornesh(fn func(*int)) {
	pointer := new(int)
	DeferredNativePointerGornesh = pointer
	defer fn(pointer)
	deferrednative.BodyEntered()
	deferrednative.BodyBlock()
}`); err != nil {
		t.Fatalf("define deferred runtime native call: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `runDeferredNativeGornesh(deferrednative.Mutate)`)
		done <- err
	}()
	select {
	case <-bodyEntered:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("deferred-native body did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("deferred-native Eval returned nil cancellation error")
	}
	close(bodyRelease)
	select {
	case <-mutateEntered:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("deferred native call did not start")
	}
	if _, err := i.Eval(`*DeferredNativePointerGornesh = 99; 0`); err != nil {
		t.Fatalf("detach during deferred native call: %v", err)
	}
	close(mutateRelease)
	select {
	case <-mutateFinished:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("deferred native call did not finish")
	}
	waitForFuncSweepGornesh(i)
	value, err := i.Eval(`*DeferredNativePointerGornesh`)
	if err != nil || value.Interface() != 2 {
		t.Fatalf("deferred native pointer split across detach: value=%v err=%v", value, err)
	}
}

func TestGorneshDeferredBuiltinMutationsAreSerializedWithDetach(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"deferredbuiltin/deferredbuiltin": {
		"Block": reflect.ValueOf(func() {
			close(entered)
			<-release
			close(finished)
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "deferredbuiltin"
var DeferredBuiltinMapGornesh = make(map[int]int)
var DeferredBuiltinDestinationGornesh = make([]int, 20000)
var DeferredBuiltinSourceGornesh = make([]int, 20000)
func prepareDeferredBuiltinGornesh() {
	for index := 0; index < 20000; index++ {
		DeferredBuiltinMapGornesh[index] = index
		DeferredBuiltinSourceGornesh[index] = index + 1
	}
}
func runDeferredBuiltinGornesh() {
	defer delete(DeferredBuiltinMapGornesh, 0)
	defer copy(DeferredBuiltinDestinationGornesh, DeferredBuiltinSourceGornesh)
	deferredbuiltin.Block()
}
prepareDeferredBuiltinGornesh()`); err != nil {
		t.Fatalf("define deferred builtins: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `runDeferredBuiltinGornesh()`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("deferred-builtin body did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("deferred-builtin Eval returned nil cancellation error")
	}

	laterDone := make(chan error, 1)
	go func() {
		_, err := i.Eval(`DeferredBuiltinMapGornesh[0] = 99; DeferredBuiltinDestinationGornesh[0] = 99; 0`)
		laterDone <- err
	}()
	close(release)
	if err := <-laterDone; err != nil {
		t.Fatalf("detach during deferred builtin mutations: %v", err)
	}
	select {
	case <-finished:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("deferred-builtin body did not finish")
	}
	waitForFuncSweepGornesh(i)
	value, err := i.Eval(`DeferredBuiltinMapGornesh[0] == 99 && DeferredBuiltinDestinationGornesh[0] == 99`)
	if err != nil || value.Interface() != true {
		t.Fatalf("old deferred builtin mutated detached root: value=%v err=%v", value, err)
	}
}

func TestGorneshFuncSweepFenceAllowsConversionHookReentrantEval(t *testing.T) {
	i := New(Options{})
	var nested reflect.Value
	var nestedErr error
	var conversions [][2]reflect.Type
	convertHook := func(from, to reflect.Type) func(reflect.Value, reflect.Value) {
		conversions = append(conversions, [2]reflect.Type{from, to})
		if from != reflect.TypeOf(gorneshReentrantConversionSource(0)) || to != reflect.TypeOf(gorneshReentrantConversionDest(0)) {
			return nil
		}
		return func(source, dest reflect.Value) {
			nested, nestedErr = i.Eval(`40 + 2`)
			dest.SetInt(source.Int() + 1)
		}
	}
	if err := i.Use(Exports{
		"github.com/traefik/yaegi/yaegi": {
			"convert": reflect.ValueOf(convertHook),
		},
		"conversion/conversion": {
			"Source": reflect.ValueOf((*gorneshReentrantConversionSource)(nil)),
			"Dest":   reflect.ValueOf((*gorneshReentrantConversionDest)(nil)),
		},
	}); err != nil {
		t.Fatal(err)
	}
	value, err := evalWithGorneshFenceTimeout(t, i, `
import "conversion"
var conversionHookSourceGornesh = conversion.Source(41)
conversion.Dest(conversionHookSourceGornesh)`)
	if err != nil || value.Interface() != gorneshReentrantConversionDest(42) {
		t.Fatalf("conversion hook result: value=%v err=%v conversions=%v", value, err, conversions)
	}
	if nestedErr != nil || !nested.IsValid() || nested.Interface() != 42 {
		t.Fatalf("conversion hook reentrant Eval: value=%v err=%v", nested, nestedErr)
	}
}

func TestGorneshConversionHookBindsDirectInterpretedCallbackToActiveOwner(t *testing.T) {
	var retained func()
	convertHook := func(from, to reflect.Type) func(reflect.Value, reflect.Value) {
		if from != reflect.TypeOf(gorneshConversionCallbackSource(nil)) || to != reflect.TypeOf(gorneshConversionCallbackDest(nil)) {
			return nil
		}
		return func(source, dest reflect.Value) {
			source.Call(nil)
			dest.Set(source.Convert(dest.Type()))
		}
	}
	i := New(Options{})
	if err := i.Use(Exports{
		"github.com/traefik/yaegi/yaegi": {
			"convert": reflect.ValueOf(convertHook),
		},
		"callbackconversion/callbackconversion": {
			"Source": reflect.ValueOf((*gorneshConversionCallbackSource)(nil)),
			"Dest":   reflect.ValueOf((*gorneshConversionCallbackDest)(nil)),
			"Get": reflect.ValueOf(func() gorneshConversionCallbackSource {
				return gorneshConversionCallbackSource(retained)
			}),
		},
	}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "callbackconversion"
var ConversionCallbackRunsGornesh int
var ConversionCallbackStoredGornesh callbackconversion.Dest
func makeConversionCallbackGornesh() func() {
	return func() { ConversionCallbackRunsGornesh++ }
}
makeConversionCallbackGornesh()`)
	if err != nil {
		t.Fatalf("create retained conversion callback: %v", err)
	}
	retained = value.Interface().(func())
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`
ConversionCallbackRunsGornesh = 100
ConversionCallbackStoredGornesh = callbackconversion.Dest(callbackconversion.Get())
ConversionCallbackRunsGornesh`)
	if err != nil || value.Interface() != 101 {
		t.Fatalf("conversion hook direct callback owner: value=%v err=%v", value, err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`ConversionCallbackRunsGornesh = 200; ConversionCallbackStoredGornesh(); ConversionCallbackRunsGornesh`)
	if err != nil || value.Interface() != 201 {
		t.Fatalf("persisted converted callback after detach: value=%v err=%v", value, err)
	}
}

func TestGorneshNativeCallbackPublishesOwnedResultsBeforeHostReturns(t *testing.T) {
	var retained *int
	i := New(Options{})
	if err := i.Use(Exports{"callbackresult/callbackresult": {
		"Retain": reflect.ValueOf(func(callback func() *int) { retained = callback() }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "callbackresult"
var CallbackResultPointerGornesh *int
func publishCallbackResultGornesh() {
	callbackresult.Retain(func() *int {
		CallbackResultPointerGornesh = new(int)
		return CallbackResultPointerGornesh
	})
}
publishCallbackResultGornesh()`); err != nil {
		t.Fatalf("publish callback result: %v", err)
	}
	if retained == nil {
		t.Fatal("native host did not retain callback result")
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`*CallbackResultPointerGornesh = 2; *CallbackResultPointerGornesh`)
	if err != nil || value.Interface() != 2 || *retained != 2 {
		t.Fatalf("callback result split after detach: value=%v retained=%v err=%v", value, retained, err)
	}
}

func TestGorneshNativeRecoveredCallbackPanicPublishesOwnedValue(t *testing.T) {
	var retained *int
	i := New(Options{})
	if err := i.Use(Exports{"callbackpanic/callbackpanic": {
		"Recover": reflect.ValueOf(func(callback func()) {
			defer func() {
				if recovered := recover(); recovered != nil {
					value := reflect.ValueOf(recovered)
					retained = unwrapOwnedValue(value).Interface().(*int)
				}
			}()
			callback()
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "callbackpanic"
var CallbackPanicPointerGornesh *int
func publishCallbackPanicGornesh() {
	callbackpanic.Recover(func() {
		CallbackPanicPointerGornesh = new(int)
		panic(CallbackPanicPointerGornesh)
	})
}
publishCallbackPanicGornesh()`); err != nil {
		t.Fatalf("publish recovered callback panic: %v", err)
	}
	if retained == nil {
		t.Fatal("native host did not recover callback panic value")
	}
	i.funcMu.RLock()
	object := i.ownedObjectLocked(reflect.ValueOf(retained))
	refs := 0
	if object != nil {
		refs = len(object.panicTokens)
	}
	i.funcMu.RUnlock()
	if object == nil || !object.hostShared || refs != 0 {
		t.Fatalf("callback panic publication: object=%v hostShared=%v panicRefs=%d", object, object != nil && object.hostShared, refs)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`*CallbackPanicPointerGornesh = 2; *CallbackPanicPointerGornesh`)
	if err != nil || value.Interface() != 2 || *retained != 2 {
		t.Fatalf("callback panic value split after detach: value=%v retained=%v err=%v", value, retained, err)
	}
}

func TestGorneshHostPublishedLateMutatedPanicFinishesExactToken(t *testing.T) {
	i := New(Options{})
	value, err := i.Eval(`
func makeLateMutatedHostPanicGornesh() func() {
	return func() {
		value := map[string]interface{}{"pointer": new(int)}
		defer func() {
			delete(value, "pointer")
			value["nested"] = map[string]interface{}{"pointer": new(int)}
			value["cycle"] = value
		}()
		panic(value)
	}
}
makeLateMutatedHostPanicGornesh()`)
	if err != nil {
		t.Fatalf("return late-mutated host panic callback: %v", err)
	}
	func() {
		defer func() { _ = recover() }()
		value.Interface().(func())()
	}()
	i.funcMu.RLock()
	defer i.funcMu.RUnlock()
	if len(i.panicTokens) != 0 {
		t.Fatalf("host-published panic tokens = %d, want 0", len(i.panicTokens))
	}
	for _, object := range i.ownedObjects {
		if len(object.panicTokens) != 0 {
			t.Fatalf("host-published object retains %d panic tokens", len(object.panicTokens))
		}
	}
	for _, meta := range i.funcMeta {
		if meta.group != nil && len(meta.group.panicTokens) != 0 {
			t.Fatalf("host-published func group retains %d panic tokens", len(meta.group.panicTokens))
		}
	}
}

func TestGorneshOverlappingPanicTokensKeepExactFuncGroups(t *testing.T) {
	for _, shared := range []bool{true, false} {
		name := "disjoint"
		if shared {
			name = "shared"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan int, 2)
			releases := []chan struct{}{make(chan struct{}), make(chan struct{})}
			i := New(Options{Stderr: io.Discard})
			if err := i.Use(Exports{"panicgroups/panicgroups": {
				"Wait": reflect.ValueOf(func(index int) { entered <- index; <-releases[index] }),
			}}); err != nil {
				t.Fatal(err)
			}
			if _, err := i.Eval(`
import "panicgroups"
func makePanicGroupPairGornesh(firstIndex, secondIndex int) (func(), func()) {
	value := 0
	firstSibling := func() { value++ }
	secondSibling := func() { value++ }
	first := func() {
		payload := map[string]func(){"callback": firstSibling}
		defer panicgroups.Wait(firstIndex)
		panic(payload)
	}
	second := func() {
		payload := map[string]func(){"callback": secondSibling}
		defer panicgroups.Wait(secondIndex)
		panic(payload)
	}
	return first, second
}
var PanicGroupFirstGornesh func()
var PanicGroupSecondGornesh func()
`); err != nil {
				t.Fatalf("define overlapping panic groups: %v", err)
			}
			install := `PanicGroupFirstGornesh, PanicGroupSecondGornesh = makePanicGroupPairGornesh(0, 1)`
			if !shared {
				install = `PanicGroupFirstGornesh, _ = makePanicGroupPairGornesh(0, 0); _, PanicGroupSecondGornesh = makePanicGroupPairGornesh(1, 1)`
			}
			if _, err := i.Eval(install); err != nil {
				t.Fatalf("install overlapping panic groups: %v", err)
			}
			firstValue, err := i.Eval(`PanicGroupFirstGornesh`)
			if err != nil {
				t.Fatal(err)
			}
			secondValue, err := i.Eval(`PanicGroupSecondGornesh`)
			if err != nil {
				t.Fatal(err)
			}
			invoke := func(callback func(), done chan<- struct{}) {
				defer close(done)
				defer func() { _ = recover() }()
				callback()
			}
			firstDone, secondDone := make(chan struct{}), make(chan struct{})
			go invoke(firstValue.Interface().(func()), firstDone)
			go invoke(secondValue.Interface().(func()), secondDone)
			seen := map[int]bool{}
			for len(seen) != 2 {
				select {
				case index := <-entered:
					seen[index] = true
				case <-time.After(gorneshFenceTestTimeout):
					t.Fatal("overlapping panic callbacks did not block")
				}
			}
			i.funcMu.RLock()
			tokenCount := len(i.panicTokens)
			if tokenCount != 2 {
				i.funcMu.RUnlock()
				t.Fatalf("overlapping panic token count = %d, want 2", tokenCount)
			}
			maxGroupTokens := 0
			for _, meta := range i.funcMeta {
				if meta.group != nil && len(meta.group.panicTokens) > maxGroupTokens {
					maxGroupTokens = len(meta.group.panicTokens)
				}
			}
			i.funcMu.RUnlock()
			if shared && maxGroupTokens != 2 {
				t.Fatalf("shared sibling group token count = %d, want 2", maxGroupTokens)
			}
			if !shared && maxGroupTokens > 1 {
				t.Fatalf("disjoint sibling group token count = %d, want at most 1", maxGroupTokens)
			}
			close(releases[0])
			select {
			case <-firstDone:
			case <-time.After(gorneshFenceTestTimeout):
				t.Fatal("first overlapping panic did not finish")
			}
			i.funcMu.RLock()
			remaining := len(i.panicTokens)
			i.funcMu.RUnlock()
			if remaining != 1 {
				t.Fatalf("panic token count after first finish = %d, want 1", remaining)
			}
			close(releases[1])
			select {
			case <-secondDone:
			case <-time.After(gorneshFenceTestTimeout):
				t.Fatal("second overlapping panic did not finish")
			}
			i.funcMu.RLock()
			remaining = len(i.panicTokens)
			i.funcMu.RUnlock()
			if remaining != 0 {
				t.Fatalf("panic token count after both finish = %d, want 0", remaining)
			}
		})
	}
}

func TestGorneshDirectHostCallbackPublishesOwnedResultAndPanic(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var DirectHostCallbackResultPointerGornesh *int
var DirectHostCallbackPanicPointerGornesh *int
func makeDirectHostResultCallbackGornesh() func() *int {
	return func() *int {
		DirectHostCallbackResultPointerGornesh = new(int)
		return DirectHostCallbackResultPointerGornesh
	}
}

func makeDirectHostPanicCallbackGornesh() func() {
	return func() {
		DirectHostCallbackPanicPointerGornesh = new(int)
		panic(DirectHostCallbackPanicPointerGornesh)
	}
}`); err != nil {
		t.Fatalf("define direct host callbacks: %v", err)
	}
	resultValue, err := i.Eval(`makeDirectHostResultCallbackGornesh()`)
	if err != nil {
		t.Fatalf("return direct result callback: %v", err)
	}
	resultPointer := resultValue.Interface().(func() *int)()
	panicValue, err := i.Eval(`makeDirectHostPanicCallbackGornesh()`)
	if err != nil {
		t.Fatalf("return direct panic callback: %v", err)
	}
	var panicPointer *int
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				panicPointer = unwrapOwnedValue(reflect.ValueOf(recovered)).Interface().(*int)
			}
		}()
		panicValue.Interface().(func())()
	}()
	if resultPointer == nil || panicPointer == nil {
		t.Fatalf("host callback publication missing: result=%v panic=%v", resultPointer, panicPointer)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err := i.Eval(`
*DirectHostCallbackResultPointerGornesh = 2
*DirectHostCallbackPanicPointerGornesh = 3
*DirectHostCallbackResultPointerGornesh + *DirectHostCallbackPanicPointerGornesh`)
	if err != nil || value.Interface() != 5 || *resultPointer != 2 || *panicPointer != 3 {
		t.Fatalf("direct host callback aliases split: value=%v result=%v panic=%v err=%v", value, resultPointer, panicPointer, err)
	}
}

func TestGorneshCanceledNativeCallbackSuppressesLateResultAndPanic(t *testing.T) {
	for _, panicMode := range []bool{false, true} {
		name := "result"
		if panicMode {
			name = "panic"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan struct{})
			var retained *int
			i := New(Options{})
			if err := i.Use(Exports{"canceledcallback/canceledcallback": {
				"Block": reflect.ValueOf(func() { close(entered); <-release }),
				"Run": reflect.ValueOf(func(callback func() *int) {
					retained = callback()
					close(finished)
				}),
				"Recover": reflect.ValueOf(func(callback func()) {
					defer func() {
						if recovered := recover(); recovered != nil {
							retained = unwrapOwnedValue(reflect.ValueOf(recovered)).Interface().(*int)
						}
						close(finished)
					}()
					callback()
				}),
			}}); err != nil {
				t.Fatal(err)
			}
			body := `canceledcallback.Run(func() (result *int) {
	CanceledCallbackPointerGornesh = new(int)
	result = CanceledCallbackPointerGornesh
	defer canceledcallback.Block()
	return
})`
			if panicMode {
				body = `canceledcallback.Recover(func() {
	CanceledCallbackPointerGornesh = new(int)
	defer canceledcallback.Block()
	panic(CanceledCallbackPointerGornesh)
})`
			}
			if _, err := i.Eval(`
import "canceledcallback"
var CanceledCallbackPointerGornesh *int
func runCanceledCallbackGornesh() {
` + body + `
}`); err != nil {
				t.Fatalf("define canceled callback: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				_, err := i.EvalWithContext(ctx, `runCanceledCallbackGornesh()`)
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(gorneshFenceTestTimeout):
				cancel()
				t.Fatal("callback did not reach deferred blocker")
			}
			cancel()
			if err := <-done; err == nil {
				t.Fatal("canceled callback Eval returned nil error")
			}
			value, err := i.Eval(`*CanceledCallbackPointerGornesh = 99; *CanceledCallbackPointerGornesh`)
			if err != nil || value.Interface() != 99 {
				t.Fatalf("detach canceled callback pointer: value=%v err=%v", value, err)
			}
			close(release)
			select {
			case <-finished:
			case <-time.After(gorneshFenceTestTimeout):
				t.Fatal("canceled callback host invocation did not finish")
			}
			if retained != nil {
				t.Fatalf("canceled callback published late %s value: %v", name, retained)
			}
			value, err = i.Eval(`*CanceledCallbackPointerGornesh`)
			if err != nil || value.Interface() != 99 {
				t.Fatalf("late canceled callback affected current root: value=%v err=%v", value, err)
			}
		})
	}
}

func TestGorneshCanceledRawHostCallbackSuppressesLateResult(t *testing.T) {
	published := make(chan struct{})
	outerEntered := make(chan struct{})
	outerRelease := make(chan struct{})
	innerEntered := make(chan struct{})
	innerRelease := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"canceledraw/canceledraw": {
		"Published": reflect.ValueOf(func() { close(published) }),
		"OuterBlock": reflect.ValueOf(func() {
			close(outerEntered)
			<-outerRelease
		}),
		"InnerBlock": reflect.ValueOf(func() {
			close(innerEntered)
			<-innerRelease
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `
import "canceledraw"
var CanceledRawPointerGornesh *int
var CanceledRawCallbackGornesh func() *int
func installCanceledRawCallbackGornesh() {
	CanceledRawCallbackGornesh = func() (result *int) {
		CanceledRawPointerGornesh = new(int)
		result = CanceledRawPointerGornesh
		defer canceledraw.InnerBlock()
		return
	}
	canceledraw.Published()
	canceledraw.OuterBlock()
}
installCanceledRawCallbackGornesh()`)
		done <- err
	}()
	select {
	case <-published:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("raw callback was not published")
	}
	select {
	case <-outerEntered:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("raw callback owner did not block")
	}
	raw := i.Globals()["CanceledRawCallbackGornesh"].Interface().(func() *int)
	result := make(chan *int, 1)
	go func() { result <- raw() }()
	select {
	case <-innerEntered:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("raw host callback did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled raw callback owner returned nil error")
	}
	value, err := i.Eval(`*CanceledRawPointerGornesh = 99; *CanceledRawPointerGornesh`)
	if err != nil || value.Interface() != 99 {
		t.Fatalf("detach raw callback pointer: value=%v err=%v", value, err)
	}
	close(innerRelease)
	select {
	case retained := <-result:
		if retained != nil {
			t.Fatalf("canceled raw callback published late result: %v", retained)
		}
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("raw host callback did not finish")
	}
	close(outerRelease)
	waitForFuncSweepGornesh(i)
	value, err = i.Eval(`*CanceledRawPointerGornesh`)
	if err != nil || value.Interface() != 99 {
		t.Fatalf("late raw callback affected current root: value=%v err=%v", value, err)
	}
}

func TestGorneshCanceledRawHostCallbackSuppressesLatePanic(t *testing.T) {
	published := make(chan struct{})
	outerEntered := make(chan struct{})
	outerRelease := make(chan struct{})
	innerEntered := make(chan struct{})
	innerRelease := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"canceledrawpanic/canceledrawpanic": {
		"Published": reflect.ValueOf(func() { close(published) }),
		"OuterBlock": reflect.ValueOf(func() {
			close(outerEntered)
			<-outerRelease
		}),
		"InnerBlock": reflect.ValueOf(func() {
			close(innerEntered)
			<-innerRelease
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `
import "canceledrawpanic"
var CanceledRawPanicPointerGornesh *int
var CanceledRawPanicCallbackGornesh func()
func installCanceledRawPanicCallbackGornesh() {
	CanceledRawPanicCallbackGornesh = func() {
		CanceledRawPanicPointerGornesh = new(int)
		defer canceledrawpanic.InnerBlock()
		panic(CanceledRawPanicPointerGornesh)
	}
	canceledrawpanic.Published()
	canceledrawpanic.OuterBlock()
}
installCanceledRawPanicCallbackGornesh()`)
		done <- err
	}()
	select {
	case <-published:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("raw panic callback was not published")
	}
	select {
	case <-outerEntered:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("raw panic callback owner did not block")
	}
	raw := i.Globals()["CanceledRawPanicCallbackGornesh"].Interface().(func())
	result := make(chan interface{}, 1)
	go func() {
		var recovered interface{}
		func() {
			defer func() { recovered = recover() }()
			raw()
		}()
		result <- recovered
	}()
	select {
	case <-innerEntered:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("raw host panic callback did not block")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled raw panic callback owner returned nil error")
	}
	value, err := i.Eval(`*CanceledRawPanicPointerGornesh = 99; *CanceledRawPanicPointerGornesh`)
	if err != nil || value.Interface() != 99 {
		t.Fatalf("detach raw panic callback pointer: value=%v err=%v", value, err)
	}
	close(innerRelease)
	select {
	case recovered := <-result:
		if recovered != nil {
			t.Fatalf("canceled raw callback published late panic: %v", recovered)
		}
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("raw host panic callback did not finish")
	}
	close(outerRelease)
	waitForFuncSweepGornesh(i)
	value, err = i.Eval(`*CanceledRawPanicPointerGornesh`)
	if err != nil || value.Interface() != 99 {
		t.Fatalf("late raw panic callback affected current root: value=%v err=%v", value, err)
	}
}

func TestGorneshInterpretedCallbackReturnDoesNotPublishOwnedValue(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
var InternalCallbackPointerGornesh *int
func createInternalCallbackPointerGornesh() *int { return new(int) }
func storeInternalCallbackPointerGornesh() { InternalCallbackPointerGornesh = createInternalCallbackPointerGornesh() }
storeInternalCallbackPointerGornesh()
0`); err != nil {
		t.Fatalf("store internal callback result: %v", err)
	}
	symbol := i.srcPkg[mainID]["InternalCallbackPointerGornesh"]
	if symbol == nil {
		t.Fatal("internal callback pointer symbol missing")
	}
	pointer := unwrapOwnedValue(i.frame.data[symbol.index])
	i.funcMu.RLock()
	object := i.ownedObjectLocked(pointer)
	i.funcMu.RUnlock()
	if object == nil || object.hostShared {
		t.Fatalf("ordinary interpreted return was published: object=%v hostShared=%v", object, object != nil && object.hostShared)
	}
}

func TestGorneshHostReturnedClosureReentryClonesLexicalOwnedCapture(t *testing.T) {
	childEntered := make(chan struct{})
	childRelease := make(chan struct{})
	childFinished := make(chan struct{})
	var retained func() int
	i := New(Options{})
	if err := i.Use(Exports{"directreentry/directreentry": {
		"ChildBlock": reflect.ValueOf(func() {
			close(childEntered)
			<-childRelease
			close(childFinished)
		}),
		"Get": reflect.ValueOf(func() func() int { return retained }),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "directreentry"
var DirectReentryCurrentGornesh func() int
func makeDirectReentryGornesh() func() int {
	state := map[string]int{"value": 1}
	go func() {
		defer func() { state["value"] = 2 }()
		directreentry.ChildBlock()
	}()
	return func() int { return state["value"] }
}

makeDirectReentryGornesh()`)
	if err != nil {
		t.Fatalf("return direct reentry closure: %v", err)
	}
	retained = value.Interface().(func() int)
	select {
	case <-childEntered:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("captured-state child did not block")
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`DirectReentryCurrentGornesh = directreentry.Get(); DirectReentryCurrentGornesh()`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("active direct closure reentry: value=%v err=%v", value, err)
	}
	close(childRelease)
	select {
	case <-childFinished:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("captured-state child did not finish")
	}
	waitForFuncSweepGornesh(i)
	value, err = i.Eval(`DirectReentryCurrentGornesh()`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("old lexical mutation crossed active reentry clone: value=%v err=%v", value, err)
	}
}

func TestGorneshDirectFuncLineageSurvivesRepeatedDetach(t *testing.T) {
	var retained func() int
	i := New(Options{})
	if err := i.Use(Exports{"directlineage/directlineage": {
		"Get": reflect.ValueOf(func() func() int { return retained }),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "directlineage"
var DirectLineageCurrentGornesh func() int
func makeDirectLineageCounterGornesh() func() int {
	count := 0
	return func() int { count++; return count }
}
makeDirectLineageCounterGornesh()`)
	if err != nil {
		t.Fatalf("return direct lineage counter: %v", err)
	}
	retained = value.Interface().(func() int)
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`DirectLineageCurrentGornesh = directlineage.Get(); DirectLineageCurrentGornesh()`)
	if err != nil || value.Interface() != 1 {
		t.Fatalf("first direct lineage activation: value=%v err=%v", value, err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`DirectLineageCurrentGornesh = directlineage.Get(); DirectLineageCurrentGornesh()`)
	if err != nil || value.Interface() != 2 {
		t.Fatalf("second direct lineage activation: value=%v err=%v", value, err)
	}
}

func TestGorneshDirectFuncActivationCachesSiblingClones(t *testing.T) {
	var first, second func() int
	i := New(Options{})
	if err := i.Use(Exports{"directsiblings/directsiblings": {
		"First":  reflect.ValueOf(func() func() int { return first }),
		"Second": reflect.ValueOf(func() func() int { return second }),
	}}); err != nil {
		t.Fatal(err)
	}
	value, err := i.Eval(`
import "directsiblings"
type directSiblingPairGornesh struct { First func() int; Second func() int }
var DirectSiblingFirstGornesh func() int
var DirectSiblingSecondGornesh func() int
func makeDirectSiblingPairGornesh() directSiblingPairGornesh {
	count := 0
	return directSiblingPairGornesh{
		First: func() int { count++; return count },
		Second: func() int { count++; return count },
	}
}
makeDirectSiblingPairGornesh()`)
	if err != nil {
		t.Fatalf("return direct sibling closures: %v", err)
	}
	pair := unwrapOwnedValue(value)
	first = pair.Field(0).Interface().(func() int)
	second = pair.Field(1).Interface().(func() int)
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`
DirectSiblingFirstGornesh = directsiblings.First()
DirectSiblingSecondGornesh = directsiblings.Second()
DirectSiblingFirstGornesh()*10 + DirectSiblingSecondGornesh()`)
	if err != nil || value.Interface() != 12 {
		t.Fatalf("direct sibling activation did not share carrier: value=%v err=%v", value, err)
	}
	forceDetachedRootCloneGornesh(t, i)
	value, err = i.Eval(`
DirectSiblingFirstGornesh = directsiblings.First()
DirectSiblingSecondGornesh = directsiblings.Second()
DirectSiblingFirstGornesh()*10 + DirectSiblingSecondGornesh()`)
	if err != nil || value.Interface() != 34 {
		t.Fatalf("direct sibling lineage after detach: value=%v err=%v", value, err)
	}
}

func TestGorneshFuncSweepFenceAllowsRootResizeWithBackgroundGoroutine(t *testing.T) {
	started := make(chan struct{})
	stop := make(chan struct{})
	finished := make(chan struct{})
	var stopOnce sync.Once
	stopBackground := func() { stopOnce.Do(func() { close(stop) }) }
	defer stopBackground()

	i := New(Options{})
	if err := i.Use(Exports{"fence/fence": {
		"Started": reflect.ValueOf(func() { close(started) }),
		"Stopped": reflect.ValueOf(func() { close(finished) }),
		"ShouldStop": reflect.ValueOf(func() bool {
			select {
			case <-stop:
				return true
			default:
				return false
			}
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "fence"
var fenceBackgroundTicksGornesh int
go func() {
	fence.Started()
	for !fence.ShouldStop() { fenceBackgroundTicksGornesh++ }
	fence.Stopped()
}()
0`); err != nil {
		t.Fatalf("launch background goroutine: %v", err)
	}
	select {
	case <-started:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("background goroutine did not start")
	}

	value, err := evalWithGorneshFenceTimeout(t, i, `
var fenceLaterGlobalGornesh = func() int { return 42 }
fenceLaterGlobalGornesh()`)
	if err != nil {
		t.Fatalf("later Eval with root resize: %v", err)
	}
	if value.Interface() != 42 {
		t.Fatalf("later Eval result = %v, want 42", value)
	}

	stopBackground()
	select {
	case <-finished:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("background goroutine did not stop")
	}
}

func TestGorneshFuncSweepDoesNotTraverseNativeMutatedReference(t *testing.T) {
	entered := make(chan struct{})
	stop := make(chan struct{})
	finished := make(chan struct{})
	i := New(Options{})
	if err := i.Use(Exports{"fence/fence": {
		"Mutate": reflect.ValueOf(func(callbacks map[int]func()) {
			close(entered)
			for {
				select {
				case <-stop:
					close(finished)
					return
				default:
					callbacks[0] = func() {}
				}
			}
		}),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "fence"
var fenceNativeMutatedCallbacksGornesh = map[int]func(){0: func(){}}`); err != nil {
		t.Fatalf("define native-mutated callback map: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `fence.Mutate(fenceNativeMutatedCallbacksGornesh)`)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(gorneshFenceTestTimeout):
		cancel()
		t.Fatal("native mutator did not start")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled native-mutator Eval returned nil error")
	}
	// The native call is already mutating its retained alias. Detach and later
	// sweeps must never traverse that explicitly host-shared map.
	if _, err := evalWithGorneshFenceTimeout(t, i, `0`); err != nil {
		t.Fatalf("detach canceled root: %v", err)
	}
	for iteration := 0; iteration < 20; iteration++ {
		if _, err := evalWithGorneshFenceTimeout(t, i, `0`); err != nil {
			close(stop)
			t.Fatalf("sweep during native mutation %d: %v", iteration, err)
		}
	}
	close(stop)
	select {
	case <-finished:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("native mutator did not stop")
	}
	waitForFuncSweepGornesh(i)
}

func TestGorneshOwnedReleaseDoesNotTraverseGlobalsSharedStructCell(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
type globalsSharedStructGornesh struct { Pointer *int }
var globalsSharedStructValueGornesh = globalsSharedStructGornesh{Pointer: new(int)}
func globalsSharedStructChurnGornesh() {
	value := new(int)
	if *value == -1 { panic("unreachable") }
}`); err != nil {
		t.Fatalf("define Globals-shared struct: %v", err)
	}
	cell := i.Globals()["globalsSharedStructValueGornesh"]
	if !cell.IsValid() {
		t.Fatal("Globals-shared struct cell missing")
	}
	field := cell.FieldByName("Pointer")
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-stop:
				return
			default:
				field.Set(reflect.New(field.Type().Elem()))
			}
		}
	}()
	for iteration := 0; iteration < 50; iteration++ {
		if _, err := i.Eval(`globalsSharedStructChurnGornesh()`); err != nil {
			close(stop)
			t.Fatalf("release during Globals struct mutation %d: %v", iteration, err)
		}
		_ = i.Globals()
	}
	close(stop)
	select {
	case <-finished:
	case <-time.After(gorneshFenceTestTimeout):
		t.Fatal("Globals struct mutator did not stop")
	}
}
