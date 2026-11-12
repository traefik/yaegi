// context values: WithValue chains with shadowing, unexported key types so
// that keys of different types do not collide, values read from goroutines,
// values seen through WithCancel children, and the panics of bad keys.
package main

import (
	"context"
	"fmt"
	"sync"
)

type ctxKey string
type otherKey string
type intKey int

type userKey struct{}

type user struct {
	name string
	id   int
}

func main() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKey("a"), 1)
	ctx = context.WithValue(ctx, ctxKey("b"), "two")
	ctx = context.WithValue(ctx, otherKey("a"), "other a")
	ctx = context.WithValue(ctx, intKey(1), 1.5)
	ctx = context.WithValue(ctx, userKey{}, user{"ann", 7})

	fmt.Println(ctx.Value(ctxKey("a")), ctx.Value(ctxKey("b")), ctx.Value(otherKey("a")))
	fmt.Println(ctx.Value(intKey(1)), ctx.Value(intKey(2)), ctx.Value(1), ctx.Value("a"))
	u, ok := ctx.Value(userKey{}).(user)
	fmt.Println(u.name, u.id, ok)
	_, ok = ctx.Value(ctxKey("missing")).(int)
	fmt.Println("missing ok", ok)

	// Shadowing: the nearest value wins.
	shadow := context.WithValue(ctx, ctxKey("a"), 100)
	fmt.Println(shadow.Value(ctxKey("a")), ctx.Value(ctxKey("a")))

	// Values pass through cancel contexts.
	cctx, cancel := context.WithCancel(shadow)
	cancel()
	fmt.Println(cctx.Value(ctxKey("a")), cctx.Value(ctxKey("b")), cctx.Err())

	// A chain of 50 values.
	chain := context.Background()
	for i := 0; i < 50; i++ {
		chain = context.WithValue(chain, intKey(i), i*i)
	}
	sum := 0
	for i := 0; i < 50; i++ {
		sum += chain.Value(intKey(i)).(int)
	}
	fmt.Println("chain sum", sum)

	// Read from goroutines.
	results := make([]string, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := context.WithValue(ctx, ctxKey("worker"), i)
			results[i] = fmt.Sprintf("%v/%v", c.Value(ctxKey("worker")), c.Value(ctxKey("b")))
		}()
	}
	wg.Wait()
	fmt.Println(results)

	// nil value is stored and found.
	nctx := context.WithValue(context.Background(), ctxKey("nil"), nil)
	fmt.Println(nctx.Value(ctxKey("nil")) == nil)

	// Bad keys panic.
	try := func(name string, f func()) {
		defer func() { fmt.Println(name, "recovered:", recover()) }()
		f()
	}
	try("nil key", func() { context.WithValue(context.Background(), nil, 1) })
	try("slice key", func() { context.WithValue(context.Background(), []int{1}, 1) })
	try("nil parent", func() { context.WithValue(nil, ctxKey("x"), 1) })
	try("nil parent cancel", func() { context.WithCancel(nil) })
}
