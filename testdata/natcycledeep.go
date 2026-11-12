package main

// Values that contain themselves, inside a struct, coming back from a
// native container (sync.Map, atomic.Value). The struct is converted back
// by fromNativeDeepC, which had no cycle check for maps and slices: a
// type M map[string]M with m["self"] = m, a type T []T with t[0] = t, or a
// struct R with r.M["self"] = r never ended and killed the host with a
// fatal Go stack overflow (default, -aot and -restricted). The VM value
// now has the same cycle.

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type M map[string]M

type H struct{ M M }

type T []T

type HT struct{ T T }

type R struct {
	N int
	M map[string]R
}

type MP map[string]*MP

type HP struct{ P MP }

type S struct {
	Name string
	L    []any
}

func main() {
	m := M{"leaf": nil}
	m["self"] = m
	var av atomic.Value
	av.Store(H{m})
	h2 := av.Load().(H)
	fmt.Println(len(h2.M["self"]["self"]))

	var sm sync.Map
	sm.Store("k", H{m})
	v, _ := sm.Load("k")
	h := v.(H)
	fmt.Println(len(h.M), len(h.M["self"]["self"]), h.M["leaf"] == nil)
	h.M["self"]["new"] = M{} // the cycle is the same map
	fmt.Println(len(h.M))

	t := T{nil, T{}}
	t[0] = t
	var at atomic.Value
	at.Store(HT{t})
	g := at.Load().(HT)
	fmt.Println(len(g.T), len(g.T[0][0]), len(g.T[1]))
	g.T[0][1] = T{nil, nil, nil} // the cycle is the same array
	fmt.Println(len(g.T[1]))
	sm.Store("t", HT{t})
	v, _ = sm.Load("t")
	fmt.Println(len(v.(HT).T[0][0][0]))

	r := R{N: 7, M: map[string]R{}}
	r.M["self"] = r
	var ar atomic.Value
	ar.Store(r)
	gr := ar.Load().(R)
	fmt.Println(gr.N, len(gr.M), gr.M["self"].N, len(gr.M["self"].M["self"].M))
	gr.M["self"].M["x"] = R{N: 1} // the cycle is the same map
	fmt.Println(len(gr.M))
	sm.Store("r", r)
	v, _ = sm.Load("r")
	fmt.Println(v.(R).M["self"].M["self"].N)

	mp := MP{}
	mp["self"] = &mp
	sm.Store("p", HP{mp})
	v, _ = sm.Load("p")
	hp := v.(HP)
	fmt.Println(len(hp.P), len(*(*hp.P["self"])["self"]))

	l := []any{"a", nil}
	l[1] = l
	sm.Store("s", S{Name: "s", L: l})
	v, _ = sm.Load("s")
	s := v.(S)
	fmt.Println(s.Name, len(s.L), s.L[1].([]any)[0])
}
