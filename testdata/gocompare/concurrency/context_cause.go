// context causes: WithCancelCause with a custom error and with nil, Cause of
// a parent seen by its children, the first cause wins, Cause of a context
// that is not canceled or that is canceled by a plain cancel, and
// WithDeadlineCause with a deadline in the past.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type myErr struct{ code int }

func (e *myErr) Error() string { return fmt.Sprintf("my error %d", e.code) }

func main() {
	bg := context.Background()
	fmt.Println("bg cause", context.Cause(bg))

	ctx, cancel := context.WithCancelCause(bg)
	child, childCancel := context.WithCancel(ctx)
	defer childCancel()
	fmt.Println("before", ctx.Err(), context.Cause(ctx), context.Cause(child))
	e1 := &myErr{42}
	cancel(e1)
	cancel(errors.New("second cause ignored"))
	fmt.Println("after", ctx.Err(), context.Cause(ctx), context.Cause(child))
	var me *myErr
	fmt.Println(errors.As(context.Cause(child), &me), me.code, context.Cause(ctx) == e1)

	// nil cause: Cause is Canceled.
	c2, cancel2 := context.WithCancelCause(bg)
	cancel2(nil)
	fmt.Println("nil cause", c2.Err(), context.Cause(c2), context.Cause(c2) == context.Canceled)

	// Plain cancel: Cause is Err.
	c3, cancel3 := context.WithCancel(bg)
	cancel3()
	fmt.Println("plain", context.Cause(c3))

	// Child canceled with its own cause before the parent.
	parent, pcancel := context.WithCancelCause(bg)
	kid, kcancel := context.WithCancelCause(parent)
	kcancel(errors.New("kid cause"))
	pcancel(errors.New("parent cause"))
	fmt.Println(context.Cause(kid), "|", context.Cause(parent))

	// Wrapped cause.
	base := errors.New("base")
	c4, cancel4 := context.WithCancelCause(bg)
	cancel4(fmt.Errorf("wrapped: %w", base))
	fmt.Println(context.Cause(c4), errors.Is(context.Cause(c4), base), errors.Is(c4.Err(), context.Canceled))

	// Cause through a value context.
	vctx := context.WithValue(c4, "k", 1)
	fmt.Println("through value", context.Cause(vctx))

	// Cause in a goroutine that waits on Done.
	c5, cancel5 := context.WithCancelCause(bg)
	res := make(chan string)
	go func() {
		<-c5.Done()
		res <- fmt.Sprint(c5.Err(), " / ", context.Cause(c5))
	}()
	cancel5(errors.New("stop now"))
	fmt.Println(<-res)

	// Deadline and timeout causes with a time already in the past.
	past := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	dc, dcancel := context.WithDeadlineCause(bg, past, errors.New("too late"))
	defer dcancel()
	fmt.Println(dc.Err(), "|", context.Cause(dc))
	tc, tcancel := context.WithTimeoutCause(bg, 0, errors.New("no time"))
	defer tcancel()
	fmt.Println(tc.Err(), "|", context.Cause(tc))

	// WithoutCancel keeps no cause.
	wc := context.WithoutCancel(c4)
	fmt.Println("without cancel", wc.Err(), context.Cause(wc))
}
