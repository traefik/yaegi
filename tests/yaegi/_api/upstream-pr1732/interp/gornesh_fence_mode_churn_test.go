package interp_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/traefik/yaegi/interp"
)

// TestGorneshFenceModeCaptureUnderZombieChurn stresses the funcSweep fence
// mode capture: each exec step acquires the fence in a mode chosen from the
// zombieDefers counter at entry, and the release helpers (native calls,
// selects) release it during the step. The counter is flipped concurrently by
// canceled workers draining their deferred calls (zombie phase). Before the
// acquired mode was captured per step, a helper that re-derived the mode at
// release time could Unlock an RLock-held fence — an uncatchable fatal — or
// wedge the fence. The churn below runs both modes against constant counter
// flips; surviving without a fatal is the assertion.
func TestGorneshFenceModeCaptureUnderZombieChurn(t *testing.T) {
	i := interp.New(interp.Options{})
	if err := i.Use(interp.Exports{
		"fencechurn/fencechurn": {
			"Nop": reflect.ValueOf(func() {}),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`
import "fencechurn"

func churnDeferredWorkFencechurn() {
	sum := 0
	for k := 0; k < 30000; k++ {
		fencechurn.Nop()
		sum = sum + k
	}
	_ = sum
}

func churnZombieRunFencechurn() {
	defer churnDeferredWorkFencechurn()
	defer fencechurn.Nop()
	sum := 0
	for k := 0; k < 200000; k++ {
		sum = sum + k
	}
	_ = sum
}
`); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Steady-state workers: steps that acquire the fence shared, call a
	// native function (releasing and reacquiring the fence inside the step
	// through the release helpers), then continue stepping.
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := i.Eval(`
import "fencechurn"

func churnNativeStepFencechurn() int {
	// A long interpreted stretch precedes the native call: the longer the
	// step runs after entry chose the fence mode, the wider the window in
	// which a zombie counter flip would make a re-derived release fatal.
	sum := 0
	for k := 0; k < 20000; k++ {
		sum = sum + k
	}
	fencechurn.Nop()
	for k := 0; k < 20000; k++ {
		sum = sum + k
	}
	return sum
}
churnNativeStepFencechurn()
`); err != nil {
					t.Errorf("steady eval: %v", err)
					return
				}
			}
		}()
	}

	// Zombie workers: canceled evaluations whose deferred interpreted calls
	// unwind after the API call returns, flipping zombieDefers up and down
	// while the steady workers are mid-step.
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, _ = i.EvalWithContext(ctx, `churnZombieRunFencechurn()`)
				}()
				time.Sleep(200 * time.Microsecond)
				cancel()
				select {
				case <-done:
				case <-time.After(30 * time.Second):
					t.Error("zombie eval did not finish")
					return
				}
			}
		}()
	}

	time.Sleep(3 * time.Second)
	close(stop)
	wg.Wait()
}
