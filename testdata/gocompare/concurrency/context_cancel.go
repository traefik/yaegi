// context cancellation without timing: WithCancel trees (a parent cancels
// its children, a child does not cancel its parent), Err values, Done
// channels, cancel called twice, goroutines that stop on Done, a deadline
// already in the past, and WithoutCancel.
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

func state(name string, ctx context.Context) {
	select {
	case <-ctx.Done():
		fmt.Println(name, "done:", ctx.Err())
	default:
		fmt.Println(name, "alive:", ctx.Err())
	}
}

func main() {
	bg := context.Background()
	fmt.Println(bg, context.TODO())
	fmt.Println(bg.Done() == nil, bg.Err())
	_, has := bg.Deadline()
	fmt.Println("bg deadline", has)

	root, cancelRoot := context.WithCancel(bg)
	a, cancelA := context.WithCancel(root)
	b, cancelB := context.WithCancel(root)
	a1, cancelA1 := context.WithCancel(a)
	defer cancelA1()
	defer cancelB()

	cancelA()
	state("root", root)
	state("a", a)
	state("a1", a1)
	state("b", b)

	cancelRoot()
	cancelRoot()
	state("root", root)
	state("b", b)
	fmt.Println(errors.Is(b.Err(), context.Canceled), b.Err() == context.Canceled)

	// A child of a canceled context is canceled at once.
	late, cancelLate := context.WithCancel(root)
	state("late", late)
	cancelLate()

	// Goroutines stop on Done; each reports how far it got.
	ctx, cancel := context.WithCancel(bg)
	var wg sync.WaitGroup
	stopped := make([]bool, 5)
	started := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started <- struct{}{}
			<-ctx.Done()
			stopped[i] = ctx.Err() == context.Canceled
		}()
	}
	for i := 0; i < 5; i++ {
		<-started
	}
	cancel()
	wg.Wait()
	fmt.Println("stopped", stopped)

	// Generator that stops when canceled.
	gctx, gcancel := context.WithCancel(bg)
	gen := func(ctx context.Context) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for n := 0; ; n++ {
				select {
				case out <- n:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}
	sum := 0
	g := gen(gctx)
	for v := range g {
		sum += v
		if v == 10 {
			gcancel()
			break
		}
	}
	for range g {
	}
	fmt.Println("gen sum", sum)

	// A deadline in the past: canceled at once with DeadlineExceeded.
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	dctx, dcancel := context.WithDeadline(bg, past)
	defer dcancel()
	state("past deadline", dctx)
	dl, ok := dctx.Deadline()
	fmt.Println(dl.Equal(past), ok, errors.Is(dctx.Err(), context.DeadlineExceeded))
	var de interface{ Timeout() bool }
	if errors.As(dctx.Err(), &de) {
		fmt.Println("timeout", de.Timeout())
	}
	tctx, tcancel := context.WithTimeout(bg, -time.Second)
	defer tcancel()
	state("negative timeout", tctx)

	// WithoutCancel.
	p, pcancel := context.WithCancel(context.WithValue(bg, "k", "v"))
	wc := context.WithoutCancel(p)
	pcancel()
	state("parent", p)
	state("without cancel", wc)
	fmt.Println(wc.Value("k"), wc.Done() == nil)

	// Errors strings.
	fmt.Println(context.Canceled.Error(), "|", context.DeadlineExceeded.Error())
}
