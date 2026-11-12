package main

import (
	"container/list"
	"context"
	"fmt"
	"sync"
)

// Pointers to interpreted types stored by native code come back with their
// type and identity.

type Item struct {
	key   string
	value int
}

type Pair[K comparable, V any] struct {
	k K
	v V
}

type ctxKey struct{}

func main() {
	l := list.New()
	it := &Item{"a", 1}
	l.PushBack(it)
	l.PushBack(&Pair[string, int]{"b", 2})
	it.value = 10
	got, ok := l.Front().Value.(*Item)
	fmt.Println(got.value, ok, got == it)
	p, ok := l.Back().Value.(*Pair[string, int])
	fmt.Println(p.k, p.v, ok)

	ctx := context.WithValue(context.Background(), ctxKey{}, it)
	fmt.Println(ctx.Value(ctxKey{}).(*Item) == it)

	var m sync.Map
	m.Store("x", it)
	v, _ := m.Load("x")
	v.(*Item).value++
	fmt.Println(it.value)
}
