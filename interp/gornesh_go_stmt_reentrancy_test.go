package interp_test

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/traefik/yaegi/interp"
)

// TestGorneshGoStatementEvalWaitsForGate pins the reentrancy-token contract:
// an Eval issued from a host callback running on a goroutine spawned by an
// interpreted `go` statement is NOT reentrant. The goroutine has runCfg on
// its stack but holds no execution token, so its Eval must wait for the
// execution gate instead of bypassing it and running concurrently with the
// gated execution (the stack-scan probe this replaces raced
// prepareExecutionFrame against the running execution).
func TestGorneshGoStatementEvalWaitsForGate(t *testing.T) {
	i := interp.New(interp.Options{})

	var mu sync.Mutex
	innerResult := 0
	entered := make(chan struct{})
	release := make(chan struct{})
	innerDone := make(chan struct{})
	callbackStarted := make(chan struct{})

	if err := i.Use(interp.Exports{
		"gostmttoken/gostmttoken": {
			// Block parks the outer execution while it holds the gate.
			"Block": reflect.ValueOf(func() {
				close(entered)
				<-release
			}),
			// CallbackEval runs on a goroutine spawned by an interpreted
			// `go` statement and calls back into Eval.
			"CallbackEval": reflect.ValueOf(func() {
				close(callbackStarted)
				v, err := i.Eval(`40 + 2`)
				mu.Lock()
				if err != nil {
					t.Error(err)
				} else {
					innerResult = v.Interface().(int)
				}
				mu.Unlock()
				close(innerDone)
			}),
		},
	}); err != nil {
		t.Fatal(err)
	}

	outerDone := make(chan error, 1)
	go func() {
		_, err := i.Eval(`
import "gostmttoken"

go gostmttoken.CallbackEval()

gostmttoken.Block()
0
`)
		outerDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("outer evaluation did not reach the blocking call")
	}
	// The go-statement goroutine may not have been scheduled yet when the
	// outer execution parked; wait for its callback before releasing.
	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("go-statement callback did not start")
	}

	// While the outer evaluation holds the gate, the callback's Eval must
	// not complete: without a token it waits instead of bypassing.
	select {
	case <-innerDone:
		t.Fatal("go-statement Eval ran concurrently with the gated execution")
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-outerDone:
		if err != nil {
			t.Fatalf("outer evaluation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("outer evaluation did not finish")
	}

	select {
	case <-innerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("go-statement Eval did not complete after the gate was released")
	}
	mu.Lock()
	result := innerResult
	mu.Unlock()
	if result != 42 {
		t.Fatalf("go-statement Eval result: got %d, want 42", result)
	}
}

// TestGorneshHostCallbackReentrantAfterDeepNativeCalls verifies the token
// still grants reentrancy on the execution's own goroutine, under native
// recursion deeper than any fixed stack window: the nested Eval re-enters
// while the outer evaluation is parked inside the host callback, so losing
// the token would deadlock instead of returning.
func TestGorneshHostCallbackReentrantAfterDeepNativeCalls(t *testing.T) {
	i := interp.New(interp.Options{})
	deepEvalDone := make(chan error, 1)

	var recurse func(n int)
	recurse = func(n int) {
		if n < 64 {
			recurse(n + 1)
			return
		}
		// Bottom of a 64-frame native stack, synchronously inside the outer
		// evaluation: the reentrant Eval must bypass the gate its own
		// execution holds.
		_, err := i.Eval(`1 + 1`)
		deepEvalDone <- err
	}

	if err := i.Use(interp.Exports{
		"deeptoken/deeptoken": {
			"Recurse": reflect.ValueOf(func(n int) { recurse(n) }),
		},
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := i.Eval(`
import "deeptoken"

deeptoken.Recurse(0)
`)
		done <- err
	}()

	select {
	case err := <-deepEvalDone:
		if err != nil {
			t.Fatalf("reentrant eval from deep native stack: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reentrant eval from deep native stack deadlocked")
	}
	if err := <-done; err != nil {
		t.Fatalf("outer evaluation: %v", err)
	}
}
