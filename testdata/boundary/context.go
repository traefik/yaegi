// Native boundary of context: program types given to context (docs/BOUNDARY.md).
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// plainCtx is a program context without AfterFunc: context watches it
// with a goroutine. Its Done channel comes from a context of the standard
// library, but its Value method hides that context.
type plainCtx struct {
	inner context.Context
	stop  context.CancelCauseFunc
}

func newPlain() *plainCtx {
	c, f := context.WithCancelCause(context.Background())
	return &plainCtx{inner: c, stop: f}
}

func (c *plainCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *plainCtx) Done() <-chan struct{}       { return c.inner.Done() }
func (c *plainCtx) Err() error {
	if c.inner.Err() == nil {
		return nil
	}
	return context.Cause(c.inner)
}
func (c *plainCtx) Value(key any) any {
	if s, ok := key.(string); ok && s == "who" {
		return "plain"
	}
	return nil
}
func (c *plainCtx) cancel(err error) { c.stop(err) }

// chanCtx is a program context whose Done channel is made by the program.
type chanCtx struct {
	done chan struct{}
	err  error
}

func (c *chanCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *chanCtx) Done() <-chan struct{}       { return c.done }
func (c *chanCtx) Err() error {
	select {
	case <-c.done:
		return c.err
	default:
		return nil
	}
}
func (c *chanCtx) Value(key any) any { return nil }

// afterCtx is a program context with an AfterFunc method: context calls it.
type afterCtx struct {
	plainCtx
	mu    sync.Mutex
	funcs []func()
	log   []string
}

func newAfter() *afterCtx { return &afterCtx{plainCtx: *newPlain()} }

func (c *afterCtx) AfterFunc(f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = append(c.log, "AfterFunc")
	i := len(c.funcs)
	c.funcs = append(c.funcs, f)
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.log = append(c.log, "stop")
		if c.funcs[i] == nil {
			return false
		}
		c.funcs[i] = nil
		return true
	}
}

func (c *afterCtx) cancelAll(err error) {
	c.plainCtx.cancel(err)
	c.mu.Lock()
	fs := c.funcs
	c.mu.Unlock()
	for _, f := range fs {
		if f != nil {
			f()
		}
	}
}

func (c *afterCtx) logs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.log...)
}

// namedCtx is a program context with a String method.
type namedCtx struct{ *plainCtx }

func (c namedCtx) String() string { return "namedCtx" }

// key types of WithValue.
type key struct{ name string }
type pkey int
type gkey[T any] struct{ v T }
type skey []string
type boxKey struct{ v any }
type strKey struct{ n int }

func (k strKey) String() string { return fmt.Sprintf("strKey(%d)", k.n) }

var errStop = errors.New("parent stopped")

// derive returns the constructor of each With* function.
func derive() []struct {
	name string
	f    func(context.Context) (context.Context, func())
} {
	far := time.Now().Add(time.Hour)
	return []struct {
		name string
		f    func(context.Context) (context.Context, func())
	}{
		{"WithCancel", func(p context.Context) (context.Context, func()) {
			c, f := context.WithCancel(p)
			return c, f
		}},
		{"WithCancelCause", func(p context.Context) (context.Context, func()) {
			c, f := context.WithCancelCause(p)
			return c, func() { f(errors.New("own cause")) }
		}},
		{"WithDeadline", func(p context.Context) (context.Context, func()) {
			c, f := context.WithDeadline(p, far)
			return c, f
		}},
		{"WithDeadlineCause", func(p context.Context) (context.Context, func()) {
			c, f := context.WithDeadlineCause(p, far, errors.New("late"))
			return c, f
		}},
		{"WithTimeout", func(p context.Context) (context.Context, func()) {
			c, f := context.WithTimeout(p, time.Hour)
			return c, f
		}},
		{"WithTimeoutCause", func(p context.Context) (context.Context, func()) {
			c, f := context.WithTimeoutCause(p, time.Hour, errors.New("late"))
			return c, f
		}},
		{"AfterFunc", func(p context.Context) (context.Context, func()) {
			ran := make(chan struct{})
			stop := context.AfterFunc(p, func() { close(ran) })
			c, f := context.WithCancel(context.Background())
			go func() {
				<-ran
				f()
			}()
			return c, func() { out("AfterFunc stop: %v", stop()); f() }
		}},
	}
}

// deriveProbe cancels contexts made by the function name over program
// parents, with and without AfterFunc.
func deriveProbe(name string) {
	for _, d := range derive() {
		if d.name != name {
			continue
		}
		// the parent is canceled first
		a := newAfter()
		c, _ := d.f(a)
		a.cancelAll(errStop)
		<-c.Done()
		out("after: parent canceled: %v %v %v", c.Err(), context.Cause(c), a.logs())
		// the child is canceled first
		a = newAfter()
		c, cancel := d.f(a)
		cancel()
		<-c.Done()
		out("after: child canceled: %v %v", c.Err(), a.logs())
		// the program context inside a value context
		a = newAfter()
		c, cancel = d.f(context.WithValue(a, "k", 1))
		cancel()
		out("value(after): %v %v", c.Err(), a.logs())
		// a context without AfterFunc
		p := newPlain()
		c, cancel = d.f(p)
		p.cancel(errStop)
		<-c.Done()
		out("plain: parent canceled: %v %v", c.Err(), context.Cause(c))
		cancel()
	}
}

func main() {
	// row: context WithCancel assert x.(context.afterFuncer)
	probe("afterFuncer/WithCancel", func() { deriveProbe("WithCancel") })
	// row: context WithCancelCause assert x.(context.afterFuncer)
	probe("afterFuncer/WithCancelCause", func() { deriveProbe("WithCancelCause") })
	// row: context WithDeadline assert x.(context.afterFuncer)
	probe("afterFuncer/WithDeadline", func() { deriveProbe("WithDeadline") })
	// row: context WithDeadlineCause assert x.(context.afterFuncer)
	probe("afterFuncer/WithDeadlineCause", func() { deriveProbe("WithDeadlineCause") })
	// row: context WithTimeout assert x.(context.afterFuncer)
	probe("afterFuncer/WithTimeout", func() { deriveProbe("WithTimeout") })
	// row: context WithTimeoutCause assert x.(context.afterFuncer)
	probe("afterFuncer/WithTimeoutCause", func() { deriveProbe("WithTimeoutCause") })
	// row: context AfterFunc assert x.(context.afterFuncer)
	probe("afterFuncer/AfterFunc", func() { deriveProbe("AfterFunc") })
	// row: context WithCancel assert x.(context.afterFuncer)
	probe("afterFuncer/chan", func() {
		p := &chanCtx{done: make(chan struct{})}
		c, cancel := context.WithCancel(p)
		p.err = errStop
		close(p.done)
		<-c.Done()
		out("chan: %v %v", c.Err(), context.Cause(c))
		cancel()
	})
	// row: context WithCancel,WithDeadline assert x.(context.afterFuncer)
	probe("afterFuncer/done", func() {
		// a parent that is already done, and a past deadline
		a := newAfter()
		a.cancelAll(errStop)
		c, cancel := context.WithCancel(a)
		out("done parent: %v %v %v", c.Err(), context.Cause(c), a.logs())
		cancel()
		a = newAfter()
		c, cancel = context.WithDeadline(a, time.Now().Add(-time.Second))
		out("past deadline: %v %v", c.Err(), a.logs())
		cancel()
		out("value through: %v", context.WithValue(newPlain(), "x", 1).Value("who"))
	})
	// row: context WithValue reflect comparability
	// row: context WithValue reflect walk
	probe("WithValue", func() {
		ctx := context.Background()
		k1, k2 := key{"a"}, &key{"b"}
		ctx = context.WithValue(ctx, k1, "v1")
		ctx = context.WithValue(ctx, k2, "v2")
		ctx = context.WithValue(ctx, pkey(3), "v3")
		ctx = context.WithValue(ctx, gkey[int]{4}, "v4")
		ctx = context.WithValue(ctx, boxKey{"s"}, "v5")
		ctx = context.WithValue(ctx, strKey{6}, strKey{7})
		out("%v %v %v %v %v %v", ctx.Value(key{"a"}), ctx.Value(k2), ctx.Value(pkey(3)), ctx.Value(gkey[int]{4}), ctx.Value(boxKey{"s"}), ctx.Value(strKey{6}))
		out("misses: %v %v %v %v", ctx.Value(key{"b"}), ctx.Value(&key{"b"}), ctx.Value(3), ctx.Value(gkey[int64]{4}))
		for _, k := range []any{skey{"x"}, map[string]int{}, func() {}} {
			func() {
				defer func() { out("%T: panic %v", k, recover()) }()
				_ = context.WithValue(context.Background(), k, 1)
			}()
		}
		func() {
			defer func() { out("nil key: panic %v", recover()) }()
			_ = context.WithValue(context.Background(), nil, 1)
		}()
		// a comparable key type that holds an uncomparable value
		func() {
			defer func() {
				r := recover()
				out("box slice: panic %T %v", r, r)
			}()
			c := context.WithValue(context.Background(), boxKey{[]int{1}}, 1)
			out("box slice: %v", c.Value(boxKey{[]int{1}}))
		}()
	})
	// row: context WithValue reflect walk
	probe("String/value", func() {
		ctx := context.WithValue(context.Background(), key{"a"}, 1)
		out("%v", ctx)
		ctx = context.WithValue(ctx, strKey{2}, strKey{3})
		out("%v", ctx)
		ctx = context.WithValue(ctx, pkey(1), gkey[string]{"x"})
		out("%v", ctx)
		ctx = context.WithValue(ctx, &key{"p"}, "str")
		out("%v", ctx)
	})
	// row: context WithValue reflect walk
	probe("String/program", func() {
		c, cancel := context.WithCancel(newPlain())
		out("%v", c)
		cancel()
		c, cancel = context.WithCancel(namedCtx{newPlain()})
		out("%v", c)
		cancel()
		out("%v", context.WithValue(newAfter(), "k", "v"))
		out("%v", context.WithoutCancel(namedCtx{newPlain()}))
	})
}
