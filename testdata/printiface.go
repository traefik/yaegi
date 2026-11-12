package main

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// println of interfaces, pointers and slices prints addresses, which differ
// between runs and between gc and the VM: the test (maskAddrs in
// cmd/runtests) replaces each non-zero address by "addr" before the
// comparison, and keeps 0x0. So this program checks that a nil
// pointer-shaped value in an interface (a typed nil) prints (addr,0x0) like
// gc, and that a non-nil value never prints 0x0, for interpreted values and
// for values that come from native code.

type myErr struct{}

func (*myErr) Error() string { return "myErr" }

type onePtr struct{ p *int }

type key struct{}

func main() {
	n := 1
	var av atomic.Value
	av.Store(42)
	ctx := context.WithValue(context.Background(), key{}, "v")
	var nilErr *myErr

	// Typed nils: the data word is 0x0.
	println(any((*int)(nil)), any(map[string]int(nil)), any((chan int)(nil)), any((func())(nil)))
	println(error(nilErr), any(onePtr{}), any([1]*int{}))
	// Non-nil values: the data word is never 0x0.
	println(any(&n), any(onePtr{&n}), any([1]*int{&n}), any(map[string]int{}), any(main))
	println(any(1), any("s"), any(1.5), any(struct{ a, b int }{}), any([]int{1}))
	println(errors.New("native"), any(time.Second), av.Load(), ctx.Value(key{}))
	println(error(&myErr{}), any(errors.New("x")), any(context.Background()))
	// Nil interfaces, pointers and slices.
	println(any(nil), error(nil), (*int)(nil), []int(nil), &n, []int{1, 2}[:1])
}
