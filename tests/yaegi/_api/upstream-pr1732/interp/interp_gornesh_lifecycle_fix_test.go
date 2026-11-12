package interp_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/traefik/yaegi/interp"
)

// TestGorneshCanceledSourceImportRetryEvaluatesCleanly pins the drained
// cancel-retry sequence for a source package: cancel an import whose
// initializer blocks in native code, let the canceled worker fully drain, and
// then evaluate the same import again. Before the detached-root sanitize fix,
// the retry committed the import but the first evaluation of the expression
// panicked with "reflect: call of reflect.Value.SetString on func Value"
// because recompiled slots reused stale typed cells from the canceled run.
func TestGorneshCanceledSourceImportRetryEvaluatesCleanly(t *testing.T) {
	var calls atomic.Int32
	blocked := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{
		GoPath: "gopath",
		SourcecodeFilesystem: fstest.MapFS{
			"gopath/src/cncl/cncl.go": {Data: []byte(`package cncl
import "cnclhost"
var A = cnclhost.Make("a")
var B = cnclhost.Make("b")
`)},
		},
	})
	makeFn := func(s string) string {
		if calls.Add(1) == 1 {
			close(blocked)
			<-release
		}
		return s
	}
	if err := i.Use(interp.Exports{"cnclhost/cnclhost": {"Make": reflect.ValueOf(makeFn)}}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `import "cncl"; cncl.A + cncl.B`)
		res <- err
	}()
	<-blocked
	cancel()
	if err := <-res; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled import err = %v, want context.Canceled", err)
	}
	close(release)
	// Let the canceled worker drain so the retry takes the failed-root
	// detach path; retrying while the zombie is still parked does not.
	time.Sleep(100 * time.Millisecond)

	v, err := i.Eval(`import "cncl"; cncl.A + cncl.B`)
	if err != nil {
		t.Fatalf("retry eval failed: %v", err)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("init host calls = %d, want at least 2", got)
	}
	if got, ok := v.Interface().(string); !ok || got != "ab" {
		t.Fatalf("retry value = %v (%T), want ab", v, v.Interface())
	}

	// Repeated evaluations stay stable after the recovered retry.
	if _, err := i.Eval(`import "cncl"; cncl.A + cncl.B`); err != nil {
		t.Fatalf("follow-up eval failed: %v", err)
	}
}

// TestGorneshOwnedAllocationScanStaysBounded guards the O(1) fast paths in
// the per-write ownership scans: an interpreter that retains a few thousand
// owned allocations must not pay a registry walk per instruction. The bound
// is deliberately generous; before the fast paths this workload took about a
// minute, afterwards it runs in tens of milliseconds.
func TestGorneshOwnedAllocationScanStaysBounded(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"gorneshfix/gorneshfix": {"Noop": reflect.ValueOf(func(v int) int { return v })}}); err != nil {
		t.Fatal(err)
	}
	i.ImportUsed()
	src := `
keep := make([][]int, 0, 2000)
for i := 0; i < 2000; i++ {
	keep = append(keep, make([]int, 4))
}
s := 0
for i := 0; i < 100000; i++ { s += i }
gorneshfix.Noop(s)
`
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := i.Eval(src)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("owned-allocation scan did not stay bounded: eval still running after 10s (elapsed %v)", time.Since(start))
	}
}

// TestGorneshRetainedCallbackStillWorksAfterBoundedSweeps exercises the
// call-path memoization: a host-retained closure passed repeatedly to a
// native function must keep working across many invocations and later Evals.
func TestGorneshRetainedCallbackStillWorksAfterBoundedSweeps(t *testing.T) {
	i := interp.New(interp.Options{})
	total := 0
	if err := i.Use(interp.Exports{"gorneshfix/gorneshfix": {
		"Apply": reflect.ValueOf(func(f func(int) int) int { total = f(total); return total }),
	}}); err != nil {
		t.Fatal(err)
	}
	i.ImportUsed()
	if _, err := i.Eval(`
f := func(x int) int { return x + 3 }
for i := 0; i < 500; i++ {
	gorneshfix.Apply(f)
}
`); err != nil {
		t.Fatal(err)
	}
	if total != 1500 {
		t.Fatalf("total = %d, want 1500", total)
	}
	// The same closure keeps working in a later Eval.
	if _, err := i.Eval(`gorneshfix.Apply(f)`); err != nil {
		t.Fatal(err)
	}
	if total != 1503 {
		t.Fatalf("total after later Eval = %d, want 1503", total)
	}
}

// TestGorneshReturnChannelReceiveInDeclaredFunc pins receive-in-return-position
// inside a REPL-declared function: before the recv store fix, the received
// unaddressable value was stored into the frame cell and the regular-return
// Set panicked with "reflect: reflect.Value.Set using unaddressable value".
func TestGorneshReturnChannelReceiveInDeclaredFunc(t *testing.T) {
	i := interp.New(interp.Options{})
	v, err := i.Eval("\nfunc f2() int {\n\tdone := make(chan int, 1)\n\tdone <- 7\n\treturn <-done\n}\nf2()")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Interface().(int); got != 7 {
		t.Fatalf("f2() = %d, want 7", got)
	}
}

// TestGorneshReentrantEvalFromDeepNativeStack pins reentrant Eval detection
// for host callbacks sitting under a deep native stack. The reentrancy probe
// must walk the whole stack: a host function that recurses natively before
// calling back into Eval used to be misread as an unrelated goroutine once the
// recursion passed the probe's fixed window, deadlocking the nested Eval on
// the execution gate while the outer Eval waited for the callback.
func TestGorneshReentrantEvalFromDeepNativeStack(t *testing.T) {
	i := interp.New(interp.Options{})
	var nested func() int
	var recNative func(int) int
	recNative = func(n int) int {
		if n == 0 {
			return nested()
		}
		return recNative(n-1) + 1
	}
	nested = func() int {
		v, err := i.Eval("40 + 2")
		if err != nil {
			t.Error(err)
			return -1
		}
		return int(v.Int())
	}
	if err := i.Use(interp.Exports{"gorneshdeep/gorneshdeep": {
		"Recurse": reflect.ValueOf(recNative),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval("import \"gorneshdeep\""); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := i.Eval("gorneshdeep.Recurse(100)")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reentrant Eval from a deep native stack blocked on the execution gate")
	}
}

// TestGorneshEvalTestPackageInitWaitsForRunningEval pins execution-gate
// coverage for EvalTest: a source package's initialization runs interpreted
// code, so it must wait for an in-flight evaluation instead of overlapping it.
func TestGorneshEvalTestPackageInitWaitsForRunningEval(t *testing.T) {
	var entered atomic.Int32
	blocked := make(chan struct{})
	release := make(chan struct{})
	i := interp.New(interp.Options{
		GoPath: "gopath",
		SourcecodeFilesystem: fstest.MapFS{
			"gopath/src/gatepkg/gatepkg.go": {Data: []byte(`package gatepkg
import "gatehost"
var Entered = gatehost.Enter()
`)},
		},
	})
	if err := i.Use(interp.Exports{"gatehost/gatehost": {
		"Block": reflect.ValueOf(func() { close(blocked); <-release }),
		"Enter": reflect.ValueOf(func() int { return int(entered.Add(1)) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`import "gatehost"`); err != nil {
		t.Fatal(err)
	}
	evalDone := make(chan error, 1)
	go func() {
		_, err := i.Eval(`gatehost.Block()`)
		evalDone <- err
	}()
	<-blocked

	testDone := make(chan error, 1)
	go func() { testDone <- i.EvalTest("gatepkg") }()

	// The package initializer must stay parked behind the blocked Eval.
	time.Sleep(250 * time.Millisecond)
	if got := entered.Load(); got != 0 {
		close(release)
		t.Fatalf("EvalTest package init ran concurrently with blocked Eval (entered=%d)", got)
	}
	close(release)
	if err := <-evalDone; err != nil {
		t.Fatalf("blocked eval failed: %v", err)
	}
	if err := <-testDone; err != nil {
		t.Fatalf("EvalTest failed: %v", err)
	}
	if got := entered.Load(); got != 1 {
		t.Fatalf("EvalTest package init host calls = %d, want 1", got)
	}
}

// TestGorneshZombieDeferredWritesDoNotRaceNewEvals pins the zombie drain
// barrier: a canceled evaluation's deferred calls still run, but they must
// not overlap the execution of a later evaluation on the same interpreter.
// Before the barrier, a canceled worker unwinding defers that write a global
// map raced the next evaluation's writes (fatal concurrent map writes).
func TestGorneshZombieDeferredWritesDoNotRaceNewEvals(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{"gorneshzombie/gorneshzombie": {
		"Block": reflect.ValueOf(func() { <-make(chan struct{}) }),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "gorneshzombie"
var stressLog = map[string]int{}
func alloc(tag string) int {
	stressLog[tag] = len(stressLog) + 1
	return len(stressLog)
}
func spin(n int) { for i := 0; i < n; i++ { _ = i } }
func plain() {
	defer spin(2000)
	defer func() { alloc("plain-defer") }()
	alloc("pre")
	gorneshzombie.Block()
}
`); err != nil {
		t.Fatal(err)
	}
	// A publisher keeps reading host-facing state concurrently, as embedders do.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			func() {
				defer func() { _ = recover() }()
				_ = i.Globals()
				_ = i.Symbols("main")
			}()
		}
	}()
	defer close(stop)

	for k := 0; k < 40; k++ {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := i.EvalWithContext(ctx, "plain()")
			done <- err
		}()
		time.Sleep(2 * time.Millisecond) // let the run reach the blocking host call
		cancel()                         // API returns; worker keeps unwinding defers
		if err := <-done; err == nil {
			t.Fatalf("iteration %d: canceled eval returned nil error", k)
		}
		// The next evaluation must start while the previous worker may still
		// be unwinding its deferred phase; the barrier serializes them.
		if _, err := i.Eval("1 + 1"); err != nil {
			t.Fatalf("iteration %d: follow-up eval failed: %v", k, err)
		}
	}
}
