// context with program types: WithValue keys of program types (two key
// types with the same underlying value do not collide), values that are
// program structs, WithCancelCause and Cause with program errors,
// AfterFunc, WithoutCancel, and context errors printed.
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type userKey struct{}
type traceKey string
type otherKey string

type User struct {
	Name  string
	Roles []string
}

func (u User) Can(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type StopErr struct{ Reason string }

func (e *StopErr) Error() string { return "stopped: " + e.Reason }

func userFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

func main() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, userKey{}, User{"ann", []string{"admin"}})
	ctx = context.WithValue(ctx, traceKey("id"), "trace-1")
	ctx = context.WithValue(ctx, otherKey("id"), 42)
	u, ok := userFrom(ctx)
	fmt.Println(u, ok, u.Can("admin"), u.Can("root"))
	fmt.Println(ctx.Value(traceKey("id")), ctx.Value(otherKey("id")), ctx.Value("id"), ctx.Value(traceKey("none")))
	shadow := context.WithValue(ctx, userKey{}, User{Name: "bob"})
	u2, _ := userFrom(shadow)
	u1, _ := userFrom(ctx)
	fmt.Println(u2.Name, u1.Name)
	_, ok = userFrom(context.TODO())
	fmt.Println(ok)

	cctx, cancel := context.WithCancelCause(ctx)
	var wg sync.WaitGroup
	results := make([]string, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-cctx.Done()
			results[i] = fmt.Sprintf("worker %d: %v / %v", i, cctx.Err(), context.Cause(cctx))
		}(i)
	}
	stopped := make(chan string, 1)
	stop := context.AfterFunc(cctx, func() { stopped <- "afterfunc ran" })
	cancel(&StopErr{"shutdown"})
	cancel(errors.New("second cause ignored"))
	wg.Wait()
	for _, r := range results {
		fmt.Println(r)
	}
	fmt.Println(<-stopped, stop())
	var se *StopErr
	fmt.Println(errors.As(context.Cause(cctx), &se), se.Reason, errors.Is(cctx.Err(), context.Canceled))
	child, childCancel := context.WithCancel(cctx)
	defer childCancel()
	fmt.Println(child.Err(), context.Cause(child))
	nc := context.WithoutCancel(cctx)
	fmt.Println(nc.Err(), nc.Done() == nil, nc.Value(traceKey("id")))
	tctx, tcancel := context.WithTimeoutCause(ctx, time.Nanosecond, &StopErr{"too slow"})
	<-tctx.Done()
	fmt.Println(tctx.Err(), context.Cause(tctx), errors.Is(tctx.Err(), context.DeadlineExceeded))
	tcancel()
	dctx, dcancel := context.WithDeadline(ctx, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	d, has := dctx.Deadline()
	fmt.Println(dctx.Err(), d, has)
	dcancel()
	_, has = ctx.Deadline()
	fmt.Println(has, context.Cause(ctx))
	c2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	fmt.Println(c2.Err(), context.Cause(c2))
	stop2 := context.AfterFunc(context.Background(), func() {})
	fmt.Println(stop2())
	fmt.Println(fmt.Sprint(context.Background()), fmt.Sprint(context.TODO()))
}
