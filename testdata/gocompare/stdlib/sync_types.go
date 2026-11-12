// sync and sync/atomic with program types: sync.Map of program keys and
// values (printed sorted), Once, OnceValue and OnceValues with program
// results and panics, atomic.Value and atomic.Pointer of program types,
// Mutex embedded in program structs, and a WaitGroup with Go.
package main

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

type Key struct {
	NS   string
	Name string
}

type Entry struct {
	Hits int
	Tags []string
}

type Counter struct {
	sync.Mutex
	counts map[Key]int
}

func (c *Counter) Inc(k Key) {
	c.Lock()
	defer c.Unlock()
	c.counts[k]++
}

type Config struct {
	Name    string
	Version int
}

func main() {
	var m sync.Map
	keys := []Key{{"a", "x"}, {"b", "y"}, {"a", "z"}}
	for i, k := range keys {
		m.Store(k, Entry{Hits: i, Tags: []string{k.Name}})
	}
	v, ok := m.Load(Key{"a", "z"})
	fmt.Println(v, ok)
	v, loaded := m.LoadOrStore(Key{"a", "x"}, Entry{Hits: 99})
	fmt.Println(v, loaded)
	v, loaded = m.LoadOrStore(Key{"c", "w"}, Entry{Hits: 5})
	fmt.Println(v, loaded)
	prev, loaded := m.Swap(Key{"b", "y"}, Entry{Hits: 7})
	fmt.Println(prev, loaded)
	m.Store(Key{"n", ""}, Config{"c", 5})
	fmt.Println(m.CompareAndSwap(Key{"n", ""}, Config{"c", 5}, Config{"c", 6}), m.CompareAndSwap(Key{"n", ""}, Config{"c", 5}, Config{}), m.CompareAndDelete(Key{"n", ""}, Config{"c", 7}))
	v, _ = m.Load(Key{"n", ""})
	fmt.Println(v, m.CompareAndDelete(Key{"n", ""}, Config{"c", 6}))
	m.Delete(Key{"a", "z"})
	v, loaded = m.LoadAndDelete(Key{"zz", ""})
	fmt.Println(v, loaded)
	var lines []string
	m.Range(func(k, v any) bool {
		lines = append(lines, fmt.Sprintf("%v=%v", k, v))
		return true
	})
	sort.Strings(lines)
	fmt.Println(lines)
	m.Clear()
	n := 0
	m.Range(func(k, v any) bool { n++; return true })
	fmt.Println(n)

	c := &Counter{counts: map[Key]int{}}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Go(func() { c.Inc(keys[i%3]) })
	}
	wg.Wait()
	fmt.Println(c.counts)

	var once sync.Once
	calls := 0
	for i := 0; i < 3; i++ {
		once.Do(func() { calls++ })
	}
	fmt.Println(calls)
	load := sync.OnceValue(func() Config { calls++; return Config{"cfg", calls} })
	fmt.Println(load(), load(), calls)
	parse := sync.OnceValues(func() (Config, error) { return Config{}, errors.New("parse failed") })
	cfg, err := parse()
	cfg2, err2 := parse()
	fmt.Println(cfg, err, cfg2, err2 == err)
	boom := sync.OnceFunc(func() { panic(Config{"panic", 1}) })
	for i := 0; i < 2; i++ {
		func() {
			defer func() { fmt.Println("recovered:", recover()) }()
			boom()
		}()
	}

	var av atomic.Value
	fmt.Println(av.Load())
	av.Store(Config{"one", 1})
	fmt.Println(av.Load().(Config), av.Swap(Config{"two", 2}), av.CompareAndSwap(Config{"two", 2}, Config{"three", 3}), av.Load())
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		av.Store("a string")
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		av.Store(nil)
	}()
	var ap atomic.Pointer[Config]
	fmt.Println(ap.Load() == nil)
	c1 := &Config{"p1", 1}
	ap.Store(c1)
	old := ap.Swap(&Config{"p2", 2})
	fmt.Println(old.Name, ap.Load().Name, ap.CompareAndSwap(c1, nil), ap.CompareAndSwap(ap.Load(), c1), ap.Load().Name)
	var ai atomic.Int64
	var au atomic.Uint32
	var ab atomic.Bool
	for i := 0; i < 20; i++ {
		wg.Go(func() { ai.Add(3); au.Add(1); ab.Store(true) })
	}
	wg.Wait()
	fmt.Println(ai.Load(), au.Load(), ab.Load(), ai.CompareAndSwap(60, -1), ai.Load(), au.And(0b1100), au.Load(), ai.Or(0))
	var rw sync.RWMutex
	rw.RLock()
	fmt.Println(rw.TryLock(), rw.TryRLock())
	rw.RUnlock()
	rw.RUnlock()
	fmt.Println(rw.TryLock())
	rw.Unlock()
	cond := sync.NewCond(&sync.Mutex{})
	ready := false
	done := make(chan Config)
	go func() {
		cond.L.Lock()
		for !ready {
			cond.Wait()
		}
		cond.L.Unlock()
		done <- Config{"signaled", 1}
	}()
	cond.L.Lock()
	ready = true
	cond.Broadcast()
	cond.L.Unlock()
	fmt.Println(<-done)
}
