// sync.Pool, sync.Map, atomic.Value and context values with non-pointer
// named interpreted types.
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type Buf [4]byte
type Pair struct {
	A int
	B string
}
type Key struct{ name string }
type Slice []int

func main() {
	var p sync.Pool
	p.New = func() any { return Pair{1, "new"} }
	v := p.Get().(Pair)
	fmt.Println(v)
	p.Put(Pair{2, "put"})
	got := p.Get()
	pv, ok := got.(Pair)
	fmt.Println(ok, pv.A == 1 || pv.A == 2)

	p2 := sync.Pool{New: func() any { return Buf{} }}
	b := p2.Get().(Buf)
	b[0] = 7
	p2.Put(b)
	b2 := p2.Get().(Buf)
	fmt.Println(b2[0] == 0 || b2[0] == 7, len(b2))

	p3 := sync.Pool{New: func() any { return Slice{} }}
	s := p3.Get().(Slice)
	s = append(s, 1)
	p3.Put(s)
	fmt.Printf("%T\n", p3.Get())

	var m sync.Map
	m.Store(Key{"a"}, Pair{3, "x"})
	m.Store(Key{"b"}, Buf{1})
	val, ok := m.Load(Key{"a"})
	fmt.Println(val.(Pair).B, ok)
	val, ok = m.Load(Key{"c"})
	fmt.Println(val, ok)
	act, loaded := m.LoadOrStore(Key{"b"}, 0)
	fmt.Printf("%T %v\n", act, loaded)
	n := 0
	m.Range(func(k, v any) bool { _ = k.(Key); n++; return true })
	fmt.Println(n)

	var av atomic.Value
	av.Store(Pair{4, "atomic"})
	fmt.Println(av.Load().(Pair).A)
	fmt.Println(av.CompareAndSwap(Pair{4, "atomic"}, Pair{5, "cas"}), av.Load())

	var ap atomic.Pointer[Pair]
	ap.Store(&Pair{6, "ptr"})
	fmt.Println(ap.Load().B)

	ctx := context.WithValue(context.Background(), Key{"k"}, Pair{7, "ctx"})
	ctx = context.WithValue(ctx, Key{"s"}, Slice{1, 2})
	fmt.Println(ctx.Value(Key{"k"}), ctx.Value(Key{"s"}).(Slice), ctx.Value(Key{"none"}))
	type local string
	ctx = context.WithValue(ctx, local("l"), 1)
	fmt.Println(ctx.Value(local("l")), ctx.Value("l"))
}
