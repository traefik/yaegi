// sync.Map, sync.Pool and atomic.Value keep the sharing of Go: a struct value
// comes back as a copy, but its slice, map and pointer fields still point to
// the same memory, and a stored pointer is the same pointer. Load then
// mutate then Load again, LoadOrStore, Swap, CompareAndSwap and
// CompareAndDelete (comparison of struct values), Range, Pool Get/Put of
// *bytes.Buffer and of struct values, atomic.Value with inconsistent types,
// struct keys with pointer fields, and concurrent goroutines.

package main

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

type Inner struct{ N int }

type Rec struct {
	Name  string
	Tags  []string
	Attrs map[string]int
	Ptr   *Inner
	Arr   [2]int
}

type Key struct {
	A string
	B int
}

type Pt struct{ X, Y int }

func structValues() {
	var m sync.Map
	tags := []string{"a", "b"}
	attrs := map[string]int{"x": 1}
	in := &Inner{N: 1}
	r := Rec{Name: "r", Tags: tags, Attrs: attrs, Ptr: in, Arr: [2]int{1, 2}}
	m.Store("r", r)

	// The original changes after Store: the shared parts follow.
	tags[0] = "A"
	attrs["y"] = 2
	in.N = 10
	r.Name = "changed"
	r.Arr[0] = 100

	v, _ := m.Load("r")
	got := v.(Rec)
	fmt.Println(got.Name, got.Tags, got.Attrs, got.Ptr.N, got.Arr)

	// Mutate what was loaded, then Load again.
	got.Tags[1] = "B"
	got.Attrs["z"] = 3
	got.Ptr.N = 20
	got.Name = "local"
	got.Arr[1] = 200
	v2, _ := m.Load("r")
	got2 := v2.(Rec)
	fmt.Println(got2.Name, got2.Tags, got2.Attrs, got2.Ptr.N, got2.Arr)
	fmt.Println(tags, attrs, in.N)
	fmt.Println(got.Ptr == got2.Ptr, got2.Ptr == in, &got.Tags[0] == &got2.Tags[0], &tags[0] == &got2.Tags[0])

	// Append past the capacity does not change what is stored.
	got.Tags = append(got.Tags, "c", "d", "e")
	v3, _ := m.Load("r")
	fmt.Println(v3.(Rec).Tags)
}

func pointers() {
	var m sync.Map
	p := &Rec{Name: "p", Tags: []string{"t"}}
	m.Store("p", p)
	v, _ := m.Load("p")
	v.(*Rec).Name = "q"
	v.(*Rec).Tags[0] = "u"
	fmt.Println(p.Name, p.Tags, v.(*Rec) == p)
	p.Name = "r"
	v2, _ := m.Load("p")
	fmt.Println(v2.(*Rec).Name, v2 == v)

	// A slice and a map stored as is.
	s := []int{1, 2, 3}
	mp := map[string]int{"a": 1}
	m.Store("s", s)
	m.Store("m", mp)
	vs, _ := m.Load("s")
	vs.([]int)[0] = 9
	vm, _ := m.Load("m")
	vm.(map[string]int)["b"] = 2
	fmt.Println(s, mp)
}

func loadOrStore() {
	var m sync.Map
	tags := []string{"x"}
	act, loaded := m.LoadOrStore("k", Rec{Name: "first", Tags: tags})
	fmt.Println(act.(Rec).Name, loaded)
	act.(Rec).Tags[0] = "y"
	fmt.Println(tags)
	act2, loaded2 := m.LoadOrStore("k", Rec{Name: "second"})
	fmt.Println(act2.(Rec).Name, act2.(Rec).Tags, loaded2)
	act3, loaded3 := m.LoadOrStore("p", &Inner{N: 5})
	act4, loaded4 := m.LoadOrStore("p", &Inner{N: 6})
	act3.(*Inner).N++
	fmt.Println(act3.(*Inner).N, loaded3, act4.(*Inner).N, loaded4, act3 == act4)
}

func swap() {
	var m sync.Map
	tags := []string{"1"}
	prev, loaded := m.Swap("k", Rec{Name: "a", Tags: tags})
	fmt.Println(prev, loaded)
	prev, loaded = m.Swap("k", Rec{Name: "b"})
	prev.(Rec).Tags[0] = "2"
	fmt.Println(prev.(Rec).Name, loaded, tags)
	v, ok := m.LoadAndDelete("k")
	fmt.Println(v.(Rec).Name, ok)
	_, ok = m.Load("k")
	fmt.Println(ok)
}

func compare() {
	var m sync.Map
	m.Store(Key{"a", 1}, Pt{1, 2})
	fmt.Println(m.CompareAndSwap(Key{"a", 1}, Pt{1, 3}, Pt{9, 9}))
	fmt.Println(m.CompareAndSwap(Key{"a", 1}, Pt{1, 2}, Pt{5, 6}))
	v, _ := m.Load(Key{"a", 1})
	fmt.Println(v)
	fmt.Println(m.CompareAndDelete(Key{"a", 1}, Pt{1, 2}))
	fmt.Println(m.CompareAndDelete(Key{"a", 1}, Pt{5, 6}))
	_, ok := m.Load(Key{"a", 1})
	fmt.Println(ok)
	// A pointer compares by identity.
	p := &Pt{1, 1}
	m.Store("p", p)
	fmt.Println(m.CompareAndSwap("p", &Pt{1, 1}, &Pt{2, 2}))
	fmt.Println(m.CompareAndSwap("p", p, &Pt{3, 3}))
	v, _ = m.Load("p")
	fmt.Println(*v.(*Pt))
	// A struct with a pointer field compares the pointer.
	type withPtr struct {
		P *Pt
		S string
	}
	q := &Pt{4, 4}
	m.Store("w", withPtr{q, "s"})
	fmt.Println(m.CompareAndSwap("w", withPtr{&Pt{4, 4}, "s"}, withPtr{nil, "t"}))
	fmt.Println(m.CompareAndSwap("w", withPtr{q, "s"}, withPtr{q, "t"}))
	v, _ = m.Load("w")
	v.(withPtr).P.X = 40
	fmt.Println(v.(withPtr).S, *q)
	// An uncomparable struct value as old panics as in Go.
	m.Store("r", Rec{Name: "r"})
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(m.CompareAndSwap("r", Rec{Name: "r"}, Rec{}))
	}()
	// Struct keys: equal values find the same entry.
	m.Store(Key{"k", 2}, 1)
	m.Store(Key{"k", 2}, 2)
	v, _ = m.Load(Key{"k", 2})
	fmt.Println(v)
}

type PtrKey struct {
	P *Inner
	S string
}

type IfKey struct{ I any }

type FKey struct{ F float64 }

func structKeys() {
	var m sync.Map
	p := &Inner{N: 1}
	m.Store(PtrKey{p, "a"}, 1)
	v, ok := m.Load(PtrKey{p, "a"})
	fmt.Println(v, ok)
	_, ok = m.Load(PtrKey{&Inner{N: 1}, "a"})
	fmt.Println(ok)
	m.Range(func(k, v any) bool {
		k.(PtrKey).P.N = 7
		fmt.Println(k.(PtrKey).P == p)
		return true
	})
	fmt.Println(p.N)
	m.Store(IfKey{p}, 2)
	v, ok = m.Load(IfKey{p})
	fmt.Println(v, ok)
	m.Store([2]*Inner{p, nil}, 3)
	v, ok = m.Load([2]*Inner{p, nil})
	fmt.Println(v, ok)
	fmt.Println(m.CompareAndSwap(PtrKey{p, "a"}, 1, 10), m.CompareAndDelete([2]*Inner{p, nil}, 3))
	v, _ = m.Load(PtrKey{p, "a"})
	fmt.Println(v)
	_, ok = m.Load([2]*Inner{p, nil})
	fmt.Println(ok)
	prev, loaded := m.Swap(IfKey{p}, 20)
	fmt.Println(prev, loaded)
	v, loaded = m.LoadAndDelete(IfKey{p})
	fmt.Println(v, loaded)

	// +0 and -0 are the same key; NaN is never found.
	var f sync.Map
	negZero := math.Copysign(0, -1)
	f.Store(FKey{0}, "zero")
	v, ok = f.Load(FKey{negZero})
	fmt.Println(v, ok)
	f.Store(FKey{math.NaN()}, "nan")
	_, ok = f.Load(FKey{math.NaN()})
	fmt.Println(ok)
	n := 0
	f.Range(func(k, v any) bool { n++; return true })
	fmt.Println(n)
}

func rangeMap() {
	var m sync.Map
	shared := map[string]int{}
	for i := 0; i < 3; i++ {
		m.Store(i, Rec{Name: fmt.Sprint("n", i), Attrs: shared, Ptr: &Inner{N: i}})
	}
	var names []string
	m.Range(func(k, v any) bool {
		r := v.(Rec)
		r.Attrs[r.Name] = k.(int)
		r.Ptr.N += 100
		names = append(names, r.Name)
		return true
	})
	sort.Strings(names)
	fmt.Println(names, shared)
	var ns []int
	m.Range(func(k, v any) bool {
		ns = append(ns, v.(Rec).Ptr.N)
		return true
	})
	sort.Ints(ns)
	fmt.Println(ns)
}

func pool() {
	p := sync.Pool{New: func() any { return new(bytes.Buffer) }}
	b := p.Get().(*bytes.Buffer)
	b.WriteString("hello")
	p.Put(b)
	// The pool may drop b: only check what Go always does.
	b2 := p.Get().(*bytes.Buffer)
	b2.Reset()
	b2.WriteString("x")
	fmt.Println(b2.String())

	tags := []string{"t"}
	sp := sync.Pool{}
	sp.Put(Rec{Name: "pooled", Tags: tags})
	if v := sp.Get(); v != nil {
		v.(Rec).Tags[0] = "u"
		// Go keeps the item in a per-P cache: it comes back here.
		fmt.Println(v.(Rec).Name, tags[0] == "u")
	} else {
		fmt.Println("pooled true")
	}
	fmt.Println(sp.Get())
}

func atomicValue() {
	var v atomic.Value
	tags := []string{"a"}
	v.Store(Rec{Name: "a", Tags: tags})
	v.Load().(Rec).Tags[0] = "b"
	fmt.Println(tags)
	old := v.Swap(Rec{Name: "c"})
	fmt.Println(old.(Rec).Name, v.Load().(Rec).Name)
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(v.CompareAndSwap(Rec{Name: "c"}, Rec{Name: "d"}))
	}()

	var pv atomic.Value
	pv.Store(Pt{1, 2})
	fmt.Println(pv.CompareAndSwap(Pt{1, 2}, Pt{3, 4}), pv.Load())
	try := func(name string, f func()) {
		defer func() { fmt.Println(name, "recovered:", recover()) }()
		f()
	}
	try("store other", func() { pv.Store(Key{"a", 1}) })
	try("store int", func() { pv.Store(1) })
	try("swap other", func() { pv.Swap(Key{"a", 1}) })
	try("cas other", func() { pv.CompareAndSwap(Pt{3, 4}, Key{"a", 1}) })
	try("store nil", func() { pv.Store(nil) })
	try("swap nil", func() { pv.Swap(nil) })
	try("cas nil", func() { pv.CompareAndSwap(Pt{3, 4}, nil) })
	fmt.Println(pv.Load())
	var pp atomic.Value
	pp.Store(&Pt{1, 1})
	try("store ptr other", func() { pp.Store(&Key{}) })
	pp.Load().(*Pt).X = 5
	fmt.Println(*pp.Load().(*Pt))
}

func concurrent() {
	var m sync.Map
	type counter struct{ n *int64 }
	var total int64
	m.Store("c", counter{&total})
	shared := &Inner{}
	m.Store("s", shared)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				v, _ := m.Load("c")
				atomic.AddInt64(v.(counter).n, 1)
				s, _ := m.Load("s")
				mu.Lock()
				s.(*Inner).N++
				mu.Unlock()
				m.Store(Key{"g", g}, Pt{g, i})
				m.LoadOrStore(Key{"once", 0}, Pt{0, 0})
			}
		}(g)
	}
	wg.Wait()
	fmt.Println(total, shared.N)
	n := 0
	m.Range(func(k, v any) bool {
		if kk, ok := k.(Key); ok && kk.A == "g" {
			n += v.(Pt).Y
		}
		return true
	})
	fmt.Println(n)
}

func main() {
	structValues()
	pointers()
	loadOrStore()
	swap()
	compare()
	structKeys()
	rangeMap()
	pool()
	atomicValue()
	concurrent()
}
