// An errgroup written by hand: Go starts tasks, Wait returns the first error
// (by a fixed rule, the error of the smallest task index, so that the result
// does not depend on the scheduler), the context is canceled on error, and a
// limit on the number of tasks running at once.
package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

type Group struct {
	wg     sync.WaitGroup
	mu     sync.Mutex
	errs   map[int]error
	cancel context.CancelCauseFunc
	sem    chan struct{}
	next   int
}

func WithContext(ctx context.Context) (*Group, context.Context) {
	ctx, cancel := context.WithCancelCause(ctx)
	return &Group{cancel: cancel, errs: map[int]error{}}, ctx
}

func (g *Group) SetLimit(n int) { g.sem = make(chan struct{}, n) }

func (g *Group) Go(f func() error) {
	idx := g.next
	g.next++
	if g.sem != nil {
		g.sem <- struct{}{}
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer func() {
			if g.sem != nil {
				<-g.sem
			}
		}()
		if err := f(); err != nil {
			g.mu.Lock()
			g.errs[idx] = err
			g.mu.Unlock()
			if g.cancel != nil {
				g.cancel(err)
			}
		}
	}()
}

// Wait returns the error of the smallest index and the number of errors.
func (g *Group) Wait() (error, int) {
	g.wg.Wait()
	if g.cancel != nil {
		g.cancel(nil)
	}
	if len(g.errs) == 0 {
		return nil, 0
	}
	keys := make([]int, 0, len(g.errs))
	for k := range g.errs {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return g.errs[keys[0]], len(keys)
}

var errOdd = errors.New("odd input")

func main() {
	// All succeed.
	g := &Group{errs: map[int]error{}}
	out := make([]int, 10)
	for i := 0; i < 10; i++ {
		g.Go(func() error {
			out[i] = i * 3
			return nil
		})
	}
	err, n := g.Wait()
	fmt.Println(out, err, n)

	// Several fail: the error of the first index wins.
	g2 := &Group{errs: map[int]error{}}
	for i := 0; i < 10; i++ {
		g2.Go(func() error {
			if i%3 == 2 {
				return fmt.Errorf("task %d: %w", i, errOdd)
			}
			return nil
		})
	}
	err, n = g2.Wait()
	fmt.Println(err, n, errors.Is(err, errOdd))

	// With context: a failing task cancels the others, which wait on Done.
	g3, ctx := WithContext(context.Background())
	var canceled atomic.Int32
	ready := make(chan struct{})
	for i := 0; i < 4; i++ {
		g3.Go(func() error {
			<-ctx.Done()
			canceled.Add(1)
			return nil
		})
	}
	g3.Go(func() error {
		close(ready)
		return errors.New("failure")
	})
	<-ready
	err, n = g3.Wait()
	fmt.Println(err, n, canceled.Load(), ctx.Err(), context.Cause(ctx))

	// With context and no error: the context is canceled by Wait with
	// Canceled as cause.
	g4, ctx4 := WithContext(context.Background())
	g4.Go(func() error { return nil })
	err, n = g4.Wait()
	fmt.Println(err, n, ctx4.Err(), context.Cause(ctx4))

	// Limit: at most 3 tasks at the same time.
	g5 := &Group{errs: map[int]error{}}
	g5.SetLimit(3)
	var running, peak atomic.Int32
	var total atomic.Int64
	for i := 0; i < 30; i++ {
		g5.Go(func() error {
			r := running.Add(1)
			for {
				p := peak.Load()
				if r <= p || peak.CompareAndSwap(p, r) {
					break
				}
			}
			total.Add(int64(i))
			running.Add(-1)
			return nil
		})
	}
	err, n = g5.Wait()
	fmt.Println(err, n, total.Load(), peak.Load() <= 3, peak.Load() >= 1, running.Load())

	// A joined error of all failures, sorted by index.
	g6 := &Group{errs: map[int]error{}}
	for i := 0; i < 5; i++ {
		g6.Go(func() error {
			if i%2 == 0 {
				return fmt.Errorf("e%d", i)
			}
			return nil
		})
	}
	g6.Wait()
	keys := []int{}
	for k := range g6.errs {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	var all []error
	for _, k := range keys {
		all = append(all, g6.errs[k])
	}
	fmt.Printf("%q\n", errors.Join(all...).Error())
}
