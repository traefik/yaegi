package interp_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/traefik/yaegi/interp"
)

// TestGorneshZombieMidDrainCancelWaitsForGate pins the live-owner rule for
// the gate bypass: the cancellation can land AFTER the deferred drain already
// started, while the worker is parked inside a deferred host call. The API
// returns and frees the gate immediately, and the drain later resumes and
// calls Eval — that Eval must observe the (now closed) execution owner at
// gate-acquisition time and wait, instead of bypassing with a token sampled
// before the cancellation. A bypass would let the drain's
// prepareExecutionFrame rewrite the live root's owner concurrently with the
// gated execution.
func TestGorneshZombieMidDrainCancelWaitsForGate(t *testing.T) {
	i := interp.New(interp.Options{})

	var mu sync.Mutex
	zombieResult := 0
	drainEntered := make(chan struct{})
	releaseDrain := make(chan struct{})
	gatedStarted := make(chan struct{})
	zombieDone := make(chan struct{})
	var gatedOnce, drainOnce sync.Once

	if err := i.Use(interp.Exports{
		"zombietoctou/zombietoctou": {
			// DrainCb is the deferred call of the context evaluation: it
			// parks until the test releases it, then re-enters Eval from the
			// drain.
			"DrainCb": reflect.ValueOf(func() {
				drainOnce.Do(func() { close(drainEntered) })
				<-releaseDrain
				v, err := i.Eval(`6 * 7`)
				mu.Lock()
				if err != nil {
					t.Error(err)
				} else {
					zombieResult = v.Interface().(int)
				}
				mu.Unlock()
				close(zombieDone)
			}),
			// Gated holds the gate for the later unrelated evaluation.
			"Gated": reflect.ValueOf(func() {
				gatedOnce.Do(func() { close(gatedStarted) })
				time.Sleep(300 * time.Millisecond)
			}),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "zombietoctou"
`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	evalDone := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `
defer zombietoctou.DrainCb()

1
`)
		evalDone <- err
	}()

	// The body completes immediately and the drain parks inside DrainCb
	// BEFORE the test cancels — the drain's one-shot zombie sample observed
	// a live owner.
	select {
	case <-drainEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("evaluation did not reach the deferred drain call")
	}
	cancel()
	select {
	case err := <-evalDone:
		if err == nil {
			t.Fatal("canceled evaluation returned nil error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled evaluation did not return")
	}

	// Hold the gate with an unrelated evaluation BEFORE resuming the drain.
	gatedDone := make(chan struct{})
	go func() {
		if _, err := i.Eval(`
import "zombietoctou"

zombietoctou.Gated()
`); err != nil {
			t.Error(err)
		}
		close(gatedDone)
	}()
	select {
	case <-gatedStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("gated evaluation did not start")
	}

	close(releaseDrain)

	// The drain's Eval must wait for the held gate: its owner fired while it
	// was parked, so the bypass is refused at gate-acquisition time.
	select {
	case <-zombieDone:
		t.Fatal("mid-drain-cancel zombie Eval bypassed the held gate")
	case <-gatedDone:
	}

	select {
	case <-zombieDone:
	case <-time.After(5 * time.Second):
		t.Fatal("zombie Eval did not complete after the gate was released")
	}
	mu.Lock()
	result := zombieResult
	mu.Unlock()
	if result != 42 {
		t.Fatalf("zombie Eval result: got %d, want 42", result)
	}
}
