// sync.Map: Load, Store, LoadOrStore, LoadAndDelete, Delete, Swap,
// CompareAndSwap, CompareAndDelete, Clear and Range, with concurrent stores;
// the Range results are collected and sorted.
package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

func dump(m *sync.Map) string {
	var parts []string
	m.Range(func(k, v any) bool {
		parts = append(parts, fmt.Sprintf("%v=%v", k, v))
		return true
	})
	sort.Strings(parts)
	return "[" + strings.Join(parts, " ") + "]"
}

func main() {
	var m sync.Map
	v, ok := m.Load("x")
	fmt.Println(v, ok)
	m.Store("x", 1)
	m.Store("y", "two")
	m.Store(3, 3.5)
	v, ok = m.Load("x")
	fmt.Println(v, ok)
	fmt.Println(dump(&m))

	act, loaded := m.LoadOrStore("x", 100)
	fmt.Println("LoadOrStore x", act, loaded)
	act, loaded = m.LoadOrStore("z", 26)
	fmt.Println("LoadOrStore z", act, loaded)

	old, loaded := m.LoadAndDelete("y")
	fmt.Println("LoadAndDelete y", old, loaded)
	old, loaded = m.LoadAndDelete("y")
	fmt.Println("LoadAndDelete y again", old, loaded)

	prev, loaded := m.Swap("x", 11)
	fmt.Println("Swap x", prev, loaded)
	prev, loaded = m.Swap("new", "n")
	fmt.Println("Swap new", prev, loaded)

	fmt.Println("CAS x 11->12", m.CompareAndSwap("x", 11, 12))
	fmt.Println("CAS x 11->13", m.CompareAndSwap("x", 11, 13))
	fmt.Println("CAS missing", m.CompareAndSwap("missing", nil, 1))
	fmt.Println("CAD z 25", m.CompareAndDelete("z", 25))
	fmt.Println("CAD z 26", m.CompareAndDelete("z", 26))
	m.Delete(3)
	m.Delete("never")
	fmt.Println(dump(&m))

	// Range stops when f returns false.
	for i := 0; i < 10; i++ {
		m.Store(fmt.Sprintf("k%d", i), i)
	}
	n := 0
	m.Range(func(k, v any) bool {
		n++
		return n < 3
	})
	fmt.Println("range stopped at", n)

	m.Clear()
	fmt.Println("after Clear", dump(&m))

	// Concurrent stores and LoadOrStore: exactly one winner per key.
	var cm sync.Map
	var wg sync.WaitGroup
	winners := make([]int, 10)
	var mu sync.Mutex
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 10; k++ {
				if _, loaded := cm.LoadOrStore(k, g); !loaded {
					mu.Lock()
					winners[k]++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("winners", winners)

	// Concurrent counter with CompareAndSwap loops.
	var counters sync.Map
	for k := 0; k < 3; k++ {
		counters.Store(k, 0)
	}
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				k := i % 3
				for {
					cur, _ := counters.Load(k)
					if counters.CompareAndSwap(k, cur, cur.(int)+1) {
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println(dump(&counters))

	// Values of various types and nil.
	var tm sync.Map
	tm.Store("nil", nil)
	tm.Store("slice", []int{1, 2})
	tm.Store(struct{ a int }{1}, "struct key")
	v, ok = tm.Load("nil")
	fmt.Println(v, ok)
	v, ok = tm.Load(struct{ a int }{1})
	fmt.Println(v, ok)
	fmt.Println(dump(&tm))
}
