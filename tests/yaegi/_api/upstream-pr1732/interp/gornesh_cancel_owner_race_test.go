package interp_test

import (
	"context"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// TestGorneshCanceledOwnerRewriteRace exercises the shared root's cancellation
// owner: an interpreted goroutine outliving its Eval keeps sampling
// frame.canceled() on every step (the runCfg loop and the zombie-defer check
// read it), while a later Eval on the same interpreter rewrites the owner in
// prepareExecutionFrame under the frame lock. Before the read side was placed
// under the frame's read lock, the Go race detector flagged that read/write
// pair, and a misjudged zombie classification could follow from a stale
// observation.
func TestGorneshCanceledOwnerRewriteRace(t *testing.T) {
	const rounds = 2
	const interpreters = 2

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		for n := 0; n < interpreters; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runCancelOwnerRewriteRound(t)
			}()
		}
		wg.Wait()
	}
}

// runCancelOwnerRewriteRound runs on a worker goroutine; it reports
// failures with t.Errorf (FailNow is only documented for the test
// goroutine) and returns early instead of calling t.Fatal.
func runCancelOwnerRewriteRound(t *testing.T) {
	t.Helper()

	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Errorf("use stdlib: %v", err)
		return
	}

	entered := make(chan struct{})
	var enteredOnce sync.Once
	done := make(chan struct{})
	var doneOnce sync.Once
	release := make(chan struct{})
	if err := i.Use(interp.Exports{
		"ownerrace/ownerrace": {
			// Entered reports that the orphaned goroutine reached its gate;
			// Done reports that it finished even if its owner was never
			// closed. Wait blocks the orphan until the test releases it, so
			// the read-dense loop below is guaranteed to still be running
			// while the rewrite evals hammer the shared owner — the overlap
			// is structural, not a timing race between goroutine schedules.
			"Entered": reflect.ValueOf(func() { enteredOnce.Do(func() { close(entered) }) }),
			"Wait":    reflect.ValueOf(func() { <-release }),
			"Done":    reflect.ValueOf(func() { doneOnce.Do(func() { close(done) }) }),
		},
	}); err != nil {
		t.Errorf("use ownerrace: %v", err)
		return
	}

	// The orphan goroutine keeps running after Eval has returned. Each
	// interpreted call it makes runs runCfg on a frame rooted at the shared
	// global frame, and the deferred zombie check of that call reads
	// f.root.canceled() — so a dense call loop overlaps those reads with the
	// next Evals rewriting the shared owner in prepareExecutionFrame.
	if _, err := i.Eval(`
import "ownerrace"

func addOne(k int) int {
	return k + 1
}

go func() {
	ownerrace.Entered()
	ownerrace.Wait()
	sum := 0
	for k := 0; k < 1000000; k++ {
		sum = sum + addOne(k)
	}
	ownerrace.Done()
	_ = sum
}()
`); err != nil {
		t.Errorf("orphan eval: %v", err)
		return
	}

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Error("orphaned goroutine did not start")
		return
	}

	// Release the orphan's native gate and hammer the owner rewrite
	// immediately: the orphan's call loop is running from this point on.
	close(release)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := i.Eval(`1 + 1`); err != nil {
			t.Errorf("rewrite eval: %v", err)
			return
		}
		runtime.Gosched()
	}

	// Also rewrite through a context owner, which replaces interp.done with
	// a fresh per-execution channel before discarding it.
	ctx, cancel := context.WithCancel(context.Background())
	_, err := i.EvalWithContext(ctx, `2 + 2`)
	cancel()
	if err != nil {
		t.Errorf("context eval: %v", err)
		return
	}

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Error("orphaned interpreted goroutine did not finish")
		return
	}
}
