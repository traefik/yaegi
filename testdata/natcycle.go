package main

// Native values that contain themselves coming back into the VM: a []any
// with s[0] = s, or a map[string]any with m["me"] = m, stored in a native
// container (sync.Pool, atomic.Value, container/list) and read again. The
// conversion of a native slice to a VM slice (natToVM) had no cycle check:
// in default mode it never ended and killed the host with a fatal Go stack
// overflow. It now gives back the VM value being built (natSeen in
// vm/twin_value.go), so the VM value has the same cycle.

import (
	"container/list"
	"fmt"
	"sync"
	"sync/atomic"
)

// viaPool puts v in a sync.Pool and gets it back. Put stores it in the
// private slot of the current P, which no other P can take: when the
// goroutine moves to another P (or a GC runs) between Put and Get, Get
// returns nil (the -budget run of this test failed so in CI). Put and Get
// again until the value comes back.
func viaPool(pool *sync.Pool, v any) any {
	for {
		pool.Put(v)
		if g := pool.Get(); g != nil {
			return g
		}
	}
}

func main() {
	var pool sync.Pool
	s := []any{nil, 1}
	s[0] = s
	g := viaPool(&pool, s).([]any)
	g0 := g[0].([]any)
	fmt.Println(len(g), g[1], len(g0), g0[1], len(g0[0].([]any)))
	g0[1] = 2 // the cycle is the same array
	fmt.Println(g[1])

	m := map[string]any{"n": 1}
	m["me"] = m
	gm := viaPool(&pool, m).(map[string]any)
	fmt.Println(len(gm), gm["n"], len(gm["me"].(map[string]any)))

	// a cycle through a map and a slice
	mm := map[string]any{}
	ss := []any{mm, "x"}
	mm["s"] = ss
	gs := viaPool(&pool, ss).([]any)
	fmt.Println(len(gs), gs[1], len(gs[0].(map[string]any)["s"].([]any)))

	var av atomic.Value
	t := []any{nil}
	t[0] = t
	av.Store(t)
	at := av.Load().([]any)
	fmt.Println(len(at), len(at[0].([]any)[0].([]any)))

	l := list.New()
	u := []any{"a", nil}
	u[1] = u
	l.PushBack(u)
	lu := l.Front().Value.([]any)
	fmt.Println(lu[0], lu[1].([]any)[1].([]any)[0])
}
