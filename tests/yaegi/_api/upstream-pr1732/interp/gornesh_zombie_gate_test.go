package interp_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/traefik/yaegi/interp"
)

// TestGorneshZombieEvalWaitsForGate pins the zombie gate rule: a canceled
// worker draining its deferred calls runs outside the execution gate, so a
// host callback of the drain that calls Eval must WAIT for the gate instead
// of bypassing it. While the drain still holds the reentrancy token, a
// bypass would let the zombie's prepareExecutionFrame rewrite the live
// root's cancellation owner concurrently with the gated execution's steps.
func TestGorneshZombieEvalWaitsForGate(t *testing.T) {
	i := interp.New(interp.Options{})

	var mu sync.Mutex
	zombieResult := 0
	parkEntered := make(chan struct{})
	releasePark := make(chan struct{})
	gatedStarted := make(chan struct{})
	zombieEvalStarted := make(chan struct{})
	zombieDone := make(chan struct{})
	var gatedOnce, zombieOnce, doneOnce sync.Once

	if err := i.Use(interp.Exports{
		"zombiegate/zombiegate": {
			// Park parks the context evaluation inside a host call.
			"Park": reflect.ValueOf(func() {
				close(parkEntered)
				<-releasePark
			}),
			// ZombieEval runs as the canceled worker's deferred call: it
			// signals, then calls Eval on the drain's goroutine.
			"ZombieEval": reflect.ValueOf(func() {
				zombieOnce.Do(func() { close(zombieEvalStarted) })
				v, err := i.Eval(`6 * 7`)
				mu.Lock()
				if err != nil {
					t.Error(err)
				} else {
					zombieResult = v.Interface().(int)
				}
				mu.Unlock()
				doneOnce.Do(func() { close(zombieDone) })
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
import "zombiegate"

func zombieCallbackZombiegate() {
	zombiegate.ZombieEval()
}
`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	evalDone := make(chan error, 1)
	go func() {
		_, err := i.EvalWithContext(ctx, `
defer zombieCallbackZombiegate()

zombiegate.Park()
0
`)
		evalDone <- err
	}()

	select {
	case <-parkEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("context evaluation did not reach the park call")
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

	// Start the later unrelated evaluation and wait until it actually holds
	// the gate BEFORE releasing the zombie: the gate is free between the
	// canceled API return and this Eval, and the zombie must observe it held.
	gatedDone := make(chan struct{})
	go func() {
		defer close(gatedDone)
		if _, err := i.Eval(`
import "zombiegate"

zombiegate.Gated()
`); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-gatedStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("gated evaluation did not start")
	}

	// Releasing the parked worker starts the zombie drain, whose deferred
	// callback issues the nested Eval against the held gate.
	close(releasePark)
	select {
	case <-zombieEvalStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("zombie drain did not reach the deferred callback")
	}

	select {
	case <-zombieDone:
		t.Fatal("zombie Eval bypassed the gate held by a later unrelated evaluation")
	case <-gatedDone:
	}

	// The gate is free again; the zombie's Eval must now complete.
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
